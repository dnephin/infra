package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/opt"

	"github.com/infrahq/infra/api"
	"github.com/infrahq/infra/internal"
	"github.com/infrahq/infra/internal/generate"
	"github.com/infrahq/infra/internal/server/data"
	"github.com/infrahq/infra/internal/server/models"
	"github.com/infrahq/infra/internal/testing/database"
	tpatch "github.com/infrahq/infra/internal/testing/patch"
	"github.com/infrahq/infra/uid"
)

func setupDB(t *testing.T) *data.DB {
	t.Helper()
	driver := database.PostgresDriver(t, "_server")
	if driver == nil {
		lite, err := data.NewSQLiteDriver("file::memory:")
		assert.NilError(t, err)
		driver = &database.Driver{Dialector: lite}
	}

	tpatch.ModelsSymmetricKey(t)
	db, err := data.NewDB(driver.Dialector, nil)
	assert.NilError(t, err)
	t.Cleanup(data.InvalidateCache)

	t.Cleanup(func() {
		assert.NilError(t, db.Close())
	})

	db.Statement.Context = data.WithOrg(db.Statement.Context, db.DefaultOrg)
	return db
}

func issueToken(t *testing.T, db *gorm.DB, identityName string, sessionDuration time.Duration) string {
	user := &models.Identity{Name: identityName}

	err := data.CreateIdentity(db, user)
	assert.NilError(t, err)

	provider := data.InfraProvider(db)

	token := &models.AccessKey{
		IssuedFor:  user.ID,
		ProviderID: provider.ID,
		ExpiresAt:  time.Now().Add(sessionDuration).UTC(),
	}
	body, err := data.CreateAccessKey(db, token)
	assert.NilError(t, err)

	return body
}

func TestRequestTimeoutError(t *testing.T) {
	router := gin.New()
	router.Use(TimeoutMiddleware(100 * time.Millisecond))
	router.GET("/", func(c *gin.Context) {
		time.Sleep(110 * time.Millisecond)

		assert.ErrorIs(t, c.Request.Context().Err(), context.DeadlineExceeded)

		c.Status(200)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

func TestRequestTimeoutSuccess(t *testing.T) {
	router := gin.New()
	router.Use(TimeoutMiddleware(60 * time.Second))
	router.GET("/", func(c *gin.Context) {
		assert.NilError(t, c.Request.Context().Err())

		c.Status(200)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

func TestDBTimeout(t *testing.T) {
	var ctx context.Context
	var cancel context.CancelFunc

	srv := newServer(Options{})
	srv.db = setupDB(t)

	router := gin.New()
	router.Use(
		func(c *gin.Context) {
			// this is a custom copy of the timeout middleware so I can grab and control the cancel() func. Otherwise the test is too flakey with timing race conditions.
			ctx, cancel = context.WithTimeout(c.Request.Context(), 100*time.Millisecond)
			defer cancel()

			c.Request = c.Request.WithContext(ctx)
			c.Next()
		},
		unauthenticatedMiddleware(srv),
	)
	router.GET("/", func(c *gin.Context) {
		db, ok := c.MustGet("db").(*gorm.DB)
		assert.Check(t, ok)
		cancel()
		err := db.Exec("select 1;").Error
		assert.Error(t, err, "context canceled")

		c.Status(200)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

func TestRequireAccessKey(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"AccessKeyValid": {
			"authFunc": func(t *testing.T, db *gorm.DB, c *gin.Context) {
				authentication := issueToken(t, db, "existing@infrahq.com", time.Minute*1)
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Add("Authorization", "Bearer "+authentication)
				c.Request = r
			},
			"verifyFunc": func(t *testing.T, c *gin.Context, err error) {
				assert.NilError(t, err)
			},
		},
		"AccessKeyExpired": {
			"authFunc": func(t *testing.T, db *gorm.DB, c *gin.Context) {
				authentication := issueToken(t, db, "existing@infrahq.com", time.Minute*-1)
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Add("Authorization", "Bearer "+authentication)
				c.Request = r
			},
			"verifyFunc": func(t *testing.T, c *gin.Context, err error) {
				assert.ErrorIs(t, err, data.ErrAccessKeyExpired)
			},
		},
		"AccessKeyInvalidKey": {
			"authFunc": func(t *testing.T, db *gorm.DB, c *gin.Context) {
				token := issueToken(t, db, "existing@infrahq.com", time.Minute*1)
				secret := token[:models.AccessKeySecretLength]
				authentication := fmt.Sprintf("%s.%s", uid.New().String(), secret)
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Add("Authorization", "Bearer "+authentication)
				c.Request = r
			},
			"verifyFunc": func(t *testing.T, c *gin.Context, err error) {
				assert.ErrorContains(t, err, "record not found")
			},
		},
		"AccessKeyNoMatch": {
			"authFunc": func(t *testing.T, db *gorm.DB, c *gin.Context) {
				authentication := fmt.Sprintf("%s.%s", uid.New().String(), generate.MathRandom(models.AccessKeySecretLength, generate.CharsetAlphaNumeric))
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Add("Authorization", "Bearer "+authentication)
				c.Request = r
			},
			"verifyFunc": func(t *testing.T, c *gin.Context, err error) {
				assert.ErrorContains(t, err, "record not found")
			},
		},
		"AccessKeyInvalidSecret": {
			"authFunc": func(t *testing.T, db *gorm.DB, c *gin.Context) {
				token := issueToken(t, db, "existing@infrahq.com", time.Minute*1)
				authentication := fmt.Sprintf("%s.%s", strings.Split(token, ".")[0], generate.MathRandom(models.AccessKeySecretLength, generate.CharsetAlphaNumeric))
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Add("Authorization", "Bearer "+authentication)
				c.Request = r
			},
			"verifyFunc": func(t *testing.T, c *gin.Context, err error) {
				assert.ErrorContains(t, err, "access key invalid secret")
			},
		},
		"UnknownAuthenticationMethod": {
			"authFunc": func(t *testing.T, db *gorm.DB, c *gin.Context) {
				authentication, err := generate.CryptoRandom(32, generate.CharsetAlphaNumeric)
				assert.NilError(t, err)
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Add("Authorization", "Bearer "+authentication)
				c.Request = r
			},
			"verifyFunc": func(t *testing.T, c *gin.Context, err error) {
				assert.ErrorContains(t, err, "invalid access key format")
			},
		},
		"NoAuthentication": {
			"authFunc": func(t *testing.T, db *gorm.DB, c *gin.Context) {
				// nil pointer if we don't seup the request header here
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				c.Request = r
			},
			"verifyFunc": func(t *testing.T, c *gin.Context, err error) {
				assert.ErrorContains(t, err, "valid token not found in request")
			},
		},
		"EmptyAuthentication": {
			"authFunc": func(t *testing.T, db *gorm.DB, c *gin.Context) {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Add("Authorization", "")
				c.Request = r
			},
			"verifyFunc": func(t *testing.T, c *gin.Context, err error) {
				assert.ErrorContains(t, err, "valid token not found in request")
			},
		},
		"EmptySpaceAuthentication": {
			"authFunc": func(t *testing.T, db *gorm.DB, c *gin.Context) {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Add("Authorization", " ")
				c.Request = r
			},
			"verifyFunc": func(t *testing.T, c *gin.Context, err error) {
				assert.ErrorContains(t, err, "valid token not found in request")
			},
		},
		"EmptyCookieAuthentication": {
			"authFunc": func(t *testing.T, db *gorm.DB, c *gin.Context) {
				r := httptest.NewRequest(http.MethodGet, "/", nil)

				r.AddCookie(&http.Cookie{
					Name:     cookieAuthorizationName,
					MaxAge:   int(time.Until(time.Now().Add(time.Minute * 1)).Seconds()),
					Path:     cookiePath,
					SameSite: http.SameSiteStrictMode,
					Secure:   true,
					HttpOnly: true,
				})

				r.Header.Add("Authorization", " ")
				c.Request = r
			},
			"verifyFunc": func(t *testing.T, c *gin.Context, err error) {
				assert.ErrorContains(t, err, "skipped validating empty token")
			},
		},
	}

	for k, v := range cases {
		t.Run(k, func(t *testing.T) {
			db := setupDB(t).DB

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("db", db)

			authFunc, ok := v["authFunc"].(func(*testing.T, *gorm.DB, *gin.Context))
			assert.Assert(t, ok)
			authFunc(t, db, c)

			_, err := requireAccessKey(db, c.Request)

			verifyFunc, ok := v["verifyFunc"].(func(*testing.T, *gin.Context, error))
			assert.Assert(t, ok)

			verifyFunc(t, c, err)
		})
	}
}

func TestHandleInfraDestinationHeader(t *testing.T) {
	srv := setupServer(t, withAdminUser)
	routes := srv.GenerateRoutes(prometheus.NewRegistry())
	db := srv.DB()

	connector := models.Identity{Name: "connectorA"}
	err := data.CreateIdentity(db, &connector)
	assert.NilError(t, err)

	grant := models.Grant{
		Subject:   uid.NewIdentityPolymorphicID(connector.ID),
		Privilege: models.InfraConnectorRole,
		Resource:  "infra",
	}
	err = data.CreateGrant(db, &grant)
	assert.NilError(t, err)

	token := models.AccessKey{
		IssuedFor:  connector.ID,
		ProviderID: data.InfraProvider(db).ID,
		ExpiresAt:  time.Now().Add(time.Hour).UTC(),
	}
	secret, err := data.CreateAccessKey(db, &token)
	assert.NilError(t, err)

	t.Run("good", func(t *testing.T) {
		destination := &models.Destination{Name: t.Name(), UniqueID: t.Name()}
		err := data.CreateDestination(db, destination)
		assert.NilError(t, err)

		r := httptest.NewRequest("GET", "/api/grants", nil)
		r.Header.Add("Infra-Destination", destination.UniqueID)
		r.Header.Add("Authorization", fmt.Sprintf("Bearer %s", secret))
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)

		destination, err = data.GetDestination(db, data.ByOptionalUniqueID(destination.UniqueID))
		assert.NilError(t, err)
		assert.DeepEqual(t, destination.LastSeenAt, time.Now(), opt.TimeWithThreshold(time.Second))
	})

	t.Run("good no destination header", func(t *testing.T) {
		destination := &models.Destination{Name: t.Name(), UniqueID: t.Name()}
		err := data.CreateDestination(db, destination)
		assert.NilError(t, err)

		r := httptest.NewRequest("GET", "/good", nil)
		r.Header.Add("Authorization", fmt.Sprintf("Bearer %s", secret))
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)

		destination, err = data.GetDestination(db, data.ByOptionalUniqueID(destination.UniqueID))
		assert.NilError(t, err)
		assert.Equal(t, destination.LastSeenAt.UTC(), time.Time{})
	})

	t.Run("good no destination", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/good", nil)
		r.Header.Add("Infra-Destination", "nonexistent")
		r.Header.Add("Authorization", fmt.Sprintf("Bearer %s", secret))
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)

		_, err := data.GetDestination(db, data.ByOptionalUniqueID("nonexistent"))
		assert.ErrorIs(t, err, internal.ErrNotFound)
	})
}

func TestGetOrgFromRequest_FromRequestHost(t *testing.T) {
	srv := setupServer(t, withAdminUser)
	db := srv.DB()

	router := gin.New()

	org := &models.Organization{
		Name:   "The Umbrella Academy",
		Domain: "umbrella.infrahq.com",
	}
	err := data.CreateOrganization(db, org)
	assert.NilError(t, err)

	router.GET("/foo",
		setOrganizationInCtx(srv),
		unauthenticatedMiddleware(srv),
		func(ctx *gin.Context) {
			ctxOrg := data.OrgFromContext(ctx)

			assert.Check(t, ctxOrg != nil)
			assert.Check(t, ctxOrg.ID == org.ID, "got org %v", ctxOrg.Name)
			ctx.JSON(200, api.EmptyResponse{})
		})

	httpSrv := httptest.NewServer(router)
	t.Cleanup(httpSrv.Close)

	req, err := http.NewRequest("GET", httpSrv.URL+"/foo", nil)
	assert.NilError(t, err)
	req.Host = org.Domain

	client := httpSrv.Client()
	resp, err := client.Do(req)
	assert.NilError(t, err)

	assert.Equal(t, resp.StatusCode, 200)
}

func TestGetOrgFromRequest_MissingOrgErrors(t *testing.T) {
	srv := setupServer(t, withAdminUser)

	router := gin.New()

	router.GET("/foo",
		unauthenticatedMiddleware(srv),
		orgRequired(),
		func(ctx *gin.Context) {
			assert.Assert(t, false, "Shouldn't get here")
		})

	req := httptest.NewRequest("GET", "http://notadomainweknowabout.org/foo", nil)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, resp.Code, http.StatusBadRequest)
}

func TestGetOrgFromRequest_UsesDefaultWhenDomainDoesNotMatchAnyOrg(t *testing.T) {
	srv := setupServer(t, withAdminUser)

	router := gin.New()

	router.GET("/foo",
		unauthenticatedMiddleware(srv),
		func(ctx *gin.Context) {
			ctx.JSON(200, nil)
		})

	req := httptest.NewRequest("GET", "/foo", nil)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, resp.Code, http.StatusOK)
}
