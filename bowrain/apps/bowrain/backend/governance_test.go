package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedRequest captures what the proxy actually sent to the server.
type recordedRequest struct {
	method string
	path   string
	query  string
	auth   string
	body   string
}

// newGovTestApp wires an App to a test HTTP server as its bowrain server,
// in the connected state with a cached token, and returns the App plus a
// pointer that receives the last request the handler observed.
func newGovTestApp(t *testing.T, handler http.HandlerFunc) (*App, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method = r.Method
		rec.path = r.URL.Path
		rec.query = r.URL.RawQuery
		rec.auth = r.Header.Get("Authorization")
		if r.Body != nil {
			buf := make([]byte, r.ContentLength)
			if r.ContentLength > 0 {
				_, _ = r.Body.Read(buf)
			}
			rec.body = string(buf)
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	app := newTestApp(t)
	// A lazily-dialing gRPC client satisfies isConnected()'s remote != nil
	// check without any real network use; the proxy uses plain HTTP, not gRPC.
	client, err := NewServerClient("127.0.0.1:1", "tok-xyz", false)
	require.NoError(t, err)

	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = srv.URL
	app.activeWS = "acme"
	app.remote = client
	app.authInfo = &storedDesktopAuth{ServerURL: srv.URL, AccessToken: "tok-xyz"}
	app.mu.Unlock()

	return app, rec
}

func TestGovernanceNotConnected(t *testing.T) {
	app := newTestApp(t)

	_, err := app.ListMembers("acme")
	require.ErrorIs(t, err, errNotConnected)

	err = app.AddMember("acme", "u1", "member")
	require.ErrorIs(t, err, errNotConnected)

	_, err = app.GetSuggestedRules("acme", "p1", 0, false)
	require.ErrorIs(t, err, errNotConnected)
}

func TestListMembers(t *testing.T) {
	app, rec := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]MemberInfo{
			{UserID: "u1", Role: "owner"},
			{UserID: "u2", Role: "member"},
		})
	})

	members, err := app.ListMembers("acme")
	require.NoError(t, err)
	require.Len(t, members, 2)
	assert.Equal(t, "u1", members[0].UserID)
	assert.Equal(t, "owner", members[0].Role)

	assert.Equal(t, http.MethodGet, rec.method)
	assert.Equal(t, "/api/v1/acme/members", rec.path)
	assert.Equal(t, "Bearer tok-xyz", rec.auth)
}

func TestMemberMutations(t *testing.T) {
	tests := []struct {
		name       string
		call       func(a *App) error
		wantMethod string
		wantPath   string
		wantBody   string // substrings expected in the JSON body, "" = no body
	}{
		{
			name:       "add",
			call:       func(a *App) error { return a.AddMember("acme", "u9", "admin") },
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/members",
			wantBody:   `"user_id":"u9"`,
		},
		{
			name:       "update role",
			call:       func(a *App) error { return a.UpdateMemberRole("acme", "u9", "viewer") },
			wantMethod: http.MethodPut,
			wantPath:   "/api/v1/acme/members/u9/role",
			wantBody:   `"role":"viewer"`,
		},
		{
			name:       "remove",
			call:       func(a *App) error { return a.RemoveMember("acme", "u9") },
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/acme/members/u9",
			wantBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, rec := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			require.NoError(t, tt.call(app))
			assert.Equal(t, tt.wantMethod, rec.method)
			assert.Equal(t, tt.wantPath, rec.path)
			if tt.wantBody != "" {
				assert.Contains(t, rec.body, tt.wantBody)
			}
		})
	}
}

func TestBrandGovernanceRoutes(t *testing.T) {
	tests := []struct {
		name       string
		call       func(a *App) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{
			name: "list candidates with filters",
			call: func(a *App) error {
				_, err := a.GetSuggestedRules("acme", "prof-1", 3, true)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/brand-profiles/prof-1/candidates",
			wantQuery:  "all=true&min_count=3",
		},
		{
			name: "promote rule",
			call: func(a *App) error {
				_, err := a.PromoteRule("acme", "prof-1", CandidateRuleArgs{Term: "utilize", Replacement: "use", CorrectionCount: 5})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/brand-profiles/prof-1/promote-rule",
			wantBody:   `"term":"utilize"`,
		},
		{
			name: "reject rule",
			call: func(a *App) error {
				return a.RejectRule("acme", "prof-1", CandidateRuleArgs{Term: "leverage"})
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/brand-profiles/prof-1/reject-rule",
			wantBody:   `"term":"leverage"`,
		},
		{
			name: "evaluate rule",
			call: func(a *App) error {
				_, err := a.EvaluateRule("acme", "prof-1", EvaluateRuleArgs{Term: "synergy", ProjectID: "proj-7"})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/acme/brand-profiles/prof-1/evaluate-rule",
			wantBody:   `"project_id":"proj-7"`,
		},
		{
			name: "drift",
			call: func(a *App) error {
				_, err := a.GetBrandDrift("acme", "proj-7", 30, 0, 0)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/proj-7/brand-voice/main/drift",
			wantQuery:  "recent_days=30",
		},
		{
			name: "scores",
			call: func(a *App) error {
				_, err := a.GetBrandScores("acme", "proj-7")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/proj-7/brand-voice/main/scores",
		},
		{
			name: "list profiles",
			call: func(a *App) error {
				_, err := a.ListBrandProfiles("acme")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/acme/brand-profiles",
		},
		{
			name: "starter packs",
			call: func(a *App) error {
				_, err := a.ListStarterPacks()
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/brand-voice/starter-packs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, rec := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
				// Return a benign JSON value all the typed/raw decoders accept.
				_, _ = w.Write([]byte(`{}`))
			})
			require.NoError(t, tt.call(app))
			assert.Equal(t, tt.wantMethod, rec.method)
			assert.Equal(t, tt.wantPath, rec.path)
			if tt.wantQuery != "" {
				assert.Equal(t, tt.wantQuery, rec.query)
			}
			if tt.wantBody != "" {
				assert.Contains(t, rec.body, tt.wantBody)
			}
			assert.Equal(t, "Bearer tok-xyz", rec.auth)
		})
	}
}

func TestGovernanceServerError(t *testing.T) {
	app, _ := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	})

	_, err := app.ListMembers("acme")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestGovernanceReturnsDecodedJSON(t *testing.T) {
	app, _ := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"term":"utilize","replacement":"use","correction_count":4,"dimension":"vocabulary","status":"pending"}]`))
	})

	raw, err := app.GetSuggestedRules("acme", "prof-1", 0, false)
	require.NoError(t, err)

	var candidates []map[string]any
	require.NoError(t, json.Unmarshal(raw, &candidates))
	require.Len(t, candidates, 1)
	assert.Equal(t, "utilize", candidates[0]["term"])
	assert.Equal(t, "pending", candidates[0]["status"])
}
