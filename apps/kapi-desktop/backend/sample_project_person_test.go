package backend

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The sample's context import and its seeded sessions go through the host's
// policy as the person at the keyboard, whatever shell launched the app. A
// terminal inside an agent host leaves the host's marker in the environment,
// and read from there the import would be refused as an agent's.
func TestCreateSampleProjectActsAsPersonUnderAnAgentHost(t *testing.T) {
	t.Setenv("KAPI_HOME_DIR", t.TempDir())
	t.Setenv("KAPI_ACTOR", "agent")
	t.Setenv("CLAUDECODE", "1")

	app := NewApp()
	tab, err := app.CreateSampleProject("kapimart")
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	digest, err := app.ProjectContextDigest(tab.ID, "")
	require.NoError(t, err)
	require.Len(t, digest.Conflicts, 1, "the two seeded sessions disagree about one word")
	require.Equal(t, 5, digest.Numbers.Suggested)
}
