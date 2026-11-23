package commands

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/venue/project"
)

// `kapi auth claim` with no token argument sends the sync state's claim token
// only to the server it was issued by: a sync state that records no server,
// or records another than the one the person is signed in to, sends nothing.
func TestAuthClaimSendsTheCachedTokenOnlyToItsServer(t *testing.T) {
	for _, tc := range []struct {
		name     string
		recorded func(signedIn string) string
		want     string
	}{
		{"no server recorded", func(string) string { return "" }, "records no server it was issued by"},
		{"another server recorded", func(string) string { return "https://issuer.example" }, "was issued by https://issuer.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()
			t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
			t.Setenv("BOWRAIN_AUTH_TOKEN", "signed-in-token")
			t.Setenv("BOWRAIN_SERVER_URL", srv.URL)

			root := t.TempDir()
			proj, err := project.InitProject(root, &project.Recipe{
				Defaults: coreproj.Defaults{SourceLanguage: "en"},
				Server:   &project.ServerSpec{URL: srv.URL + "/projects/proj123"},
			})
			require.NoError(t, err)
			cache := project.LoadSyncCache(proj.Layout)
			cache.ClaimToken = "clm_issued_elsewhere"
			cache.ServerURL = tc.recorded(srv.URL)
			require.NoError(t, cache.Save(proj.Layout))

			t.Setenv("KAPI_NO_PROJECT", "1")
			cmd := &cobra.Command{Use: "claim", RunE: authClaimCmd.RunE, SilenceUsage: true, SilenceErrors: true}
			addProjectFlag(cmd)
			cmd.SetArgs([]string{"-p", root})
			err = cmd.Execute()
			require.ErrorContains(t, err, tc.want)
			assert.Zero(t, requests.Load(), "nothing reached the signed-in server")
		})
	}
}
