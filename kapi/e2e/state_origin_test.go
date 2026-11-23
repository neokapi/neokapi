//go:build e2e && unix

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// copiedCheckout runs kapi in a project, adds a vault to its state, and copies
// the whole checkout elsewhere, as a restored backup or a copied folder would.
func copiedCheckout(t *testing.T) (copyRoot, copyRecipe string) {
	t.Helper()
	src, srcRecipe := writeBareProject(t, "origin")
	kapi(t, "status", "-p", srcRecipe)
	require.NoError(t, os.MkdirAll(filepath.Join(src, ".kapi", "vault"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".kapi", "vault", "redaction.json"), []byte(`{"planted":"original"}`), 0o600))
	copyRoot = filepath.Join(t.TempDir(), "copy")
	out, err := exec.Command("cp", "-R", src, copyRoot).CombinedOutput()
	require.NoError(t, err, string(out))
	return copyRoot, filepath.Join(copyRoot, "kapi.yaml")
}

// State this machine did not write is not used: the work directory is set
// aside and rebuilt with one line saying so, and the vault is refused until a
// person vouches for it with `kapi trust allow --state`, after which the
// checkout works.
func TestStateCopiedFromAnotherCheckoutIsRefusedUntilVouchedFor(t *testing.T) {
	root, recipe := copiedCheckout(t)

	out, err := kapiAllowFail(t, "status", "-p", recipe)
	require.Error(t, err, out)
	assert.Contains(t, out, ".kapi/work was not written by kapi on this machine; it is set aside")
	assert.Contains(t, out, ".kapi/vault was not written by kapi on this machine")
	assert.Contains(t, out, "kapi trust allow --state -p "+root)
	assert.NotContains(t, out, "planted")

	out, err = kapiAllowFail(t, "trust", "allow", "--state", "--yes", "-p", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "uses the state in")

	kapi(t, "status", "-p", recipe)
}

// An agent cannot vouch for state.
func TestAnAgentCannotVouchForState(t *testing.T) {
	_, recipe := copiedCheckout(t)
	out, err := kapiEnv(t, []string{"KAPI_ACTOR=agent"}, "trust", "allow", "--state", "--yes", "-p", recipe)
	require.Error(t, err, out)
	assert.Contains(t, out, "only a person can vouch for a project's state")
}

// A checkout moved or renamed on its volume keeps its state.
func TestAMovedCheckoutKeepsItsState(t *testing.T) {
	src, srcRecipe := writeBareProject(t, "moved")
	kapi(t, "status", "-p", srcRecipe)
	require.NoError(t, os.MkdirAll(filepath.Join(src, ".kapi", "vault"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".kapi", "vault", "redaction.json"), []byte(`{}`), 0o600))
	// A vault written by hand is vouched for once, as kapi's own writes are
	// recorded.
	kapi(t, "trust", "allow", "--state", "--yes", "-p", srcRecipe)
	moved := filepath.Join(t.TempDir(), "renamed")
	require.NoError(t, os.Rename(src, moved))
	out, err := kapiAllowFail(t, "status", "-p", filepath.Join(moved, "kapi.yaml"))
	require.NoError(t, err, out)
	assert.NotContains(t, out, "not written by kapi on this machine")
}
