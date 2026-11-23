package source

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// A claim token is sent only to the server and project the sync state records
// it was issued for. A sync state that records none, or records another, sends
// no token.
func TestAClaimTokenGoesOnlyToTheServerItWasIssuedFor(t *testing.T) {
	for _, tc := range []struct {
		name      string
		recorded  func(srv string) (server, project string)
		wantToken bool
	}{
		{"recorded for this server and project", func(srv string) (string, string) { return srv, "proj123" }, true},
		{"no server recorded", func(string) (string, string) { return "", "" }, false},
		{"another server recorded", func(string) (string, string) { return "https://elsewhere.example", "proj123" }, false},
		{"another project recorded", func(srv string) (string, string) { return srv, "proj999" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BOWRAIN_AUTH_TOKEN", "")
			// No sign-in of the person running the tests is read or sent.
			t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
			var mu sync.Mutex
			var auth []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				auth = append(auth, r.Header.Get("Authorization"))
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"proj123","name":"p"}`))
			}))
			defer srv.Close()

			proj, reg := newServerProject(t, &bproject.ServerSpec{URL: srv.URL + "/projects/proj123"}, nil)
			cache := bproject.LoadSyncCache(proj.Layout)
			cache.ClaimToken = "clm_issued"
			cache.ServerURL, cache.ProjectID = tc.recorded(srv.URL)
			require.NoError(t, cache.Save(proj.Layout))

			conn, err := NewSourceConnector(t.Context(), &host.App{}, proj, reg)
			if !tc.wantToken {
				// Without the token and with no other credential, the
				// connector cannot authenticate at all.
				if err != nil {
					assert.NotContains(t, err.Error(), "clm_issued")
					return
				}
			}
			require.NoError(t, err)
			defer conn.Close()
			_, _ = conn.client.GetProjectMetadata(t.Context())

			mu.Lock()
			defer mu.Unlock()
			sent := strings.Join(auth, "\n")
			if tc.wantToken {
				assert.Contains(t, sent, "clm_issued")
			} else {
				assert.NotContains(t, sent, "clm_issued")
			}
		})
	}
}
