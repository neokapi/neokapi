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

// The CI sequence the convergence-in-CI guide gives, on runners that each start
// empty: check out, ask `kapi state path`, restore the parse cache into it, run,
// save it, then the same on a fresh runner whose checkout is a new folder at the
// same path. The restored directory holds no record; kapi takes it as this
// checkout's and writes the record. The second runner gets another directory, so
// the guide carries the cache through a fixed path.
func TestTheCISequenceCarriesTheParseCacheThroughAFixedPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	saved := filepath.Join(t.TempDir(), "kapi-parse")

	runner := func(n int) (dir string, cached []string) {
		require.NoError(t, os.RemoveAll(root))
		fresh, _ := writeBareProject(t, "ci")
		require.NoError(t, os.Rename(fresh, root))
		recipe := filepath.Join(root, "kapi.yaml")
		env := []string{"KAPI_DATA_DIR=" + filepath.Join(t.TempDir(), "data")}

		out, err := kapiEnv(t, env, "state", "path", "-p", recipe)
		require.NoError(t, err, out)
		dir = strings.TrimSpace(out)
		if _, err := os.Stat(saved); err == nil {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "work", "cache"), 0o700))
			cp, err := exec.Command("cp", "-R", saved, filepath.Join(dir, "work", "cache", "docs")).CombinedOutput()
			require.NoError(t, err, string(cp))
			assert.NoFileExists(t, filepath.Join(dir, "checkout.json"), "run %d: a restored directory holds no record", n)
		}

		out, err = kapiEnv(t, env, "status", "-p", recipe)
		require.NoError(t, err, out)
		assert.NotContains(t, out, "belongs to another directory", "run %d", n)
		data, err := os.ReadFile(filepath.Join(dir, "checkout.json"))
		require.NoError(t, err, "run %d: kapi writes the record on first use", n)
		assert.Contains(t, string(data), "repo")

		require.NoError(t, os.RemoveAll(saved))
		cp, err := exec.Command("cp", "-R", filepath.Join(dir, "work", "cache", "docs"), saved).CombinedOutput()
		require.NoError(t, err, string(cp))
		entries, err := os.ReadDir(saved)
		require.NoError(t, err)
		for _, e := range entries {
			cached = append(cached, e.Name())
		}
		return dir, cached
	}

	first, firstCache := runner(1)
	require.NotEmpty(t, firstCache, "the first run fills the parse cache")
	second, secondCache := runner(2)
	assert.NotEqual(t, first, second, "a new folder at the same path gets another state directory")
	assert.Subset(t, secondCache, firstCache, "what the first run cached is carried to the second")
}

// Two clones of one project in different folders, on runners with empty data
// roots, share KAPI_PARSE_CACHE_DIR: the first run fills it, the second reads
// what the first recorded and adds nothing, and neither keeps a parse cache in
// its state directory.
func TestTwoClonesShareTheParseCacheDirectory(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "kapi-parse")
	logs := func() int {
		m, err := filepath.Glob(filepath.Join(shared, "parts-*.log"))
		require.NoError(t, err)
		return len(m)
	}
	run := func(name string) (out, state string) {
		root, _ := writeBareProject(t, "shared")
		moved := filepath.Join(t.TempDir(), name)
		require.NoError(t, os.Rename(root, moved))
		recipe := filepath.Join(moved, "kapi.yaml")
		env := []string{"KAPI_PARSE_CACHE_DIR=" + shared, "KAPI_DATA_DIR=" + filepath.Join(t.TempDir(), "data")}
		status, err := kapiEnv(t, env, "status", "-p", recipe)
		require.NoError(t, err, status)
		path, err := kapiEnv(t, env, "state", "path", "-p", recipe)
		require.NoError(t, err, path)
		return strings.ReplaceAll(status, moved, "ROOT"), strings.TrimSpace(path)
	}

	first, firstState := run("clone-a")
	filled := logs()
	require.Positive(t, filled, "the first run fills the shared directory")
	second, secondState := run("clone-b")
	assert.Equal(t, filled, logs(), "the second clone reads what the first recorded")
	assert.Equal(t, first, second, "and reports the same")
	assert.NoDirExists(t, filepath.Join(firstState, "work", "cache", "docs"))
	assert.NoDirExists(t, filepath.Join(secondState, "work", "cache", "docs"))
}

// A relative KAPI_PARSE_CACHE_DIR is refused with a message, and nothing is
// created against the working directory.
func TestARelativeParseCacheDirectoryIsRefused(t *testing.T) {
	_, recipe := writeBareProject(t, "relative")
	out, err := kapiEnv(t, []string{"KAPI_PARSE_CACHE_DIR=kapi-parse"}, "status", "-p", recipe)
	require.Error(t, err, out)
	assert.Contains(t, out, "KAPI_PARSE_CACHE_DIR must be an absolute path")
}
