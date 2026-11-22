//go:build unix

package jobs

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/connector"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
)

// A context scan reads the regular files of the checkout it cloned. A
// symbolic link, whatever it points at, and a FIFO are skipped, and a fetch
// that names a link leading out of the checkout is refused.
func TestContextScanSkipsLinksAndOtherFiles(t *testing.T) {
	root := t.TempDir()
	checkout := filepath.Join(root, "checkout")
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(checkout, "docs"), 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(checkout, "README.md"), []byte("# Guide\n\nReal prose.\n"), 0o644))
	secret := filepath.Join(outside, "notes.md")
	require.NoError(t, os.WriteFile(secret, []byte("# Outside\n\nNot part of the repository.\n"), 0o644))
	require.NoError(t, os.Symlink(secret, filepath.Join(checkout, "docs", "linked.md")))
	require.NoError(t, os.Symlink(outside, filepath.Join(checkout, "shared")))
	require.NoError(t, syscall.Mkfifo(filepath.Join(checkout, "docs", "pipe.md"), 0o600))

	listed := []*platconn.ContentItem{
		{Path: "README.md"}, {Path: "docs/linked.md"}, {Path: "shared/notes.md"}, {Path: "docs/pipe.md"},
	}
	assert.Equal(t, []string{"README.md"}, contextScanRepoPaths(checkout, listed))

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	fc, err := connector.NewFileConnector(reg, map[string]string{"path": checkout})
	require.NoError(t, err)
	items, err := fc.List(t.Context())
	require.NoError(t, err)
	for _, it := range items {
		assert.Equal(t, "README.md", it.Path, "only the regular file is listed")
	}
	for _, p := range []string{"docs/linked.md", "shared/notes.md"} {
		_, err := fc.Fetch(t.Context(), platconn.FetchOptions{Paths: []string{p}})
		require.Error(t, err, "%s is refused", p)
	}
}
