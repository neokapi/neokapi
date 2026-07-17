package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/forge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGitHub serves the two app-API surfaces the setup flow touches: token
// minting and the installation's repository list.
func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/42/access_tokens":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token": "ghs_x", "expires_at": "2099-01-01T00:00:00Z"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			assert.Equal(t, "Bearer ghs_x", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"total_count": 2, "repositories": [
				{"full_name": "acme/site", "default_branch": "main", "private": true},
				{"full_name": "acme/docs", "default_branch": "trunk", "private": false}
			]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
}

func setupServerWithApp(t *testing.T) *Server {
	t.Helper()
	s, _ := newForgeTestServer(t, "conn1", "proj1", "s3cret")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	app, err := forge.NewGitHubApp("1", string(pemBytes), "app-secret")
	require.NoError(t, err)
	gh := fakeGitHub(t)
	t.Cleanup(gh.Close)
	app.SetAPIBase(gh.URL)
	s.GitHubApp = app
	return s
}

func setupCtx(t *testing.T, method, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, "/github/installations/42/repositories", nil)
	} else {
		r = httptest.NewRequest(method, "/github/installations/42/repositories", bytes.NewReader([]byte(body)))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.Set("workspace_id", "ws1")
	c.Set("workspace_role", platauth.RoleOwner)
	c.Set("project_permissions", platauth.PermManageConnectors)
	c.SetParamNames("installationID")
	c.SetParamValues("42")
	return c, rec
}

func TestListInstallationRepos_AnnotatesBindings(t *testing.T) {
	s := setupServerWithApp(t)

	// The harness persisted a forge config for acme/site — the list must show
	// it as bound while acme/docs stays free.
	c, rec := setupCtx(t, http.MethodGet, "")
	require.NoError(t, s.HandleListInstallationRepos(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var repos []InstallationRepoInfo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &repos))
	require.Len(t, repos, 2)
	assert.Equal(t, "acme/site", repos[0].FullName)
	assert.Equal(t, "conn1", repos[0].ConnectorID)
	assert.Equal(t, "proj1", repos[0].ProjectID)
	assert.Equal(t, "acme/docs", repos[1].FullName)
	assert.Empty(t, repos[1].ConnectorID)
	assert.Equal(t, "trunk", repos[1].DefaultBranch)
}

func TestBindInstallationRepo_CreatesPersistedConnector(t *testing.T) {
	s := setupServerWithApp(t)

	c, rec := setupCtx(t, http.MethodPost, `{"repository": "acme/docs", "project_id": "proj1", "patterns": "docs/**/*.md"}`)
	require.NoError(t, s.HandleBindInstallationRepo(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var out map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "acme/docs", out["repository"])
	assert.Equal(t, "trunk", out["branch"], "defaults to the repo's default branch")

	// Persisted with the app-auth config, no credentials of its own.
	cfg, err := s.ConnectorConfigStore.Get(context.Background(), "ws1", out["connector_id"])
	require.NoError(t, err)
	assert.Equal(t, "forge", cfg.Type)
	assert.Equal(t, "app", cfg.Config["auth"])
	assert.Equal(t, "https://github.com/acme/docs.git", cfg.Config["repo"])
	assert.Equal(t, "docs/**/*.md", cfg.Config["patterns"])
	assert.Empty(t, cfg.Config["token"])

	// A repository outside the installation is refused — the claim is checked
	// against GitHub, not the request.
	c, rec = setupCtx(t, http.MethodPost, `{"repository": "acme/other", "project_id": "proj1"}`)
	require.NoError(t, s.HandleBindInstallationRepo(c))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
