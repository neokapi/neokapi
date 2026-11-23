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

// statePath is the checkout's state directory as `kapi state path` prints it.
func statePath(t *testing.T, recipe string) string {
	t.Helper()
	return strings.TrimSpace(kapi(t, "state", "path", "-p", recipe))
}

// A checkout's state lives outside it, and kapi writes nothing into the
// checkout to keep it.
func TestACheckoutsStateLivesOutsideIt(t *testing.T) {
	root, recipe := writeBareProject(t, "outside")
	kapi(t, "status", "-p", recipe)
	dir := statePath(t, recipe)
	assert.DirExists(t, filepath.Join(dir, "work"))
	assert.False(t, strings.HasPrefix(dir, root), "the state directory is outside the checkout")
	assert.NoDirExists(t, filepath.Join(root, ".kapi"))
}

// A repository cloned where another checkout used to be sees none of that
// checkout's state, and kapi says so once.
func TestACheckoutClonedWhereAnotherWasSeesNoneOfItsState(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "app")
	first, _ := writeBareProject(t, "first")
	require.NoError(t, os.Rename(first, root))
	recipe := filepath.Join(root, "kapi.yaml")
	kapi(t, "status", "-p", recipe)
	earlier := statePath(t, recipe)
	require.NoError(t, os.MkdirAll(filepath.Join(earlier, "sync"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(earlier, "sync", "sync-cache.json"), []byte(`{"claim_token":"clm_first"}`), 0o600))

	require.NoError(t, os.RemoveAll(root))
	second, _ := writeBareProject(t, "second")
	require.NoError(t, os.Rename(second, root))
	out, err := kapiAllowFail(t, "status", "-p", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "belongs to another directory, so this checkout starts with none")
	assert.NotEqual(t, earlier, statePath(t, recipe))
	assert.NoFileExists(t, filepath.Join(statePath(t, recipe), "sync", "sync-cache.json"))
}

// State an earlier kapi left inside a checkout is not read. A person imports
// it with `kapi state import`, which shows what it takes; an agent cannot.
func TestStateInsideACheckoutIsImportedOnlyByAPerson(t *testing.T) {
	root, recipe := writeBareProject(t, "legacy")
	vault := filepath.Join(root, ".kapi", "vault", "redaction.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(vault), 0o700))
	require.NoError(t, os.WriteFile(vault, []byte(`{"version":3,"values":[]}`), 0o600))

	out, err := kapiAllowFail(t, "status", "-p", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "kapi state import -p "+root)
	dir := statePath(t, recipe)
	assert.NoFileExists(t, filepath.Join(dir, "vault", "redaction.json"), "nothing is taken unasked")

	out, err = kapiEnv(t, []string{"KAPI_ACTOR=agent"}, "state", "import", "--yes", "-p", recipe)
	require.Error(t, err, out)
	assert.Contains(t, out, "only a person can bring state into a checkout")

	out, err = kapiAllowFail(t, "state", "import", "--yes", "-p", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "vault/redaction.json")
	assert.FileExists(t, filepath.Join(dir, "vault", "redaction.json"))
	assert.FileExists(t, vault, "the copy in the checkout stays without --remove")
}

// A copied checkout starts without the state; relink takes it across.
func TestACopiedCheckoutRelinksItsState(t *testing.T) {
	root, recipe := writeBareProject(t, "original")
	kapi(t, "status", "-p", recipe)
	dir := statePath(t, recipe)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vault"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vault", "redaction.json"), []byte(`{"version":3,"values":[]}`), 0o600))

	copied := filepath.Join(t.TempDir(), "copy")
	out, err := exec.Command("cp", "-R", root, copied).CombinedOutput()
	require.NoError(t, err, string(out))
	copyRecipe := filepath.Join(copied, "kapi.yaml")
	kapi(t, "status", "-p", copyRecipe)
	assert.NoFileExists(t, filepath.Join(statePath(t, copyRecipe), "vault", "redaction.json"))

	require.NoError(t, os.RemoveAll(root))
	got, err := kapiAllowFail(t, "state", "relink", "--from", root, "--yes", "-p", copyRecipe)
	require.NoError(t, err, got)
	assert.FileExists(t, filepath.Join(statePath(t, copyRecipe), "vault", "redaction.json"))
}
