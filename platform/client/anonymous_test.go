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
			json.NewEncoder(w).Encode(map[string]string{
				"project_id":  "proj_123",
				"claim_token": "clm_abc456",
			})
		}))
		defer srv.Close()

		projectID, claimToken, err := CreateAnonymousProject(srv.URL, "my-project", "en", []string{"nb", "fr"})
		require.NoError(t, err)
		assert.Equal(t, "proj_123", projectID)
		assert.Equal(t, "clm_abc456", claimToken)
	})

	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal error"}`))
		}))
		defer srv.Close()

		_, _, err := CreateAnonymousProject(srv.URL, "my-project", "en", []string{"nb"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 500")
	})

	t.Run("trailing slash in URL", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/projects/anonymous", r.URL.Path)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"project_id":  "proj_456",
				"claim_token": "clm_def789",
			})
		}))
		defer srv.Close()

		projectID, claimToken, err := CreateAnonymousProject(srv.URL+"/", "test", "en", []string{"de"})
		require.NoError(t, err)
		assert.Equal(t, "proj_456", projectID)
		assert.Equal(t, "clm_def789", claimToken)
	})
}
