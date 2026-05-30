package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func foTMCount(t *testing.T, s *Server, token string) int {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/test/translation-memory/count", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, r)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Count int `json:"count"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out.Count
}

// TestFailOpenCheck verifies a denied mutation does NOT take effect (not just
// that the status is 403). Guards against the c.JSON-returns-nil fail-open in
// requirePermission.
func TestFailOpenCheck(t *testing.T) {
	s, ownerToken := newTestServer(t)
	memberToken := addWorkspaceMember(t, s, "fo-mem", "fo-mem@example.com", platauth.RoleMember)

	cntBefore := foTMCount(t, s, ownerToken)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/test/translation-memory", strings.NewReader(tmBody))
	r.Header.Set("Authorization", "Bearer "+memberToken)
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, r)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	cntAfter := foTMCount(t, s, ownerToken)
	assert.Equal(t, cntBefore, cntAfter, "denied POST must NOT create a TM entry (fail-open check)")
}
