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
