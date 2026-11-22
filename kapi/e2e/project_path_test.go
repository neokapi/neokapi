//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A -p that names no project fails every project command with an error that
// names the path the person gave, and the same exit code. It never reaches
// a store or the work directory: a missing path once surfaced as
// "projectdb: create work dir: mkdir /.kapi".
func TestProjectFlagNamesAPathThatIsNotAProject(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nowhere")
	empty, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	commands := [][]string{
		{"memory", "lookup", "Settings", "-t", "fr"},
		{"terms", "lookup", "password", "-t", "fr"},
		{"status"},
		{"up"},
		{"check"},
		{"store", "import", "."},
	}
	for _, tc := range []struct {
		name, path, says string
	}{
		{"missing path", missing, missing + " does not exist"},
		{"directory with no recipe", empty, empty + " is not a kapi project"},
	} {
		for _, args := range commands {
			t.Run(tc.name+"/"+args[0]+" "+args[1%len(args)], func(t *testing.T) {
				cmd := exec.Command(kapiBin, append(args, "-p", tc.path)...)
				cmd.Dir = empty
				cmd.Env = append(os.Environ(), isoEnv...)
				out, err := cmd.CombinedOutput()
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, "kapi %v must fail:\n%s", args, out)
				assert.Equal(t, 1, exit.ExitCode(), "kapi %v exit code:\n%s", args, out)
				assert.Contains(t, string(out), "-p "+tc.says, "kapi %v names the path:\n%s", args, out)
				assert.NotContains(t, string(out), "/.kapi", "kapi %v never reaches a work directory:\n%s", args, out)
			})
		}
	}
}

// A -p that names a file that is not a recipe fails with an error naming the
// file.
func TestProjectFlagNamesAFileThatIsNotARecipe(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	notes := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(notes, []byte("- a list\n- not a recipe\n"), 0o644))

	for _, args := range [][]string{{"status"}, {"memory", "lookup", "Settings", "-t", "fr"}} {
		cmd := exec.Command(kapiBin, append(args, "-p", notes)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), isoEnv...)
		out, err := cmd.CombinedOutput()
		require.Error(t, err, "kapi %v must fail:\n%s", args, out)
		assert.Contains(t, string(out), notes, "kapi %v names the file:\n%s", args, out)
	}
}

// A -p that is a symbolic link to a recipe roots the project at the recipe's
// own directory, where its content is.
func TestProjectFlagFollowsALinkToTheRecipe(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	project := filepath.Join(root, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(project, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, "src", "messages_en.json"), []byte(`{"a": "Hello"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(project, "kapi.yaml"), []byte(`version: v1
name: linked
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - name: app
    path: src/messages_en.json
    target: src/messages_{lang}.json
`), 0o644))
	link := filepath.Join(root, "linked.yaml")
	require.NoError(t, os.Symlink(filepath.Join(project, "kapi.yaml"), link))

	cmd := exec.Command(kapiBin, "status", "-p", link, "--json")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), isoEnv...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.NotContains(t, string(out), "declares no content", "%s", out)
	assert.Contains(t, string(out), "fr", "the project's target language is read:\n%s", out)
}

// A read command outside a project never creates a store: a lookup in a
// directory with no store says there is none and leaves the directory as it
// was.
func TestStoreReadsCreateNoStore(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for _, args := range [][]string{
		{"memory", "lookup", "Settings", "-t", "fr"},
		{"memory", "search", "Settings"},
		{"memory", "stats"},
		{"terms", "lookup", "password", "-t", "fr"},
		{"terms", "search", "password"},
		{"terms", "stats"},
	} {
		cmd := exec.Command(kapiBin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), isoEnv...)
		out, err := cmd.CombinedOutput()
		require.Error(t, err, "kapi %v must fail:\n%s", args, out)
		assert.Contains(t, string(out), "there is no", "kapi %v says there is no store:\n%s", args, out)
		entries, rerr := os.ReadDir(dir)
		require.NoError(t, rerr)
		assert.Empty(t, entries, "kapi %v created a file", args)
	}
}

// A read command in a project nothing has opened yet creates nothing: no work
// directory and no workspace entry.
func TestStoreReadsInAFreshProjectCreateNothing(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(`version: v1
name: fresh
defaults:
  source_language: en
  target_languages: [fr]
`), 0o644))
	for _, args := range [][]string{
		{"memory", "lookup", "Hello", "-t", "fr"},
		{"terms", "lookup", "hello", "-t", "fr"},
	} {
		cmd := exec.Command(kapiBin, append(args, "-p", ".")...)
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), isoEnv...), "KAPI_DATA_DIR="+dataRoot)
		out, err := cmd.CombinedOutput()
		require.Error(t, err, "kapi %v must fail:\n%s", args, out)
		assert.Contains(t, string(out), "yet", "kapi %v says the project has no store yet:\n%s", args, out)
		assert.NoDirExists(t, filepath.Join(dir, ".kapi"), "kapi %v created a work directory", args)
		entries, rerr := os.ReadDir(dataRoot)
		require.NoError(t, rerr)
		assert.Empty(t, entries, "kapi %v wrote to the data root", args)
	}
}
