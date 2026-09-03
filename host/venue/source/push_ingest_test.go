package source

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusTestServer serves the push-status endpoint with a fixed verdict, plus
// the project metadata the connector scaffold needs.
func statusTestServer(t *testing.T, body map[string]any, code int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/projects/proj123/sync/main/status", func(w http.ResponseWriter, _ *http.Request) {
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"error":"job system not configured"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestConfirmIngest holds the push to the difference between "the server
// accepted the upload" and "the server applied it". The commit answers 202 and
// hands the work to a worker; a client that stopped there reported success for
// content the worker could still reject — and wrote the sync cache saying the
// server holds it, so the next push found nothing to send and the content was
// stranded until someone ran --force.
func TestConfirmIngest(t *testing.T) {
	tests := []struct {
		name      string
		body      map[string]any
		httpCode  int
		pushID    string
		wantState string
		wantErr   string
	}{
		{
			name:      "ingest completed is applied",
			body:      map[string]any{"status": "completed", "total": 1, "completed": 1},
			httpCode:  http.StatusOK,
			pushID:    "push-1",
			wantState: bowrainconn.IngestApplied,
		},
		{
			name: "ingest failed is an error, not a state",
			body: map[string]any{"status": "failed", "total": 1, "failed": 1},

			httpCode: http.StatusOK,
			pushID:   "push-2",
			wantErr:  "failed to apply it",
		},
		{
			name:      "translation jobs still running do not mask a landed ingest",
			body:      map[string]any{"status": "in_progress", "total": 4, "completed": 1, "in_progress": 3},
			httpCode:  http.StatusOK,
			pushID:    "push-3",
			wantState: bowrainconn.IngestApplied,
		},
		{
			name:      "a translation job failing after the ingest landed is not the ingest failing",
			body:      map[string]any{"status": "completed", "total": 3, "completed": 1, "failed": 2},
			httpCode:  http.StatusOK,
			pushID:    "push-4",
			wantState: bowrainconn.IngestApplied,
		},
		{
			name:      "a server that cannot be asked is unknown, not failed",
			body:      nil,
			httpCode:  http.StatusServiceUnavailable,
			pushID:    "push-5",
			wantState: bowrainconn.IngestUnknown,
		},
		{
			name:      "the unchanged fast path has nothing to confirm",
			body:      map[string]any{"status": "completed", "total": 1, "completed": 1},
			httpCode:  http.StatusOK,
			pushID:    "unchanged",
			wantState: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := statusTestServer(t, tt.body, tt.httpCode)
			c := newPullTestConnector(t, srv, []string{"fr"}, 0)

			state, _, err := c.confirmIngest(t.Context(), tt.pushID)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantState, state)
		})
	}
}
