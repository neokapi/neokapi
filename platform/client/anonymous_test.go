package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAnonymousProject(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/projects/anonymous", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "my-project", req["name"])
			assert.Equal(t, "en", req["source_locale"])
			targets := req["target_locales"].([]any)
			assert.Equal(t, []any{"nb", "fr"}, targets)

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"project_id":  "proj_123",
				"claim_token": "clm_abc456",
			})
		}))
		defer srv.Close()

		projectID, claimToken, err := CreateAnonymousProject(srv.URL, "my-project", "en", []string{"nb", "fr"}, "")
		require.NoError(t, err)
		assert.Equal(t, "proj_123", projectID)
		assert.Equal(t, "clm_abc456", claimToken)
	})

	t.Run("with email", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "user@example.com", req["email"])
			assert.Equal(t, "my-project", req["name"])

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"project_id":  "proj_789",
				"claim_token": "clm_email123",
			})
		}))
		defer srv.Close()

		projectID, _, err := CreateAnonymousProject(srv.URL, "my-project", "en", nil, "user@example.com")
		require.NoError(t, err)
		assert.Equal(t, "proj_789", projectID)
	})

	t.Run("empty targets", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Nil(t, req["target_locales"], "empty targets should not be sent")

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"project_id":  "proj_dyn",
				"claim_token": "clm_dyn",
			})
		}))
		defer srv.Close()

		projectID, _, err := CreateAnonymousProject(srv.URL, "my-project", "en", nil, "")
		require.NoError(t, err)
		assert.Equal(t, "proj_dyn", projectID)
	})

	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal error"}`))
		}))
		defer srv.Close()

		_, _, err := CreateAnonymousProject(srv.URL, "my-project", "en", []string{"nb"}, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 500")
	})

	t.Run("trailing slash in URL", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/projects/anonymous", r.URL.Path)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"project_id":  "proj_456",
				"claim_token": "clm_def789",
			})
		}))
		defer srv.Close()

		projectID, claimToken, err := CreateAnonymousProject(srv.URL+"/", "test", "en", []string{"de"}, "")
		require.NoError(t, err)
		assert.Equal(t, "proj_456", projectID)
		assert.Equal(t, "clm_def789", claimToken)
	})
}

func TestCreateAuthenticatedProject(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/projects", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "Bearer my-token", r.Header.Get("Authorization"))

			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "my-project", req["name"])
			assert.Equal(t, "en", req["source_locale"])

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id": "proj_auth_123",
			})
		}))
		defer srv.Close()

		projectID, err := CreateAuthenticatedProject(srv.URL, "my-token", "my-project", "en", nil)
		require.NoError(t, err)
		assert.Equal(t, "proj_auth_123", projectID)
	})

	t.Run("unauthorized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid token"}`))
		}))
		defer srv.Close()

		_, err := CreateAuthenticatedProject(srv.URL, "bad-token", "my-project", "en", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 401")
	})
}
