//go:build e2e && !windows

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecTrustCoversTheFlowFiles: a flow file added to flows_dir after the
// project's commands were allowed does not run under that answer. Without a
// terminal, kapi run refuses it with exit code 1 and names the new step and
// the approving command, and kapi trust list reports the addition.
func TestExecTrustCoversTheFlowFiles(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipePath := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipePath, []byte("version: v1\nid: prj_cccccccccccccccccccccccc\nflows_dir: flows\nflows:\n  cmd:\n    steps:\n"+
		"      - tool: external-command\n        config:\n          command: ./tools/run.sh\n          apply_source: true\n          apply_target: false\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "flows"), 0o755))
	for _, name := range []string{"run.sh", "other.sh"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, "tools", name),
			[]byte("#!/bin/sh\necho "+name+" >> ran.txt\ncat\n"), 0o755))
	}
	src := filepath.Join(root, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting": "Hello"}`), 0o644))

	allowed := kapiIn(t, root, "trust", "allow", "-p", recipePath, "--yes")
	assert.Contains(t, allowed, "Checked, by content: tools/run.sh")
	assert.Contains(t, allowed, "kapi checks the content of the files listed, not what those files or the commands run or read.")

	require.NoError(t, os.WriteFile(filepath.Join(root, "flows", "sneaky.yaml"), []byte("steps:\n"+
		"  - tool: external-command\n    config:\n      command: ./tools/other.sh\n      apply_source: true\n      apply_target: false\n"), 0o644))

	out, err := kapiEnv(t, nil, "run", "sneaky", "-p", recipePath, "-i", src, "-o", filepath.Join(t.TempDir(), "out.json"), "--target-lang", "fr")
	require.Error(t, err, out)
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, 1, exit.ExitCode())
	assert.Contains(t, out, "Added since it was answered: flows/sneaky.yaml steps[0] external-command")
	assert.Contains(t, out, "kapi trust allow -p "+recipePath)
	assert.NoFileExists(t, filepath.Join(root, "ran.txt"), "nothing ran")

	assert.Contains(t, kapiIn(t, root, "trust", "list"), "a command was added (flows/sneaky.yaml steps[0] external-command)")
}
