//go:build e2e && unix

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyFile is the state key of the isolated configuration the suite runs kapi
// with.
func keyFile(t *testing.T) string {
	t.Helper()
	for _, kv := range isoEnv {
		if dir, ok := strings.CutPrefix(kv, "KAPI_CONFIG_DIR="); ok {
			return filepath.Join(dir, "state", "machine.key")
		}
	}
	t.Fatal("the suite sets no KAPI_CONFIG_DIR")
	return ""
}

// A project store unpacked over a checkout kapi made is set aside and rebuilt,
// though the checkout's directories keep their identity.
func TestAStoreUnpackedOverACheckoutIsSetAside(t *testing.T) {
	a, aRecipe := writeBareProject(t, "a")
	kapi(t, "status", "-p", aRecipe)
	b, bRecipe := writeBareProject(t, "b")
	kapi(t, "status", "-p", bRecipe)

	archive := filepath.Join(t.TempDir(), "a.tar")
	out, err := exec.Command("tar", "-cf", archive, "-C", a, ".kapi/work/store.db").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("tar", "-xf", archive, "-C", b).CombinedOutput()
	require.NoError(t, err, string(out))

	got, err := kapiAllowFail(t, "status", "-p", bRecipe)
	require.NoError(t, err, got)
	assert.Contains(t, got, ".kapi/work was not written by kapi on this machine; it is set aside")
}

// A project store copied onto this checkout's store, which keeps the file's
// identity, records another checkout's mark, and is set aside too.
func TestAStoreCopiedOntoTheStoreIsSetAside(t *testing.T) {
	a, aRecipe := writeBareProject(t, "a")
	kapi(t, "status", "-p", aRecipe)
	b, bRecipe := writeBareProject(t, "b")
	kapi(t, "status", "-p", bRecipe)
	data, err := os.ReadFile(filepath.Join(a, ".kapi", "work", "store.db"))
	require.NoError(t, err)
	for _, sidecar := range []string{"-wal", "-shm"} {
		_ = os.Remove(filepath.Join(b, ".kapi", "work", "store.db"+sidecar))
	}
	f, err := os.OpenFile(filepath.Join(b, ".kapi", "work", "store.db"), os.O_WRONLY|os.O_TRUNC, 0)
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := kapiAllowFail(t, "status", "-p", bRecipe)
	require.NoError(t, err, got)
	assert.Contains(t, got, ".kapi/work was not written by kapi on this machine; it is set aside")
}

// A work directory restored into a checkout on every run, as a CI cache that
// holds .kapi restores it, is set aside each time, and the state directory
// keeps one set-aside copy of it rather than one per run.
func TestARestoredWorkDirectoryLeavesOneSetAsideCopy(t *testing.T) {
	root, recipe := copiedCheckout(t)
	saved := filepath.Join(t.TempDir(), "work")
	out, err := exec.Command("cp", "-R", filepath.Join(root, ".kapi", "work"), saved).CombinedOutput()
	require.NoError(t, err, string(out))
	for range 3 {
		_ = os.RemoveAll(filepath.Join(root, ".kapi", "work"))
		out, err := exec.Command("cp", "-R", saved, filepath.Join(root, ".kapi", "work")).CombinedOutput()
		require.NoError(t, err, string(out))
		got, err := kapiAllowFail(t, "status", "-p", recipe)
		require.Error(t, err, got)
		assert.Contains(t, got, ".kapi/vault was not written by kapi on this machine")
		assert.Contains(t, got, "it is set aside")
	}
	entries, err := os.ReadDir(filepath.Join(root, ".kapi", "set-aside"))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// A state key other users can read is refused, with the fix.
func TestAStateKeyOthersCanReadIsRefused(t *testing.T) {
	_, recipe := writeBareProject(t, "key")
	kapi(t, "status", "-p", recipe)
	key := keyFile(t)
	require.NoError(t, os.Chmod(key, 0o644))
	t.Cleanup(func() { _ = os.Chmod(key, 0o600) })
	out, err := kapiAllowFail(t, "status", "-p", recipe)
	require.Error(t, err, out)
	assert.Contains(t, out, "chmod 600")
}
