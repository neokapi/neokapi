package flow_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
)

// An output path can be a symlink to where the file really lives, which is how
// a checkout shares one file between trees. A run writes a temporary file and
// renames it onto the output path, so it replaces the link with a regular file
// and leaves the real file as it was. The rename also decides the mode: a
// temporary file is the owner's alone, while every other writer here creates
// its file the way os.Create does.
func TestRunFileOutputPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture makes a symlink")
	}

	newRunner := func(t *testing.T) (*flow.FileRunner, string, string) {
		t.Helper()
		root, src, store, reg := overlayKeyFixture(t)
		runner := flow.NewFileRunner(flow.FileRunnerConfig{
			FormatReg:    reg,
			SourceLocale: "en",
			Store:        store,
			ProjectRoot:  root,
		})
		require.NoError(t, os.MkdirAll(filepath.Join(root, "out"), 0o755))
		return runner, root, src
	}

	t.Run("must fail: a symlinked output keeps the link and writes the file it points at", func(t *testing.T) {
		runner, root, src := newRunner(t)
		const before = `{"greeting":"old"}`
		target := filepath.Join(root, "out", "real.json")
		require.NoError(t, os.WriteFile(target, []byte(before), 0o640))
		link := filepath.Join(root, "out", "qps.json")
		require.NoError(t, os.Symlink(target, link))

		require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate", pseudoTools(t, "qps"), src, link, "qps"))

		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "the run replaced the symlink with a regular file")

		written, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.NotEqual(t, before, string(written), "the file the link points at holds the output")
		assert.Contains(t, string(written), "greeting")

		perm, err := os.Stat(target)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), perm.Mode().Perm(), "the file keeps its mode")
	})

	t.Run("must fail: a new output file is readable, not the owner's alone", func(t *testing.T) {
		runner, root, src := newRunner(t)
		out := filepath.Join(root, "out", "new.json")

		require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate", pseudoTools(t, "qps"), src, out, "qps"))

		info, err := os.Stat(out)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	})
}
