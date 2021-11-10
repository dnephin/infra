/*
Infra API

Infra REST API

API version: 0.1.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package api

import (
	"encoding/json"
)

// DestinationCreateRequest struct for DestinationCreateRequest
type DestinationCreateRequest struct {
	NodeID     string                 `json:"nodeID"`
	Name       string                 `json:"name"`
	Kind       DestinationKind        `json:"kind"`
	Labels     []string               `json:"labels"`
	Kubernetes *DestinationKubernetes `json:"kubernetes,omitempty"`
}

// NewDestinationCreateRequest instantiates a new DestinationCreateRequest object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewDestinationCreateRequest(nodeID string, name string, kind DestinationKind, labels []string) *DestinationCreateRequest {
	this := DestinationCreateRequest{}
	this.NodeID = nodeID
	this.Name = name
	this.Kind = kind
	this.Labels = labels
	return &this
}

// NewDestinationCreateRequestWithDefaults instantiates a new DestinationCreateRequest object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewDestinationCreateRequestWithDefaults() *DestinationCreateRequest {
	this := DestinationCreateRequest{}
	return &this
}

// GetNodeID returns the NodeID field value
func (o *DestinationCreateRequest) GetNodeID() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.NodeID
}

// GetNodeIDOk returns a tuple with the NodeID field value
// and a boolean to check if the value has been set.
func (o *DestinationCreateRequest) GetNodeIDOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.NodeID, true
}

// SetNodeID sets field value
func (o *DestinationCreateRequest) SetNodeID(v string) {
	o.NodeID = v
}

// GetName returns the Name field value
func (o *DestinationCreateRequest) GetName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Name
}

// GetNameOk returns a tuple with the Name field value
// and a boolean to check if the value has been set.
func (o *DestinationCreateRequest) GetNameOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Name, true
}

// SetName sets field value
func (o *DestinationCreateRequest) SetName(v string) {
	o.Name = v
}

// GetKind returns the Kind field value
func (o *DestinationCreateRequest) GetKind() DestinationKind {
	if o == nil {
		var ret DestinationKind
		return ret
	}

	return o.Kind
}

// GetKindOk returns a tuple with the Kind field value
// and a boolean to check if the value has been set.
func (o *DestinationCreateRequest) GetKindOk() (*DestinationKind, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Kind, true
}

// SetKind sets field value
func (o *DestinationCreateRequest) SetKind(v DestinationKind) {
	o.Kind = v
}

// GetLabels returns the Labels field value
func (o *DestinationCreateRequest) GetLabels() []string {
	if o == nil {
		var ret []string
		return ret
	}

	return o.Labels
}

// GetLabelsOk returns a tuple with the Labels field value
// and a boolean to check if the value has been set.
func (o *DestinationCreateRequest) GetLabelsOk() (*[]string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Labels, true
}

// SetLabels sets field value
func (o *DestinationCreateRequest) SetLabels(v []string) {
	o.Labels = v
}

// GetKubernetes returns the Kubernetes field value if set, zero value otherwise.
func (o *DestinationCreateRequest) GetKubernetes() DestinationKubernetes {
	if o == nil || o.Kubernetes == nil {
		var ret DestinationKubernetes
		return ret
	}
	return *o.Kubernetes
}

// GetKubernetesOk returns a tuple with the Kubernetes field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *DestinationCreateRequest) GetKubernetesOk() (*DestinationKubernetes, bool) {
	if o == nil || o.Kubernetes == nil {
		return nil, false
	}
	return o.Kubernetes, true
}

// HasKubernetes returns a boolean if a field has been set.
func (o *DestinationCreateRequest) HasKubernetes() bool {
	if o != nil && o.Kubernetes != nil {
		return true
	}

	return false
}

// SetKubernetes gets a reference to the given DestinationKubernetes and assigns it to the Kubernetes field.
func (o *DestinationCreateRequest) SetKubernetes(v DestinationKubernetes) {
	o.Kubernetes = &v
}

func (o DestinationCreateRequest) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["nodeID"] = o.NodeID
	}
	if true {
		toSerialize["name"] = o.Name
	}
	if true {
		toSerialize["kind"] = o.Kind
	}
	if true {
		toSerialize["labels"] = o.Labels
	}
	if o.Kubernetes != nil {
		toSerialize["kubernetes"] = o.Kubernetes
	}
	return json.Marshal(toSerialize)
}

type NullableDestinationCreateRequest struct {
	value *DestinationCreateRequest
	isSet bool
}

func (v NullableDestinationCreateRequest) Get() *DestinationCreateRequest {
	return v.value
}

func (v *NullableDestinationCreateRequest) Set(val *DestinationCreateRequest) {
	v.value = val
	v.isSet = true
}

func (v NullableDestinationCreateRequest) IsSet() bool {
	return v.isSet
}

func (v *NullableDestinationCreateRequest) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableDestinationCreateRequest(val *DestinationCreateRequest) *NullableDestinationCreateRequest {
	return &NullableDestinationCreateRequest{value: val, isSet: true}
}

func (v NullableDestinationCreateRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableDestinationCreateRequest) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
