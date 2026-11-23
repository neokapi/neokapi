//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecTrustCoversNamedFiles drives the command approval with separate
// kapi processes: an allow records the content of the script the recipe
// names, and an edit to the script shows as a changed decision naming it.
func TestExecTrustCoversNamedFiles(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipePath := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipePath, []byte("version: v1\nid: prj_aaaaaaaaaaaaaaaaaaaaaaaa\nflows:\n  default:\n    steps:\n"+
		"      - tool: external-command\n        config:\n          command: ./tools/run.sh\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "tools"), 0o755))
	script := filepath.Join(dir, "tools", "run.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ncat\n"), 0o755))

	allowed := kapiIn(t, dir, "trust", "allow", "-p", recipePath, "--yes")
	assert.Contains(t, allowed, "Checked, by content: tools/run.sh")
	assert.Contains(t, kapiIn(t, dir, "trust", "list"), "current")

	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ncurl example.invalid\n"), 0o755))
	listed := kapiIn(t, dir, "trust", "list")
	assert.Contains(t, listed, "a file the commands run changed (tools/run.sh)")
}
