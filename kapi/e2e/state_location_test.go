//go:build e2e && unix

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// kapiAtTerminal runs kapi on a terminal, as a person does, through script(1),
// which gives the command a pseudo-terminal. It returns everything the command
// wrote there. The one-time notices about a checkout's state are shown, and
// recorded as given, only on a terminal.
func kapiAtTerminal(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("script", append([]string{"-q", "/dev/null", kapiBin}, args...)...)
	default:
		quoted := make([]string, 0, len(args)+1)
		for _, a := range append([]string{kapiBin}, args...) {
			quoted = append(quoted, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
		}
		cmd = exec.Command("script", "-qec", strings.Join(quoted, " "), "/dev/null")
	}
	// A dumb terminal answers no colour query, so kapi asks none and waits for
	// no reply.
	cmd.Env = append(append(os.Environ(), isoEnv...), "TERM=dumb", "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	return strings.ReplaceAll(string(out), "\r\n", "\n"), err
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

// A repository of another project cloned where a checkout was sees none of
// that checkout's state, and kapi tells the person at the terminal, once,
// that the state belongs to another project.
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
	const notice = `belongs to "first", another project, so this checkout starts with none`
	out, err := kapiAtTerminal(t, "status", "-p", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, notice)
	assert.NotEqual(t, earlier, statePath(t, recipe))
	assert.NoFileExists(t, filepath.Join(statePath(t, recipe), "sync", "sync-cache.json"))

	out, err = kapiAtTerminal(t, "status", "-p", recipe)
	require.NoError(t, err, out)
	assert.NotContains(t, out, notice, "told once")
}

// State an earlier kapi left inside a checkout is not read. A person imports
// it with `kapi state import`, which shows what it takes; an agent cannot.
// The person is told at a terminal; a run whose output a program reads, or a
// `--json` run, tells nobody and leaves the notice owed.
func TestStateInsideACheckoutIsImportedOnlyByAPerson(t *testing.T) {
	root, recipe := writeBareProject(t, "legacy")
	vault := filepath.Join(root, ".kapi", "vault", "redaction.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(vault), 0o700))
	require.NoError(t, os.WriteFile(vault, []byte(`{"version":3,"values":[]}`), 0o600))
	notice := "kapi state import -p " + root

	out, err := kapiAllowFail(t, "status", "-p", recipe)
	require.NoError(t, err, out)
	assert.NotContains(t, out, notice, "a run whose output a program reads tells nobody")
	out, err = kapiAtTerminal(t, "version")
	require.NoError(t, err, out)
	assert.NotContains(t, out, notice, "a version check concerns no project")
	out, err = kapiAtTerminal(t, "status", "-p", recipe, "--json")
	require.NoError(t, err, out)
	assert.NotContains(t, out, notice, "a --json run tells nobody")

	out, err = kapiAtTerminal(t, "status", "-p", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, notice)
	out, err = kapiAtTerminal(t, "status", "-p", recipe)
	require.NoError(t, err, out)
	assert.NotContains(t, out, notice, "told once")
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

// A copied checkout starts without the state; relink takes it across. The
// recipe has no id, so kapi cannot say the state is this project's: the vault
// stays until the person states it is, with --same-project.
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
	assert.Contains(t, got, "pass --same-project if it does")
	assert.NoFileExists(t, filepath.Join(statePath(t, copyRecipe), "vault", "redaction.json"))

	got, err = kapiAllowFail(t, "state", "relink", "--from", root, "--yes", "--same-project", "-p", copyRecipe)
	require.NoError(t, err, got)
	assert.FileExists(t, filepath.Join(statePath(t, copyRecipe), "vault", "redaction.json"))
}

// A repository of another project cloned where a checkout was does not take
// that checkout's state, and kapi names both projects.
func TestRelinkRefusesAnotherProjectsState(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "app")
	first, _ := writeBareProject(t, "billing")
	require.NoError(t, os.Rename(first, root))
	recipe := filepath.Join(root, "kapi.yaml")
	kapi(t, "status", "-p", recipe)
	earlier := statePath(t, recipe)
	require.NoError(t, os.MkdirAll(filepath.Join(earlier, "vault"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(earlier, "vault", "redaction.json"), []byte(`{"version":3,"values":[]}`), 0o600))

	require.NoError(t, os.RemoveAll(root))
	second, _ := writeBareProject(t, "docs")
	require.NoError(t, os.Rename(second, root))
	out, err := kapiAllowFail(t, "state", "relink", "--from", root, "--yes", "--same-project", "-p", recipe)
	require.Error(t, err, out)
	assert.Contains(t, out, `belongs to "billing", and this checkout is "docs"`)
	assert.NoFileExists(t, filepath.Join(statePath(t, recipe), "vault", "redaction.json"))
	assert.FileExists(t, filepath.Join(earlier, "vault", "redaction.json"))
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
