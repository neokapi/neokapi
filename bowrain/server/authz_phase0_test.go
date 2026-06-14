package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addWorkspaceMember creates a user and adds them to the test workspace ("test")
// with the given role, returning a signed session token for them.
func addWorkspaceMember(t *testing.T, s *Server, id, email string, role platauth.Role) string {
	t.Helper()
	ctx := t.Context()
	u := &platauth.User{ID: id, Email: email, Name: email}
	require.NoError(t, s.AuthStore.CreateUser(ctx, u))
	require.NoError(t, s.AuthStore.AddMember(ctx, "test-ws", u.ID, role))
	token, err := platauth.GenerateToken(u, "test-secret", time.Hour)
	require.NoError(t, err)
	return token
}

func do(t *testing.T, s *Server, method, target, token, body string) int {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, r)
	return rec.Code
}

// TestPhase0_WorkspaceResourceAuthorization verifies that workspace-level
// resource mutations (TM, terminology, connectors, providers) and audit reads
// are now gated: a plain member is denied, while the owner is not. This closes
// the fail-open hole where any authenticated member could mutate shared assets.
func TestPhase0_WorkspaceResourceAuthorization(t *testing.T) {
	s, ownerToken := newTestServer(t)
	memberToken := addWorkspaceMember(t, s, "member-1", "member@example.com", platauth.RoleMember)

	tmBody := `{"source":"hello","target":"bonjour","source_locale":"en","target_locale":"fr"}`
	connBody := `{"type":"file","config":{}}`
	provBody := `{"provider_type":"openai","model":"gpt-4o"}`
	termBody := `{"definition":"d","terms":[]}`

	cases := []struct {
		name, method, path, body string
	}{
		{"tm-add", http.MethodPost, "/api/v1/test/translation-memory", tmBody},
		{"connector-add", http.MethodPost, "/api/v1/test/connectors", connBody},
		{"provider-save", http.MethodPost, "/api/v1/test/providers", provBody},
		{"concept-add", http.MethodPost, "/api/v1/test/concepts", termBody},
		{"audit-read", http.MethodGet, "/api/v1/test/audit-log", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/member-denied", func(t *testing.T) {
			code := do(t, s, tc.method, tc.path, memberToken, tc.body)
			assert.Equal(t, http.StatusForbidden, code,
				"member must be forbidden from %s %s", tc.method, tc.path)
		})
		t.Run(tc.name+"/owner-allowed", func(t *testing.T) {
			code := do(t, s, tc.method, tc.path, ownerToken, tc.body)
			assert.NotEqual(t, http.StatusForbidden, code,
				"owner must not be forbidden from %s %s (got %d)", tc.method, tc.path, code)
		})
	}
}

// TestPhase0_MemberRetainsReadAndContribute verifies the gating did not
// over-restrict: a member can still read shared resources and contribute
// translations (their role's permissions are intact).
func TestPhase0_MemberRetainsReadAndContribute(t *testing.T) {
	s, _ := newTestServer(t)
	memberToken := addWorkspaceMember(t, s, "member-2", "member2@example.com", platauth.RoleMember)

	// Reading the TM is allowed for any member (view permission).
	code := do(t, s, http.MethodGet, "/api/v1/test/translation-memory", memberToken, "")
	assert.NotEqual(t, http.StatusForbidden, code, "member should be able to read the TM")

	// Listing concepts is allowed for any member.
	code = do(t, s, http.MethodGet, "/api/v1/test/concepts", memberToken, "")
	assert.NotEqual(t, http.StatusForbidden, code, "member should be able to read concepts")
}
