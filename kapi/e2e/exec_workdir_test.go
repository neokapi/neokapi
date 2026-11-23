//go:build e2e && !windows

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecipeCommandRunsFromTheProjectRoot runs one recipe from the project
// root and from two subdirectories. Each run executes the same ./tools/run.sh
// in the project root under the one approval given once, and the command
// learns where kapi was started from KAPI_INVOKE_DIR.
func TestRecipeCommandRunsFromTheProjectRoot(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipePath := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipePath, []byte("version: v1\nid: prj_bbbbbbbbbbbbbbbbbbbbbbbb\nflows:\n  cmd:\n    steps:\n"+
		"      - tool: external-command\n        config:\n          command: ./tools/run.sh\n          apply_source: true\n          apply_target: false\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools", "run.sh"),
		[]byte("#!/bin/sh\necho \"$(pwd -P)|$KAPI_INVOKE_DIR\" >> ran.txt\ncat\n"), 0o755))
	src := filepath.Join(root, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting": "Hello"}`), 0o644))
	for _, sub := range []string{"docs", filepath.Join("src", "deep")} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, sub), 0o755))
	}

	kapiIn(t, root, "trust", "allow", "-p", recipePath, "--yes")
	dirs := []string{root, filepath.Join(root, "docs"), filepath.Join(root, "src", "deep")}
	for i, dir := range dirs {
		out := filepath.Join(t.TempDir(), "out.json")
		kapiIn(t, dir, "run", "cmd", "-p", recipePath, "-i", src, "-o", out, "--target-lang", "fr")
		lines := strings.Split(strings.TrimSpace(readFile(t, filepath.Join(root, "ran.txt"))), "\n")
		require.NotEmpty(t, lines, "run %d", i)
		last := lines[len(lines)-1]
		assert.Equal(t, root+"|"+dir, last, "the command ran in the project root, and was told where kapi started")
	}
	assert.NoFileExists(t, filepath.Join(root, "docs", "ran.txt"))
}
