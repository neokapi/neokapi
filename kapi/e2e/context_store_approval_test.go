//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContextStoreApproval drives the approval a folder context store needs,
// with separate kapi processes and no terminal. A sync through a folder nobody
// allowed is refused and names the command. The command records the allow. A
// recipe naming another folder is refused again, and an agent can neither
// allow a folder nor sync through one.
func TestContextStoreApproval(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte("# Guide\n\nWe utilise the widget.\n"), 0o644))
	shared, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipePath := filepath.Join(dir, "kapi.yaml")

	// A person naming the folder chooses it; withdrawing that leaves the
	// project as a cloned repository's recipe would arrive.
	kapiIn(t, dir, "share", "-p", recipePath, "--via", "folder", shared)
	revoked := kapiIn(t, dir, "trust", "revoke", "--context-store", "-p", recipePath)
	assert.Contains(t, revoked, "Withdrew the allow recorded for "+recipePath)

	out, err := kapiEnv(t, nil, "context", "sync", "-p", recipePath)
	require.Error(t, err, out)
	assert.Contains(t, out, "keeps the project's context in the folder "+shared)
	assert.Contains(t, out, "kapi trust allow --context-store -p "+recipePath)

	out, err = kapiEnv(t, []string{"KAPI_ACTOR=agent"}, "trust", "allow", "--context-store", "-p", recipePath, "--yes")
	require.Error(t, err, out)
	assert.Contains(t, out, "only a person can allow")

	allowed := kapiEnvOK(t, nil, "trust", "allow", "--context-store", "-p", recipePath, "--yes")
	assert.Contains(t, allowed, "Recorded: "+recipePath+" may keep its context in "+shared)
	listed := kapiEnvOK(t, nil, "trust", "list")
	assert.Contains(t, listed, "context-store")
	kapiEnvOK(t, nil, "context", "sync", "-p", recipePath)

	out, err = kapiEnv(t, []string{"KAPI_ACTOR=agent"}, "share", "-p", recipePath, "--via", "folder", filepath.Join(shared, "other"))
	require.Error(t, err, out)
	assert.Contains(t, out, "An agent is running this command")
	assert.NoDirExists(t, filepath.Join(shared, "other"), "nothing is written before a person allows the folder")

	recipe, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	moved := filepath.Join(shared, "moved")
	require.NoError(t, os.WriteFile(recipePath, []byte(strings.Replace(string(recipe), "path: "+filepath.Join(shared, "other"), "path: "+moved, 1)), 0o644))
	out, err = kapiEnv(t, nil, "context", "sync", "-p", recipePath)
	require.Error(t, err, out)
	assert.Contains(t, out, "keeps the project's context in the folder "+moved)
	assert.NoDirExists(t, moved)
}
