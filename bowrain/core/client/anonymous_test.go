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
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "my-project", req["name"])
			assert.Equal(t, "en", req["default_source_language"])
			targets := req["target_languages"].([]any)
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
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
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
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Nil(t, req["target_languages"], "empty targets should not be sent")

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
	// AD-011: authenticated projects are created under the workspace-scoped
	// collection (POST /api/v1/:ws/projects) — there is NO flat /api/v1/projects
	// create route. These tests assert that exact contract (the previous version
	// mocked the obsolete flat route, which hid a live 404 — see the integration
	// test in client_integration_test.go that runs against a real server).

	t.Run("resolves workspace then creates under the scoped route", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer my-token", r.Header.Get("Authorization"))
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces":
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode([]map[string]string{
					{"id": "ws1", "name": "My WS", "slug": "my-ws", "type": "personal"},
				})
			case r.Method == http.MethodPost && r.URL.Path == "/api/v1/my-ws/projects":
				var req map[string]any
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				assert.Equal(t, "my-project", req["name"])
				assert.Equal(t, "en", req["default_source_language"])
				_, hasWorkspace := req["workspace"]
				assert.False(t, hasWorkspace, "workspace belongs in the URL, not the body")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "proj_auth_123"})
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		projectID, wsSlug, err := CreateAuthenticatedProject(srv.URL, "my-token", "my-project", "en", nil, "")
		require.NoError(t, err)
		assert.Equal(t, "proj_auth_123", projectID)
		assert.Equal(t, "my-ws", wsSlug, "slug falls back to the resolved workspace when the response omits it")
	})

	t.Run("explicit workspace skips resolution and posts to the scoped route", func(t *testing.T) {
		listed := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/workspaces" {
				listed = true
			}
			assert.Equal(t, "/api/v1/team-ws/projects", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			var req map[string]any
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			_, hasWorkspace := req["workspace"]
			assert.False(t, hasWorkspace, "workspace belongs in the URL, not the body")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "proj_ws_456", "workspace_slug": "team-ws"})
		}))
		defer srv.Close()

		projectID, wsSlug, err := CreateAuthenticatedProject(srv.URL, "my-token", "my-project", "en", nil, "team-ws")
		require.NoError(t, err)
		assert.False(t, listed, "an explicit workspace must not trigger workspace resolution")
		assert.Equal(t, "proj_ws_456", projectID)
		assert.Equal(t, "team-ws", wsSlug)
	})

	t.Run("unauthorized create surfaces the HTTP status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid token"}`))
		}))
		defer srv.Close()

		_, _, err := CreateAuthenticatedProject(srv.URL, "bad-token", "my-project", "en", nil, "team-ws")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 401")
	})

	t.Run("no workspace available is a clear error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/workspaces", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]map[string]string{})
		}))
		defer srv.Close()

		_, _, err := CreateAuthenticatedProject(srv.URL, "my-token", "my-project", "en", nil, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no workspace available")
	})
}

func TestListWorkspaces(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/workspaces", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "Bearer my-token", r.Header.Get("Authorization"))

			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"id": "ws1", "name": "Personal", "slug": "personal", "type": "personal"},
				{"id": "ws2", "name": "Team", "slug": "team", "type": "team"},
			})
		}))
		defer srv.Close()

		workspaces, err := ListWorkspaces(srv.URL, "my-token")
		require.NoError(t, err)
		require.Len(t, workspaces, 2)
		assert.Equal(t, "personal", workspaces[0].Slug)
		assert.Equal(t, "team", workspaces[1].Slug)
	})

	t.Run("unauthorized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid token"}`))
		}))
		defer srv.Close()

		_, err := ListWorkspaces(srv.URL, "my-token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 401")
	})
}

func TestCreateWorkspace(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/workspaces", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "Bearer my-token", r.Header.Get("Authorization"))

			var req map[string]string
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "My Team", req["name"])
			assert.Equal(t, "my-team", req["slug"])

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id":   "ws_new",
				"name": "My Team",
				"slug": "my-team",
				"type": "team",
			})
		}))
		defer srv.Close()

		ws, err := CreateWorkspace(srv.URL, "my-token", "My Team", "my-team")
		require.NoError(t, err)
		assert.Equal(t, "my-team", ws.Slug)
		assert.Equal(t, "My Team", ws.Name)
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"slug already taken"}`))
		}))
		defer srv.Close()

		_, err := CreateWorkspace(srv.URL, "my-token", "My Team", "my-team")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 409")
	})
}
