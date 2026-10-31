package contextremote

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host/venue/schema"
)

// The redirect a push honours is the redirect the context honours.
func TestTheProjectURLRedirectIsThePushOne(t *testing.T) {
	assert.Equal(t, schema.ProjectURLEnvVar, projectURLEnvVar)
}

// The remote places itself on the project the URL names, and describes
// itself by it.
func TestNew_PlacesTheProject(t *testing.T) {
	t.Setenv(projectURLEnvVar, "")
	r, err := New(Options{URL: "https://bowrain.example/acme/p123/"})
	require.NoError(t, err)
	assert.Equal(t, workspace.RemoteDescriptor{Kind: "bowrain", Location: "https://bowrain.example/acme/p123"}, r.Describe())
}

// BOWRAIN_PROJECT_URL points the context at the project it names, as it
// points a push there.
func TestNew_TheRedirectWins(t *testing.T) {
	t.Setenv(projectURLEnvVar, "http://localhost:8080/dev/p9")
	r, err := New(Options{URL: "https://bowrain.example/acme/p123"})
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080/dev/p9", r.Describe().Location)
}

// A URL that does not name a project in a workspace is refused before any
// request.
func TestNew_RefusesAURLWithoutAWorkspaceProject(t *testing.T) {
	t.Setenv(projectURLEnvVar, "")
	for raw, want := range map[string]string{
		"ftp://bowrain.example/acme/p1":       "is not a project URL",
		"https://bowrain.example/acme":        "does not name a project",
		"https://bowrain.example/a/b/c":       "does not name a project",
		"https://bowrain.example/projects/p1": "kapi auth claim",
	} {
		_, err := New(Options{URL: raw})
		require.Error(t, err, raw)
		assert.Contains(t, err.Error(), want, raw)
	}
}

// A bearer token travels over TLS: http is taken only for a server on this
// machine.
func TestNew_RefusesCleartextOffThisMachine(t *testing.T) {
	t.Setenv(projectURLEnvVar, "")
	_, err := New(Options{URL: "http://bowrain.example/acme/p1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "give bowrain.url an https:// address")
	for _, raw := range []string{"http://localhost:8080/acme/p1", "http://127.0.0.1:8080/acme/p1", "http://[::1]:8080/acme/p1"} {
		_, err := New(Options{URL: raw})
		require.NoError(t, err, raw)
	}
}

// signInFor stands in a CI sign-in: a token, and the server that issued it.
func signInFor(t *testing.T, server string) {
	t.Setenv("BOWRAIN_AUTH_TOKEN", "the-token")
	t.Setenv("BOWRAIN_SERVER_URL", server)
	t.Setenv(projectURLEnvVar, "")
}

// countingHost is a listener standing in for a host a recipe names; it counts
// the requests that reach it.
func countingHost(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(hs.Close)
	return hs.URL, &hits
}

// The stored tokens go only to the server the sign-in names. A recipe that
// names another host is refused, and no request reaches that host.
func TestTheSignInReachesOnlyItsOwnServer(t *testing.T) {
	host, hits := countingHost(t)
	signInFor(t, "https://app.bowrain.example")
	r, err := New(Options{URL: host + "/acme/p1"})
	require.NoError(t, err)
	_, err = r.List(t.Context(), workspace.RemoteLogDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the stored sign-in does not name "+host)
	assert.Zero(t, hits.Load(), "no request reached the host the recipe names")
}

// A sign-in that names no server is refused rather than sent wherever the
// recipe points.
func TestASignInWithoutAServerIsRefused(t *testing.T) {
	host, hits := countingHost(t)
	signInFor(t, "")
	r, err := New(Options{URL: host + "/acme/p1"})
	require.NoError(t, err)
	err = r.Put(t.Context(), workspace.Object{Name: workspace.SegmentName("w1", "000000000000000000000001"), Data: []byte("x\n")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BOWRAIN_SERVER_URL")
	assert.Zero(t, hits.Load())
}

// The server the sign-in names receives the request.
func TestTheSignInReachesItsServer(t *testing.T) {
	host, hits := countingHost(t)
	signInFor(t, host)
	r, err := New(Options{URL: host + "/acme/p1"})
	require.NoError(t, err)
	_, err = r.List(t.Context(), workspace.RemoteLogDir)
	require.Error(t, err, "the stand-in answers 418")
	assert.Equal(t, int32(1), hits.Load())
}
