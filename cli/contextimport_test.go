package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/require"
)

// readProjectContext reads a fixture project's `.kapi/` layout and the context
// files its recipe binds into the project store, which is what `kapi context
// import` does for a person. A fixture that authors a voice profile or a terms
// bundle calls it: those files reach a gate no other way.
//
// It uses an App of its own, so the fixture is ready before the App under test
// opens anything. Both land in the same workspace, which in a test binary is
// derived from the project's own path.
func readProjectContext(t *testing.T, root string) {
	t.Helper()
	readContextAt(t, filepath.Join(root, project.RecipeFileName))
}

// layoutVoicePath is where a project's `.kapi/` layout keeps its voice profile,
// with the directory created so the caller writes the file and an import finds
// it. The import binds the voice it reads when the recipe binds none.
func layoutVoicePath(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, project.RelStatePath("voice.yaml"))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	return path
}

// namedTermStorePath is where the terms store a recipe names with `termstore:
// <name>` lives, the store `--termstore <name>` opens. A test that has not
// isolated the configuration directory gets a temporary one.
func namedTermStorePath(t *testing.T, name string) string {
	t.Helper()
	if os.Getenv("KAPI_CONFIG_DIR") == "" {
		t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	}
	dir := filepath.Join(host.ConfigDir(), "terms")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	return filepath.Join(dir, name+".db")
}

// readContextAt is readProjectContext for a recipe that is not named
// `kapi.yaml`.
func readContextAt(t *testing.T, recipe string) {
	t.Helper()
	readContextFrom(t, recipe, "")
}

// readContextFrom reads one directory of context files into a project's store,
// which is `kapi context import <dir>`. A sample keeps its context under
// `context/` at the project root, so that is the directory its fixtures name.
func readContextFrom(t *testing.T, recipe, dir string) {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	defer a.Shutdown()
	_, err := a.ImportProjectContext(context.Background(), recipe, host.ContextImportRequest{Dir: dir})
	require.NoError(t, err)
}
