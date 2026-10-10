package workspace_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Forgetting a project takes the record of which context files its checkouts
// read in along with the store those files were read into. The stamps are
// what lets an import skip a file already read at these bytes, and the file
// has to be read again into the empty store a re-registered project opens.
func TestForget_ClearsWhatTheCheckoutsRead(t *testing.T) {
	ctx := t.Context()
	ws, err := workspace.OpenLocal(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })

	_, err = ws.Register(ctx, "compass", "Compass", "/learn/compass")
	require.NoError(t, err)
	require.NoError(t, ws.NoteContextImports(ctx,
		workspace.ContextImportStamp{Project: "compass", Checkout: "/learn/compass", Path: "context/terms.json", Digest: "abc"},
		workspace.ContextImportStamp{Project: "other", Checkout: "/learn/other", Path: "context/terms.json", Digest: "def"},
	))

	require.NoError(t, ws.Forget(ctx, "compass"))

	_, ok, err := ws.Lookup(ctx, "compass")
	require.NoError(t, err)
	assert.False(t, ok, "the registration is gone")
	stamps, err := ws.ContextImports(ctx, "compass", "/learn/compass")
	require.NoError(t, err)
	assert.Empty(t, stamps, "the next import reads every file again")
	kept, err := ws.ContextImports(ctx, "other", "/learn/other")
	require.NoError(t, err)
	assert.Len(t, kept, 1, "another project keeps what its checkout read")
}
