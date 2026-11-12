package host

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Forgetting a project closes its context store in the workspace. Opening the
// project again, by its checkout or by its key alone, must read a fresh store,
// not a handle left over from before. The desktop does exactly this when a
// project is removed from the workspace and its sample is opened again.
func TestForgetProject_ReopenReadsAFreshStore(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	root := storeRoot(t)

	w, err := a.Projector(t.Context(), root)
	require.NoError(t, err)
	key := a.ensureProjectStores().bound[root].key
	require.NotEmpty(t, key)
	_, err = a.ProjectorFor(t.Context(), key)
	require.NoError(t, err)
	_, _, err = termsWriter(w).Search(t.Context(), "", "", "", 0, 1)
	require.NoError(t, err)

	require.NoError(t, a.ForgetProject(t.Context(), key))

	w, err = a.Projector(t.Context(), root)
	require.NoError(t, err)
	_, _, err = termsWriter(w).Search(t.Context(), "", "", "", 0, 1)
	require.NoError(t, err, "the checkout reopens on a fresh store")

	k, err := a.ProjectorFor(t.Context(), key)
	require.NoError(t, err)
	_, _, err = termsWriter(k).Search(t.Context(), "", "", "", 0, 1)
	require.NoError(t, err, "the writer opened by key reopens on a fresh store")
}
