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

// A merge body that will not parse must be refused, not read as
// "dry_run: false".
//
// The bind error used to be discarded, which left DryRun at its zero value and
// turned a request for a preview into a real merge. The caller got 200 and a
// result that looked like the preview they asked for, and the stream had
// already been merged.
//
// Echo binds an empty body without error, so a caller who sends no body at all
// still gets today's behaviour — a real merge, explicitly requested by
// omission. Only a body that was sent and could not be understood is refused.
func TestMergeStreamRefusesAnUnparseableBody(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()

	projectID := createTestProject(t, e, token, "Merge Guard")

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{
			name: "trailing comma",
			body: `{"dry_run": true,}`,
			want: http.StatusBadRequest,
		},
		{
			name: "dry_run is not a bool",
			body: `{"dry_run": "yes"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "well formed preview request",
			body: `{"dry_run": true}`,
			want: http.StatusOK,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/test/"+projectID+"/streams/main/merge", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if tc.want == http.StatusBadRequest {
				assert.Equal(t, http.StatusBadRequest, rec.Code,
					"a body that cannot be parsed must not be read as a real merge")
				return
			}
			// The merge itself may legitimately refuse main-into-parent; what
			// matters is that a parseable body is not rejected as malformed.
			assert.NotEqual(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}
}

// createTestProject creates a project through the real router and returns its
// id, so a test exercising another endpoint has something to address.
func createTestProject(t *testing.T, e http.Handler, token, name string) string {
	t.Helper()
	body := `{"name":"` + name + `","default_source_language":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID)
	return created.ID
}
