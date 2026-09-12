package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findKapi must answer for this checkout or not at all. A binary resolved from
// PATH can lag the tree by weeks and its output is shaped identically, so a run
// against the wrong build produces a confident wrong result. See #2642.
func TestFindKapiRequiresTheCheckoutBinary(t *testing.T) {
	t.Run("returns the built binary", func(t *testing.T) {
		root := t.TempDir()
		bin := filepath.Join(root, "bin")
		require.NoError(t, os.MkdirAll(bin, 0o755))
		path := filepath.Join(bin, "kapi")
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755))

		got := findKapi(root)
		require.NotEmpty(t, got, "a built binary must be found")

		resolved, err := filepath.EvalSymlinks(path)
		require.NoError(t, err)
		assert.Equal(t, resolved, got)
	})

	t.Run("returns empty when the checkout has no build", func(t *testing.T) {
		// A kapi on PATH is deliberately not consulted, so this stays empty on
		// a developer machine that has one installed.
		assert.Empty(t, findKapi(t.TempDir()),
			"an unbuilt checkout must not silently resolve an installed kapi")
	})

	t.Run("refuses a symlink pointing outside the checkout", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "installed-kapi")
		require.NoError(t, os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755))

		root := t.TempDir()
		bin := filepath.Join(root, "bin")
		require.NoError(t, os.MkdirAll(bin, 0o755))
		require.NoError(t, os.Symlink(outside, filepath.Join(bin, "kapi")))

		assert.Empty(t, findKapi(root),
			"a bin/kapi symlinked at an installed release must not pass as the checkout's")
	})
}
