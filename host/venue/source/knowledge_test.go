package source

import (
	"testing"

	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewKnowledgeClientSeparatesSkipFromFailure holds the client builder to the
// distinction its callers depend on. The terminology fold treated any error
// here as "nothing to do" and returned nil, nil — so a project that WAS claimed
// into a workspace, whose credential had merely expired, pushed all of its
// content and none of its terminology, and said nothing about it.
//
// Only the conditions under which there is genuinely no workspace graph wrap
// ErrNotWorkspaceClaimed. Everything else is a plain error the caller must surface.
func TestNewKnowledgeClientSeparatesSkipFromFailure(t *testing.T) {
	t.Run("no server block is a skip", func(t *testing.T) {
		proj, _ := newServerProject(t, nil, nil)
		_, err := NewKnowledgeClient(proj)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotWorkspaceClaimed,
			"a project with no server is not connected to a workspace graph")
	})

	t.Run("a project-scoped claim without a workspace is a skip", func(t *testing.T) {
		proj, _ := newServerProject(t, &bproject.ServerSpec{
			URL: "https://bowrain.example/projects/proj123",
		}, nil)
		_, err := NewKnowledgeClient(proj)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotWorkspaceClaimed,
			"the graph is workspace-scoped, so a project-only URL has none to read")
	})

	t.Run("a workspace project whose credential does not apply is a failure", func(t *testing.T) {
		// The credential lookup reads the developer's own keychain, so this
		// asserts the CLASSIFICATION rather than the sentence: whether the
		// machine has no login at all or one minted for another server, the
		// project names a workspace and we did not reach it. Both are failures
		// and neither is ErrNotWorkspaceClaimed — which is the whole distinction the
		// callers act on.
		t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
		t.Setenv("XDG_DATA_HOME", t.TempDir())

		proj, _ := newServerProject(t, &bproject.ServerSpec{
			URL: "https://bowrain.invalid/acme/proj123",
		}, nil)
		_, err := NewKnowledgeClient(proj)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotWorkspaceClaimed,
			"the project names a workspace: failing to reach it is a failure, not a skip")
	})
}
