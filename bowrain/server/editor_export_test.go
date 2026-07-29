package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExportTranslatedFileIsNotImplemented pins the export contract.
//
// Server-side merged-file export has not existed since #136 — source bytes are
// no longer stored in the Item model, so there is nothing to merge — and
// `kapi pull` is the export path. The handler used to express that by returning
// a stub error through serverErr, which rendered a 500: the web app showed "the
// server hit an unexpected error, try again in a moment" for a condition that
// is permanent, and the sentence naming the alternative never left the server
// log. The editor e2e spec asserted on that sentence and had no way to see it.
//
// The status must stay non-5xx-generic and the body must keep naming the CLI.
func TestExportTranslatedFileIsNotImplemented(t *testing.T) {
	s, token := newTestServer(t)
	pid := createTestProject(t, s.GetEcho(), token, "Export Contract")

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/test/"+pid+"/actions/main/export?item=en.json",
		strings.NewReader(`{"target_locale":"fr"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotImplemented, rec.Code,
		"a permanent refusal must not masquerade as an unexpected server error")

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp.Error, "kapi pull",
		"the refusal must name the export path that does work")
}
