package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/forge"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGitHub serves the app-API surfaces the setup flow touches: token
// minting, the installation's repository list, and the git trees the detect
// endpoint reads. treeFiles maps "owner/name" to the blob paths its recursive
// tree returns.
func fakeGitHub(t *testing.T, treeFiles map[string][]string) *httptest.Server {
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
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/") && strings.Contains(r.URL.Path, "/git/trees/"):
			assert.Equal(t, "Bearer ghs_x", r.Header.Get("Authorization"))
			assert.Equal(t, "1", r.URL.Query().Get("recursive"))
			repo := strings.TrimPrefix(r.URL.Path, "/repos/")
			// The switch case above already guarantees "/git/trees/" is present
			// (strings.Contains), but use Cut instead of Index+slice so a
			// missing separator can never underflow into a bad slice bound.
			repo, _, _ = strings.Cut(repo, "/git/trees/")
			files, ok := treeFiles[repo]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message": "Not Found"}`))
				return
			}
			type entry struct {
				Path string `json:"path"`
				Type string `json:"type"`
			}
			tree := make([]entry, 0, len(files))
			for _, f := range files {
				tree = append(tree, entry{Path: f, Type: "blob"})
			}
			// A tree entry proves non-blob entries are skipped.
			tree = append(tree, entry{Path: "src", Type: "tree"})
			_ = json.NewEncoder(w).Encode(map[string]any{"tree": tree, "truncated": false})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
}

func setupServerWithApp(t *testing.T) *Server {
	return setupServerWithAppTrees(t, nil)
}

func setupServerWithAppTrees(t *testing.T, treeFiles map[string][]string) *Server {
	t.Helper()
	s, _ := newForgeTestServer(t, "conn1", "proj1", "s3cret")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	app, err := forge.NewGitHubApp("1", string(pemBytes), "app-secret")
	require.NoError(t, err)
	gh := fakeGitHub(t, treeFiles)
	t.Cleanup(gh.Close)
	app.SetAPIBase(gh.URL)
	s.GitHubApp = app

	// Installation 42 belongs to ws1 — the state every test in this file starts
	// from, and what setupCtx/detectCtx address. Without the record the setup
	// endpoints answer 404, which is the point of the record.
	_, err = s.ForgeInstallationStore.Claim(context.Background(), 42, "ws1")
	require.NoError(t, err)
	return s
}

func setupCtx(t *testing.T, method, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	return setupCtxFor(t, method, body, "42")
}

// setupCtxFor is setupCtx with the addressed installation spelled out, so a
// test can aim a ws1 caller at an installation ws1 does not own.
func setupCtxFor(t *testing.T, method, body, installationID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	target := "/github/installations/" + installationID + "/repositories"
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, bytes.NewReader([]byte(body)))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.Set("workspace_id", "ws1")
	c.Set("workspace_role", platauth.RoleOwner)
	c.Set("project_permissions", platauth.PermManageConnectors)
	c.SetParamNames("installationID")
	c.SetParamValues(installationID)
	return c, rec
}

// detectCtx builds a request context for the detect endpoint with its
// :owner/:name params (and optional query string, e.g. "scope=apps/web").
func detectCtx(t *testing.T, owner, name, query string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	return detectCtxFor(t, "42", owner, name, query)
}

// detectCtxFor is detectCtx with the addressed installation spelled out.
func detectCtxFor(t *testing.T, installationID, owner, name, query string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	target := "/github/installations/" + installationID + "/repositories/" + owner + "/" + name + "/detect"
	if query != "" {
		target += "?" + query
	}
	r := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.Set("workspace_id", "ws1")
	c.Set("workspace_role", platauth.RoleOwner)
	c.Set("project_permissions", platauth.PermManageConnectors)
	c.SetParamNames("installationID", "owner", "name")
	c.SetParamValues(installationID, owner, name)
	return c, rec
}

// An installation is reachable only from the workspace that installed it.
//
// The id in the URL is GitHub's, not ours: one registered app serves every
// workspace, and its JWT can mint an access token for any installation of that
// app. So each setup endpoint asks the ownership record first, and an
// installation ws1 has not claimed answers 404 — the same answer an id nobody
// has ever installed gets, so the endpoints cannot be used to find out which
// ids are real.
//
// fakeGitHub t.Errorf's on any request it does not expect, so these cases also
// prove the denial happens BEFORE the app API is touched: a 404 that still
// minted a token would have leaked the repository list into the server.
func TestInstallationSetup_ForeignInstallationIsNotFound(t *testing.T) {
	s := setupServerWithAppTrees(t, map[string][]string{"acme/site": {"README.md"}})

	// ws2's installation, recorded and claimed by ws2. Everything below is ws1
	// asking for it.
	_, err := s.ForgeInstallationStore.Claim(t.Context(), 99, "ws2")
	require.NoError(t, err)

	tests := []struct {
		name    string
		invoke  func(t *testing.T) (*httptest.ResponseRecorder, error)
		message string
	}{
		{
			name: "list an installation owned by another workspace",
			invoke: func(t *testing.T) (*httptest.ResponseRecorder, error) {
				c, rec := setupCtxFor(t, http.MethodGet, "", "99")
				return rec, s.HandleListInstallationRepos(c)
			},
			message: "listing another workspace's installation must not enumerate its repositories",
		},
		{
			name: "detect inside an installation owned by another workspace",
			invoke: func(t *testing.T) (*httptest.ResponseRecorder, error) {
				c, rec := detectCtxFor(t, "99", "acme", "site", "")
				return rec, s.HandleDetectInstallationRepo(c)
			},
			message: "detecting in another workspace's installation must not read its file tree",
		},
		{
			name: "bind a repository from an installation owned by another workspace",
			invoke: func(t *testing.T) (*httptest.ResponseRecorder, error) {
				c, rec := setupCtxFor(t, http.MethodPost,
					`{"repository": "acme/site", "project_id": "proj1"}`, "99")
				return rec, s.HandleBindInstallationRepo(c)
			},
			message: "binding from another workspace's installation must not clone its repository",
		},
		{
			name: "an installation nobody has claimed",
			invoke: func(t *testing.T) (*httptest.ResponseRecorder, error) {
				c, rec := setupCtxFor(t, http.MethodGet, "", "1234")
				return rec, s.HandleListInstallationRepos(c)
			},
			message: "an unclaimed installation belongs to no workspace",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, err := tc.invoke(t)
			// The gate writes the response and returns errAccessDenied, so the
			// handler aborts. Returning nil here would let a caller's
			// `if err != nil` fall through and run the body anyway.
			require.ErrorIs(t, err, errAccessDenied, "the gate must abort the handler")
			assert.Equal(t, http.StatusNotFound, rec.Code, tc.message)
			var envelope struct {
				Error string `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
			assert.Equal(t, "installation not found", envelope.Error,
				"the refusal must read the same as a genuinely unknown installation")
		})
	}

	// Nothing was bound as a side effect of the refused bind.
	configs, err := s.ConnectorConfigStore.List(t.Context(), "ws1")
	require.NoError(t, err)
	assert.Len(t, configs, 1, "only the harness's own connector may exist")
}

// The claim is what makes an installation a workspace's own, and the signed
// state — not the installation id — is what earns it.
func TestClaimInstallation(t *testing.T) {
	s := setupServerWithApp(t)
	ctx := t.Context()

	claim := func(t *testing.T, wsID, installationID, state string) *httptest.ResponseRecorder {
		t.Helper()
		body := `{"state": ` + strconv.Quote(state) + `}`
		r := httptest.NewRequest(http.MethodPost,
			"/github/installations/"+installationID+"/claim", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(r, rec)
		c.Set("workspace_id", wsID)
		c.Set("workspace_role", platauth.RoleOwner)
		c.Set("project_permissions", platauth.PermManageConnectors)
		c.SetParamNames("installationID")
		c.SetParamValues(installationID)
		require.NoError(t, s.HandleClaimInstallation(c))
		return rec
	}

	ws1State, err := platauth.GenerateSetupState("ws1", "test-secret", time.Hour)
	require.NoError(t, err)

	// The webhook recorded installation 77; nobody owns it yet.
	require.NoError(t, s.ForgeInstallationStore.Record(ctx, 77, "acme"))
	owned, err := s.ForgeInstallationStore.OwnedBy(ctx, 77, "ws1")
	require.NoError(t, err)
	assert.False(t, owned, "a recorded installation is owned by nobody until claimed")

	t.Run("valid state claims the installation", func(t *testing.T) {
		rec := claim(t, "ws1", "77", ws1State)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		owned, err := s.ForgeInstallationStore.OwnedBy(ctx, 77, "ws1")
		require.NoError(t, err)
		assert.True(t, owned)
	})

	t.Run("re-claiming from the owning workspace is idempotent", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, claim(t, "ws1", "77", ws1State).Code,
			"re-running the install flow must land back on the same installation")
	})

	t.Run("another workspace cannot take a claimed installation", func(t *testing.T) {
		ws2State, err := platauth.GenerateSetupState("ws2", "test-secret", time.Hour)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, claim(t, "ws2", "77", ws2State).Code,
			"first claim wins; a second workspace gets the not-found answer")
		owned, err := s.ForgeInstallationStore.OwnedBy(ctx, 77, "ws1")
		require.NoError(t, err)
		assert.True(t, owned, "the original owner must keep the installation")
	})

	t.Run("state minted for another workspace is refused", func(t *testing.T) {
		otherState, err := platauth.GenerateSetupState("ws2", "test-secret", time.Hour)
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, claim(t, "ws1", "88", otherState).Code,
			"state names the workspace it was minted for; ws1 cannot spend ws2's")
	})

	t.Run("expired state is refused", func(t *testing.T) {
		expired, err := platauth.GenerateSetupState("ws1", "test-secret", -time.Minute)
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, claim(t, "ws1", "88", expired).Code)
	})

	t.Run("a session token is not setup state", func(t *testing.T) {
		// The audience split: a token minted for the API can never be spent as
		// setup state, however valid its signature.
		session, err := platauth.GenerateToken(
			&platauth.User{ID: "u1", Email: "u@example.com"}, "test-secret", time.Hour)
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, claim(t, "ws1", "88", session).Code)
	})

	t.Run("missing state is refused", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, claim(t, "ws1", "88", "").Code)
	})

	// None of the refusals left a row behind.
	_, err = s.ForgeInstallationStore.Get(ctx, 88)
	require.ErrorIs(t, err, bstore.ErrForgeInstallationNotFound,
		"a refused claim must not record the installation")
}

// The state endpoint mints state for the caller's own workspace only.
func TestGitHubSetupState(t *testing.T) {
	s := setupServerWithApp(t)

	r := httptest.NewRequest(http.MethodGet, "/github/setup-state", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.Set("workspace_id", "ws1")
	c.Set("workspace_role", platauth.RoleOwner)
	c.Set("project_permissions", platauth.PermManageConnectors)
	require.NoError(t, s.HandleGitHubSetupState(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out struct {
		State     string `json:"state"`
		ExpiresIn int    `json:"expires_in"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotEmpty(t, out.State)
	assert.Positive(t, out.ExpiresIn)

	wsID, err := platauth.ValidateSetupState(out.State, "test-secret")
	require.NoError(t, err)
	assert.Equal(t, "ws1", wsID, "state carries the minting workspace, not the caller's choice")

	// And it is not a session token: the audience check rejects it.
	_, err = platauth.ValidateToken(out.State, "test-secret")
	require.Error(t, err, "setup state must never pass as an API session token")
}

// A project in another workspace cannot be bound to, even from an installation
// this workspace legitimately owns. The project id arrives in the request BODY,
// which the path-based cross-tenant middleware never inspects.
func TestBindInstallationRepo_ForeignProjectIsNotFound(t *testing.T) {
	s := setupServerWithApp(t)

	require.NoError(t, s.ContentStore.CreateProject(t.Context(), &platstore.Project{
		ID: "ws2-proj", Name: "Theirs", DefaultSourceLanguage: "en", WorkspaceID: "ws2",
	}))

	c, rec := setupCtx(t, http.MethodPost, `{"repository": "acme/docs", "project_id": "ws2-proj"}`)
	require.NoError(t, s.HandleBindInstallationRepo(c))
	assert.Equal(t, http.StatusNotFound, rec.Code,
		"binding into another workspace's project must be refused")

	configs, err := s.ConnectorConfigStore.List(t.Context(), "ws1")
	require.NoError(t, err)
	assert.Len(t, configs, 1, "the refused bind must not have created a connector")
}

// The detect endpoint reads the repository tree through the app API (no
// clone) and returns the wizard's detection: monorepo + i18next catalogs
// with a scoped proposal and a match preview.
func TestDetectInstallationRepo_MonorepoI18next(t *testing.T) {
	s := setupServerWithAppTrees(t, map[string][]string{
		"acme/site": {
			"pnpm-workspace.yaml",
			"package.json",
			"apps/web/package.json",
			"apps/web/public/locales/en/common.json",
			"apps/web/public/locales/fr/common.json",
			"packages/ui/package.json",
		},
	})

	c, rec := detectCtx(t, "acme", "site", "")
	require.NoError(t, s.HandleDetectInstallationRepo(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var det forge.RepoDetection
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &det))
	assert.Equal(t, []string{"pnpm-workspace.yaml"}, det.MonorepoMarkers)
	assert.Contains(t, det.Workspaces, "apps/web")
	require.Len(t, det.Signals, 1)
	assert.Equal(t, "react-i18next", det.Signals[0].ID)
	assert.Equal(t, "apps/web/public/locales", det.Signals[0].Dir)
	assert.Equal(t, "apps/web/public/locales/**/*.json", det.ProposedPatterns)
	assert.Equal(t, 2, det.MatchCount)
	assert.Len(t, det.MatchPreview, 2)
	assert.False(t, det.Truncated)
}

// A docs repository detects as a Docusaurus site; a scope query narrows a
// monorepo detection; a patterns override drives the match count.
func TestDetectInstallationRepo_DocsAndQueries(t *testing.T) {
	s := setupServerWithAppTrees(t, map[string][]string{
		"acme/docs": {
			"docusaurus.config.ts",
			"docs/intro.md",
			"docs/guide/setup.mdx",
			"README.md",
		},
		"acme/site": {
			"pnpm-workspace.yaml",
			"apps/web/public/locales/en/common.json",
			"apps/web/package.json",
			"apps/api/package.json",
			"apps/api/main.go",
		},
	})

	c, rec := detectCtx(t, "acme", "docs", "")
	require.NoError(t, s.HandleDetectInstallationRepo(c))
	var det forge.RepoDetection
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &det))
	require.Len(t, det.Signals, 1)
	assert.Equal(t, "docusaurus", det.Signals[0].ID)
	assert.Equal(t, 2, det.MatchCount)

	// Scope: detection confined to one workspace. No catalog signal exists
	// under apps/api, so the proposal falls back to the defaults scoped to the
	// prefix — which the preview then honestly shows sweeping up the JSON
	// manifest (the user edits the patterns from there).
	c, rec = detectCtx(t, "acme", "site", "scope=apps/api")
	require.NoError(t, s.HandleDetectInstallationRepo(c))
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &det))
	assert.Empty(t, det.Signals)
	assert.True(t, strings.HasPrefix(det.ProposedPatterns, "apps/api/"))
	assert.Equal(t, []string{"apps/api/package.json"}, det.MatchPreview)

	// Patterns override: the wizard's live match feedback for edited globs.
	c, rec = detectCtx(t, "acme", "docs", "patterns="+"README.md")
	require.NoError(t, s.HandleDetectInstallationRepo(c))
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &det))
	assert.Equal(t, 1, det.MatchCount)
	assert.Equal(t, []string{"README.md"}, det.MatchPreview)
}

// An empty repository yields no signals and zero matches — what the wizard
// turns into its zero-match warning. An unknown repository is answered with a
// 502 naming the forge as the thing that did not answer, rather than a
// fabricated detection — and without GitHub's own reply, which can run to four
// kilobytes and says nothing the wizard's user can act on.
func TestDetectInstallationRepo_EmptyAndUnknown(t *testing.T) {
	s := setupServerWithAppTrees(t, map[string][]string{
		"acme/empty": {"main.go", "go.sum"},
	})

	c, rec := detectCtx(t, "acme", "empty", "")
	require.NoError(t, s.HandleDetectInstallationRepo(c))
	var det forge.RepoDetection
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &det))
	assert.Empty(t, det.Signals)
	assert.Zero(t, det.MatchCount)
	assert.NotEmpty(t, det.ProposedPatterns, "the defaults stay editable even with nothing to match")

	c, rec = detectCtx(t, "acme", "unknown", "")
	require.NoError(t, s.HandleDetectInstallationRepo(c))
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	var envelope struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.Contains(t, envelope.Error, "the forge did not answer",
		"the wizard is told which side failed")
	assert.NotContains(t, rec.Body.String(), "404",
		"GitHub's own reply stays in the log, not the response")
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
	ingested := make(chan platev.Event, 1)
	s.EventBus.Subscribe(platev.EventPushCompleted, func(ev platev.Event) { ingested <- ev })

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

	// Drain the background initial ingest the successful bind started, so the
	// test's stores outlive it.
	select {
	case <-ingested:
	case <-time.After(5 * time.Second):
		t.Fatal("no EventPushCompleted after a successful bind")
	}
}

func TestBindInstallationRepo_TriggersInitialFetch(t *testing.T) {
	s := setupServerWithApp(t)

	ingested := make(chan platev.Event, 1)
	s.EventBus.Subscribe(platev.EventPushCompleted, func(ev platev.Event) { ingested <- ev })

	c, rec := setupCtx(t, http.MethodPost, `{"repository": "acme/docs", "project_id": "proj1"}`)
	require.NoError(t, s.HandleBindInstallationRepo(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var out map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "started", out["initial_fetch"], "the bind response announces the initial fetch")

	// The bind kicks the same ingest a push webhook does: fetch, then
	// EventPushCompleted — which is what starts the first convergence run.
	select {
	case ev := <-ingested:
		assert.Equal(t, "proj1", ev.ProjectID)
		assert.Equal(t, "github-setup", ev.Source)
		assert.Equal(t, "acme/docs", ev.Data["repo"])
		assert.Equal(t, "trunk", ev.Data["branch"])
		assert.Equal(t, out["connector_id"], ev.Data["connector_id"])
	case <-time.After(5 * time.Second):
		t.Fatal("no EventPushCompleted after a successful bind")
	}

	// The fetched content landed in the project, and the connector reports a
	// real first sync with no error.
	blocks, err := s.ContentStore.GetBlocks(context.Background(), platstore.BlockQuery{
		ProjectID: "proj1", Stream: "main",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, blocks, "the initial fetch must ingest the repository's content")
	cfg, err := s.ConnectorConfigStore.Get(context.Background(), "ws1", out["connector_id"])
	require.NoError(t, err)
	assert.False(t, cfg.LastSyncAt.IsZero(), "the initial fetch stamps the first sync")
	assert.Empty(t, cfg.LastError)
}

func TestBindInstallationRepo_FetchFailureKeepsBinding(t *testing.T) {
	s := setupServerWithApp(t)

	// The bound repository's first fetch fails with the production shape from
	// the founder drill: the clone breaks (exit status 128). Crucially, the
	// real forge connector's Status() re-runs the same clone, so the status
	// probe fails with the same error on every poll — the stub mirrors that.
	cloneErr := errors.New("git clone: fatal: could not read Username for 'https://github.com': terminal prompts disabled\n: exit status 128")
	s.ConnectorReg.Register("forge", platconn.CategoryCode, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return &stubForgeConnector{id: "forge-" + config["name"], fetchErr: cloneErr, statusErr: cloneErr}, nil
	})
	pushed := make(chan platev.Event, 1)
	s.EventBus.Subscribe(platev.EventPushCompleted, func(ev platev.Event) { pushed <- ev })

	c, rec := setupCtx(t, http.MethodPost, `{"repository": "acme/docs", "project_id": "proj1"}`)
	require.NoError(t, s.HandleBindInstallationRepo(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String(), "a fetch failure must not fail the bind")

	var out map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	connID := out["connector_id"]

	// The failure is recorded on the connector row — never silent.
	require.Eventually(t, func() bool {
		cfg, err := s.ConnectorConfigStore.Get(context.Background(), "ws1", connID)
		return err == nil && cfg.LastError != ""
	}, 5*time.Second, 10*time.Millisecond, "the failed initial fetch must record a last error")

	// The binding survives: the row is still there, the live instance is still
	// addressable, and no last-sync was fabricated.
	cfg, err := s.ConnectorConfigStore.Get(context.Background(), "ws1", connID)
	require.NoError(t, err)
	assert.Contains(t, cfg.LastError, "exit status 128")
	assert.True(t, cfg.LastSyncAt.IsZero(), "a failed fetch is not a sync")
	_, err = s.Services.Connector.GetConnector("ws1", connID)
	require.NoError(t, err, "the live connector stays registered")

	// No push event: a failed ingest must not start a convergence run.
	select {
	case <-pushed:
		t.Fatal("a failed initial fetch must not publish EventPushCompleted")
	case <-time.After(100 * time.Millisecond):
	}

	// The status endpoint — what the connectors panel and the setup page poll —
	// surfaces the recorded error. This is the founder-drill regression: this
	// used to 404, so the wizard's poll never saw a status body and showed
	// "Importing your repo…" forever. Both reads must agree:
	//   - the DEFAULT (cheap) read serves the stored row without touching the
	//     connector at all — the wizard's 2s poll must never re-clone (#1362);
	//   - the DEEP read (?probe=1, the panel's manual path) degrades to the
	//     same stored state when the live Status() probe fails on the broken
	//     clone.
	for _, target := range []string{"/", "/?probe=1"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		statusRec := httptest.NewRecorder()
		sc := echo.New().NewContext(req, statusRec)
		sc.Set("workspace_id", "ws1")
		sc.SetParamNames("id")
		sc.SetParamValues(connID)
		require.NoError(t, s.HandleConnectorStatus(sc))
		require.Equal(t, http.StatusOK, statusRec.Code, statusRec.Body.String())
		var status struct {
			LastSync time.Time `json:"LastSync"`
			Errors   []string  `json:"Errors"`
		}
		require.NoError(t, json.Unmarshal(statusRec.Body.Bytes(), &status))
		require.NotEmpty(t, status.Errors, "read %s must carry the recorded error", target)
		assert.Contains(t, status.Errors[0], "exit status 128")
		assert.True(t, status.LastSync.IsZero(), "no sync is fabricated for a never-synced connector")
	}
}
