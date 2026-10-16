package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
)

// One recipe reached through a symlink and through its real path is one tab:
// the workspace registry holds the canonical path, the home opens by it, and
// a project created under a linked folder would otherwise open twice.
func TestOpenProjectDedupesThroughSymlinks(t *testing.T) {
	app := NewApp()
	base := t.TempDir()
	real := filepath.Join(base, "real")
	require.NoError(t, sample.Scaffold("kapimart", real))
	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(real, link))

	first, err := app.OpenProject(filepath.Join(real, "kapi.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(first.ID) })
	second, err := app.OpenProject(filepath.Join(link, "kapi.yaml"))
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
}
