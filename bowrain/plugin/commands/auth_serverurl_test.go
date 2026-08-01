package commands

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/plugin/schema"
	"github.com/stretchr/testify/assert"
)

// The recipe's server.url is a compound project URL —
// <server>/<workspace>/<project-id> — and the device flow appends
// "/api/v1/auth/device/start" to whatever this resolves to. Keeping the path
// produced a POST to /<workspace>/<project>/api/v1/auth/device/start, which
// matches no /api/* CDN behaviour and is rejected for its method before it
// reaches the server. `kapi bowrain auth login` failed with a CloudFront 403
// that read like an outage and was a client-side URL bug.
func TestRecipeServerURLReducesToTheOrigin(t *testing.T) {
	for name, tc := range map[string]struct{ raw, want string }{
		"workspace and project": {
			raw:  "https://app.bowrain.cloud/bowrain-hq/uek630Q5",
			want: "https://app.bowrain.cloud",
		},
		"direct project": {
			raw:  "https://app.bowrain.cloud/projects/uek630Q5",
			want: "https://app.bowrain.cloud",
		},
		"bare origin is unchanged": {
			raw:  "https://app.bowrain.cloud",
			want: "https://app.bowrain.cloud",
		},
		"self-hosted with a port": {
			raw:  "http://localhost:8080/acme/proj-1",
			want: "http://localhost:8080",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := schema.ParseProjectURL(tc.raw).ServerURL
			assert.Equal(t, tc.want, got)

			// The concrete failure: the assembled endpoint must sit under /api/.
			assert.Equal(t, tc.want+"/api/v1/auth/device/start",
				got+"/api/v1/auth/device/start")
			assert.NotContains(t, got, "/bowrain-hq",
				"a workspace segment in the API base is what produced the 403")
		})
	}
}
