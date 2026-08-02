package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BOWRAIN_PROJECT_URL exists so a repo whose committed recipe points at
// production can be dogfooded against a local stack without editing that
// recipe — an edited recipe being one `git commit -a` away from redirecting
// everyone's pushes.
func TestProjectURLOverride_RedirectsAConnectedRecipe(t *testing.T) {
	spec := &ServerSpec{URL: "https://app.bowrain.cloud/bowrain-hq/uek630Q5"}

	// Without the override, the recipe's own URL is in force.
	assert.Equal(t, "https://app.bowrain.cloud", spec.ServerURL())
	assert.Equal(t, "bowrain-hq", spec.Workspace())
	assert.Equal(t, "uek630Q5", spec.ProjectID())

	t.Setenv(ProjectURLEnvVar, "http://localhost:8080/dogfood-local/QAX66tYt")

	// All three accessors move together — they read one field, so a redirect
	// cannot leave a run half-pointed at production.
	assert.Equal(t, "http://localhost:8080", spec.ServerURL())
	assert.Equal(t, "dogfood-local", spec.Workspace())
	assert.Equal(t, "QAX66tYt", spec.ProjectID())
}

// The override REDIRECTS an existing connection; it must never CREATE one.
// Arming a project for a server is a decision that belongs in the recipe, in
// git, where it can be reviewed — not in whatever happens to be exported.
func TestProjectURLOverride_DoesNotConnectAnUnconnectedRecipe(t *testing.T) {
	t.Setenv(ProjectURLEnvVar, "http://localhost:8080/dogfood-local/QAX66tYt")

	for name, spec := range map[string]*ServerSpec{
		"nil spec":   nil,
		"empty url":  {URL: ""},
		"stream set": {Stream: "main"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, spec.ServerURL(), "a local-only project must stay local")
			assert.Empty(t, spec.Workspace())
			assert.Empty(t, spec.ProjectID())
		})
	}
}

// A malformed override is rejected where it is set, not later as a confusing
// connection failure.
func TestProjectURLOverride_IsValidated(t *testing.T) {
	spec := &ServerSpec{URL: "https://app.bowrain.cloud/bowrain-hq/uek630Q5"}
	require.NoError(t, spec.Validate())

	t.Setenv(ProjectURLEnvVar, "http://localhost:8080/dogfood-local/QAX66tYt")
	require.NoError(t, spec.Validate(), "a well-formed override validates")

	t.Setenv(ProjectURLEnvVar, "not-a-url-at-all")
	err := spec.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-url-at-all",
		"the error must name the override, not the recipe's URL")
}
