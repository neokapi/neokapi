package contextremote

import (
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
