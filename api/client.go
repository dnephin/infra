package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"time"

	"github.com/ssoroka/slice"

	"github.com/infrahq/infra/internal/logging"
	"github.com/infrahq/infra/uid"
)

var apiVersion = "0.13.0"

var (
	ErrTimeout            = errors.New("client timed out waiting for response from server")
	ErrDeviceLoginTimeout = errors.New("timed out waiting for user to complete device login")
)

const (
	InfraAdminRole     = "admin"
	InfraViewRole      = "view"
	InfraConnectorRole = "connector"
)

type Client struct {
	Name      string
	Version   string
	URL       string
	AccessKey string
	HTTP      http.Client
	// Headers are HTTP headers that will be added to every request made by the Client.
	Headers http.Header
	// OnUnauthorized is a callback hook for the client to get notified of a 401 Unauthorized response to any query.
	// This is useful as clients often need to discard expired access keys.
	OnUnauthorized func()

	// ObserveFunc is a callback to measure and record the status and duration of the request
	ObserveFunc func(time.Time, *http.Request, *http.Response, error)
}

// checkError checks the resp for an error code, and returns an api.Error with
// details about the error. Returns nil if the status code is 2xx.
//
// 3xx codes are considered an error because redirects should have already
// been followed before calling checkError.
func checkError(resp *http.Response, body []byte) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	apiError := Error{Code: int32(resp.StatusCode)}

	err := json.Unmarshal(body, &apiError)
	if err != nil {
		// Use the full body as the message if we fail to decode a response.
		apiError.Message = string(body)
	}

	return apiError
}

// ErrorStatusCode returns the http status code from the error.
// Returns 0 if the error is nil, or if the error is not of type Error.
func ErrorStatusCode(err error) int32 {
	var apiError Error
	if errors.As(err, &apiError) {
		return apiError.Code
	}
	return 0
}

func (c *Client) buildRequest(
	ctx context.Context,
	method string,
	path string,
	query Query,
	body io.Reader,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("%s%s", c.URL, path), body)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = url.Values(query).Encode()
	clientName, clientVersion := "client", "unknown"
	if c.Name != "" {
		clientName = c.Name
	}

	if c.Version != "" {
		clientVersion = c.Version
	}

	req.Header.Add("Authorization", "Bearer "+c.AccessKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Infra-Version", apiVersion)
	req.Header.Set("User-Agent", fmt.Sprintf("Infra/%v (%s %v; %v/%v)", apiVersion, clientName, clientVersion, runtime.GOOS, runtime.GOARCH))

	for k, v := range c.Headers {
		req.Header[k] = v
	}
	return req, nil
}

func request[Res any](client Client, req *http.Request) (*Res, error) {
	start := time.Now()
	resp, err := client.HTTP.Do(req)

	if client.ObserveFunc != nil {
		client.ObserveFunc(start, req, resp, err)
	}

	if resp != nil && resp.StatusCode == 401 && client.OnUnauthorized != nil {
		defer client.OnUnauthorized()
	}

	if err != nil {
		if connError := HandleConnError(err); connError != nil {
			return nil, connError
		}
		return nil, fmt.Errorf("%s %q: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %s", ErrTimeout, err)
		}
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if err := checkError(resp, body); err != nil {
		return nil, err
	}

	var resBody Res
	if len(body) > 0 {
		if err := json.Unmarshal(body, &resBody); err != nil {
			return nil, fmt.Errorf("parsing json response: %w. partial text: %q", err, partialText(body, 100))
		}
	}

	if h, ok := any(&resBody).(readsResponseHeader); ok {
		if err := h.setValuesFromHeader(resp.Header); err != nil {
			return nil, err
		}
	}

	return &resBody, nil
}

type readsResponseHeader interface {
	setValuesFromHeader(header http.Header) error
}

func get[Res any](ctx context.Context, client Client, path string, query Query) (*Res, error) {
	req, err := client.buildRequest(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, err
	}
	return request[Res](client, req)
}

func post[Req, Res any](client Client, path string, req *Req) (*Res, error) {
	body, err := encodeRequestBody(req)
	if err != nil {
		return nil, err
	}
	ctx := context.TODO()
	httpReq, err := client.buildRequest(ctx, http.MethodPost, path, nil, body)
	if err != nil {
		return nil, err
	}
	return request[Res](client, httpReq)
}

func encodeRequestBody(req any) (io.Reader, error) {
	if req == nil {
		return nil, nil
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	return bytes.NewReader(b), nil
}

func put[Req, Res any](client Client, path string, req *Req) (*Res, error) {
	body, err := encodeRequestBody(req)
	if err != nil {
		return nil, err
	}
	ctx := context.TODO()
	httpReq, err := client.buildRequest(ctx, http.MethodPut, path, nil, body)
	if err != nil {
		return nil, err
	}
	return request[Res](client, httpReq)
}

func patch[Req, Res any](client Client, path string, req *Req) (*Res, error) {
	body, err := encodeRequestBody(req)
	if err != nil {
		return nil, err
	}
	ctx := context.TODO()
	httpReq, err := client.buildRequest(ctx, http.MethodPatch, path, nil, body)
	if err != nil {
		return nil, err
	}
	return request[Res](client, httpReq)
}

func delete(client Client, path string, query Query) error {
	ctx := context.TODO()
	httpReq, err := client.buildRequest(ctx, http.MethodDelete, path, query, nil)
	if err != nil {
		return err
	}
	_, err = request[EmptyResponse](client, httpReq)
	return err
}

func (c Client) ListUsers(ctx context.Context, req ListUsersRequest) (*ListResponse[User], error) {
	ids := slice.Map[uid.ID, string](req.IDs, func(id uid.ID) string {
		return id.String()
	})
	return get[ListResponse[User]](ctx, c, "/api/users", Query{
		"name": {req.Name}, "group": {req.Group.String()}, "ids": ids,
		"page": {strconv.Itoa(req.Page)}, "limit": {strconv.Itoa(req.Limit)},
		"showSystem": {strconv.FormatBool(req.ShowSystem)},
	})
}

func (c Client) GetUser(id uid.ID) (*User, error) {
	ctx := context.TODO()
	return get[User](ctx, c, fmt.Sprintf("/api/users/%s", id), Query{})
}

func (c Client) CreateUser(req *CreateUserRequest) (*CreateUserResponse, error) {
	return post[CreateUserRequest, CreateUserResponse](c, "/api/users", req)
}

func (c Client) UpdateUser(req *UpdateUserRequest) (*User, error) {
	return put[UpdateUserRequest, User](c, fmt.Sprintf("/api/users/%s", req.ID.String()), req)
}

func (c Client) DeleteUser(id uid.ID) error {
	return delete(c, fmt.Sprintf("/api/users/%s", id), Query{})
}

func (c Client) StartDeviceFlow() (*DeviceFlowResponse, error) {
	return post[EmptyRequest, DeviceFlowResponse](c, "/api/device", nil)
}

func (c Client) GetDeviceFlowStatus(req *DeviceFlowStatusRequest) (*DeviceFlowStatusResponse, error) {
	return post[DeviceFlowStatusRequest, DeviceFlowStatusResponse](c, "/api/device/status", &DeviceFlowStatusRequest{
		DeviceCode: req.DeviceCode,
	})
}

func (c Client) ListGroups(ctx context.Context, req ListGroupsRequest) (*ListResponse[Group], error) {
	return get[ListResponse[Group]](ctx, c, "/api/groups", Query{
		"name": {req.Name}, "userID": {req.UserID.String()},
		"page": {strconv.Itoa(req.Page)}, "limit": {strconv.Itoa(req.Limit)},
	})
}

func (c Client) GetGroup(id uid.ID) (*Group, error) {
	ctx := context.TODO()
	return get[Group](ctx, c, fmt.Sprintf("/api/groups/%s", id), Query{})
}

func (c Client) CreateGroup(req *CreateGroupRequest) (*Group, error) {
	return post[CreateGroupRequest, Group](c, "/api/groups", req)
}

func (c Client) DeleteGroup(id uid.ID) error {
	return delete(c, fmt.Sprintf("/api/groups/%s", id), Query{})
}

func (c Client) UpdateUsersInGroup(req *UpdateUsersInGroupRequest) error {
	_, err := patch[UpdateUsersInGroupRequest, EmptyResponse](c, fmt.Sprintf("/api/groups/%s/users", req.GroupID), req)
	return err
}

func (c Client) ListProviders(ctx context.Context, req ListProvidersRequest) (*ListResponse[Provider], error) {
	return get[ListResponse[Provider]](ctx, c, "/api/providers", Query{
		"name": {req.Name},
		"page": {strconv.Itoa(req.Page)}, "limit": {strconv.Itoa(req.Limit)},
	})
}

func (c Client) ListOrganizations(ctx context.Context, req ListOrganizationsRequest) (*ListResponse[Organization], error) {
	return get[ListResponse[Organization]](ctx, c, "/api/organizations", Query{
		"name": {req.Name},
	})
}

func (c Client) GetOrganization(id uid.ID) (*Organization, error) {
	ctx := context.TODO()
	return get[Organization](ctx, c, fmt.Sprintf("/api/organizations/%s", id), Query{})
}

func (c Client) CreateOrganization(req *CreateOrganizationRequest) (*Organization, error) {
	return post[CreateOrganizationRequest, Organization](c, "/api/organizations", req)
}

func (c Client) DeleteOrganization(id uid.ID) error {
	return delete(c, fmt.Sprintf("/api/organizations/%s", id), Query{})
}

func (c Client) GetProvider(id uid.ID) (*Provider, error) {
	ctx := context.TODO()
	return get[Provider](ctx, c, fmt.Sprintf("/api/providers/%s", id), Query{})
}

func (c Client) CreateProvider(req *CreateProviderRequest) (*Provider, error) {
	return post[CreateProviderRequest, Provider](c, "/api/providers", req)
}

func (c Client) PatchProvider(req PatchProviderRequest) (*Provider, error) {
	return patch[PatchProviderRequest, Provider](c, fmt.Sprintf("/api/providers/%s", req.ID.String()), &req)
}

func (c Client) UpdateProvider(req UpdateProviderRequest) (*Provider, error) {
	return put[UpdateProviderRequest, Provider](c, fmt.Sprintf("/api/providers/%s", req.ID.String()), &req)
}

func (c Client) DeleteProvider(id uid.ID) error {
	return delete(c, fmt.Sprintf("/api/providers/%s", id), Query{})
}

func (c Client) ListGrants(ctx context.Context, req ListGrantsRequest) (*ListResponse[Grant], error) {
	return get[ListResponse[Grant]](ctx, c, "/api/grants", Query{
		"user":          {req.User.String()},
		"group":         {req.Group.String()},
		"resource":      {req.Resource},
		"destination":   {req.Destination},
		"privilege":     {req.Privilege},
		"showInherited": {strconv.FormatBool(req.ShowInherited)},
		"showSystem":    {strconv.FormatBool(req.ShowSystem)},
		"page":          {strconv.Itoa(req.Page)}, "limit": {strconv.Itoa(req.Limit)},
	})
}

func (c Client) CreateGrant(req *CreateGrantRequest) (*CreateGrantResponse, error) {
	return post[CreateGrantRequest, CreateGrantResponse](c, "/api/grants", req)
}

func (c Client) DeleteGrant(id uid.ID) error {
	return delete(c, fmt.Sprintf("/api/grants/%s", id), Query{})
}

func (c Client) ListDestinations(ctx context.Context, req ListDestinationsRequest) (*ListResponse[Destination], error) {
	return get[ListResponse[Destination]](ctx, c, "/api/destinations", Query{
		"name":      {req.Name},
		"unique_id": {req.UniqueID},
		"page":      {strconv.Itoa(req.Page)}, "limit": {strconv.Itoa(req.Limit)},
	})
}

func (c Client) CreateDestination(req *CreateDestinationRequest) (*Destination, error) {
	return post[CreateDestinationRequest, Destination](c, "/api/destinations", req)
}

func (c Client) UpdateDestination(req UpdateDestinationRequest) (*Destination, error) {
	return put[UpdateDestinationRequest, Destination](c, fmt.Sprintf("/api/destinations/%s", req.ID.String()), &req)
}

func (c Client) DeleteDestination(id uid.ID) error {
	return delete(c, fmt.Sprintf("/api/destinations/%s", id), Query{})
}

func (c Client) ListAccessKeys(ctx context.Context, req ListAccessKeysRequest) (*ListResponse[AccessKey], error) {
	return get[ListResponse[AccessKey]](ctx, c, "/api/access-keys", Query{
		"user_id":      {req.UserID.String()},
		"name":         {req.Name},
		"show_expired": {fmt.Sprint(req.ShowExpired)},
		"page":         {strconv.Itoa(req.Page)}, "limit": {strconv.Itoa(req.Limit)},
	})
}

func (c Client) CreateAccessKey(req *CreateAccessKeyRequest) (*CreateAccessKeyResponse, error) {
	return post[CreateAccessKeyRequest, CreateAccessKeyResponse](c, "/api/access-keys", req)
}

func (c Client) DeleteAccessKey(id uid.ID) error {
	return delete(c, fmt.Sprintf("/api/access-keys/%s", id), Query{})
}

func (c Client) DeleteAccessKeyByName(name string) error {
	return delete(c, "/api/access-keys", Query{"name": []string{name}})
}

func (c Client) CreateToken() (*CreateTokenResponse, error) {
	return post[EmptyRequest, CreateTokenResponse](c, "/api/tokens", &EmptyRequest{})
}

func (c Client) Login(req *LoginRequest) (*LoginResponse, error) {
	return post[LoginRequest, LoginResponse](c, "/api/login", req)
}

func (c Client) Logout() error {
	_, err := post[EmptyRequest, EmptyResponse](c, "/api/logout", &EmptyRequest{})
	return err
}

func (c Client) Signup(req *SignupRequest) (*SignupResponse, error) {
	return post[SignupRequest, SignupResponse](c, "/api/signup", req)
}

func (c Client) GetServerVersion() (*Version, error) {
	ctx := context.TODO()
	return get[Version](ctx, c, "/api/version", Query{})
}

func (c Client) GetSettings() (*Settings, error) {
	ctx := context.TODO()
	return get[Settings](ctx, c, "/api/settings", Query{})
}

func (c Client) UpdateSettings(req *Settings) (*Settings, error) {
	return put[Settings, Settings](c, "/api/settings", req)
}

func partialText(body []byte, limit int) string {
	if len(body) <= limit {
		return string(body)
	}

	return string(body[:limit]) + "..."
}

// HandleConnError translates common connection errors into more informative human
// readable errors. Returns `nil` if the error was not handled, so it is the callers responsibility to
// return the original error if `HandleConnError` returns nil.
func HandleConnError(err error) error {
	urlErr := &url.Error{}
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return fmt.Errorf("%w: %s", ErrTimeout, err)
		}
	}

	if errors.Is(err, io.EOF) {
		logging.Debugf("request error: %v", err)
		return fmt.Errorf("could not reach infra server, please wait a moment and try again")
	}

	return nil
}
