package atomicfile_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/atomicfile"
)

// These cases need no symbolic link, so they run on every platform, Windows
// included, where making a link needs a privilege the test runner may lack.
func TestStageWithinOnEveryPlatform(t *testing.T) {
	body := func(w io.Writer) error {
		_, err := io.WriteString(w, "body")
		return err
	}
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))

	t.Run("a new file and its directories", func(t *testing.T) {
		target := filepath.Join(root, "site", "fr", "app.json")
		s, err := atomicfile.StageWithin(root, target, body)
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		got, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "body", string(got))
	})

	t.Run("an existing file is replaced", func(t *testing.T) {
		target := filepath.Join(root, "old.json")
		require.NoError(t, os.WriteFile(target, []byte("old"), 0o644))
		s, err := atomicfile.StageWithin(root, target, body)
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		got, _ := os.ReadFile(target)
		assert.Equal(t, "body", string(got))
	})

	t.Run("a target outside the root is refused", func(t *testing.T) {
		_, err := atomicfile.StageWithin(root, filepath.Join(outside, "x.json"), body)
		require.Error(t, err)
		entries, err := os.ReadDir(outside)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("a discarded file leaves nothing", func(t *testing.T) {
		dir := filepath.Join(root, "discard")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		s, err := atomicfile.StageWithin(root, filepath.Join(dir, "x.json"), body)
		require.NoError(t, err)
		require.NoError(t, s.Discard())
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
}
