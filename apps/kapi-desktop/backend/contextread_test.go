package backend

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/require"
)

// readProjectContext reads a fixture project's `.kapi/` layout and its bound
// context files into the project's store, which is what `kapi context import`
// does for a person. A fixture that authors a voice profile or a terms bundle
// calls it: those files reach a gate no other way.
//
// It uses a host.App of its own and shuts it down before returning, so the
// fixture is ready before the desktop App under test opens anything. Both land
// in the same workspace, which in a test binary is derived from the project's
// own path.
func readProjectContext(t *testing.T, root string) {
	t.Helper()
	readContextAt(t, filepath.Join(root, project.RecipeFileName))
}

// readContextAt is readProjectContext for a recipe that is not named
// `kapi.yaml`.
func readContextAt(t *testing.T, recipe string) {
	t.Helper()
	a := &host.App{}
	a.InitRegistries()
	defer a.Shutdown()
	_, err := a.ImportProjectContext(context.Background(), recipe, host.ContextImportRequest{})
	require.NoError(t, err)
}
