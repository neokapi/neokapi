package contextbackend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// Importing the package makes the bowrain backend available to every
// project.
func TestTheBowrainBackendIsRegistered(t *testing.T) {
	assert.True(t, host.HasContextRemote(project.ContextBackendBowrain))
}

// The opener places the remote on the project the resolved spec carries, and
// refuses a spec the host did not resolve from the recipe's bowrain: block.
func TestOpen_ReadsTheResolvedVenue(t *testing.T) {
	t.Setenv("BOWRAIN_PROJECT_URL", "")
	r, err := open(t.Context(), project.ContextBackend{
		Backend: project.ContextBackendBowrain,
		Venue:   &project.ContextVenue{URL: "https://bowrain.example/acme/p123"},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://bowrain.example/acme/p123", r.Describe().Location)

	_, err = open(t.Context(), project.ContextBackend{Backend: project.ContextBackendBowrain})
	require.Error(t, err)
}
