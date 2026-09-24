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

// TestExportTranslatedFileIsNotImplemented verifies that unsupported server-side
// merged-file export returns a specific non-5xx response and directs the user to
// kapi pull. Item rows do not retain source bytes for a merged-file export.
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
