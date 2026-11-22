//go:build unix

package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A -o destination beneath a link out of the project is refused, and no
// directory is created through the link first: the file home makes the
// destination's directories at commit, after the check.
func TestRunSingleFileCreatesNoDirectoryThroughALinkOut(t *testing.T) {
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{"en.json": `{"greeting":"Hello"}`})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	a.ProjectContext = project.NewProjectContext(proj, recipe)
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "out2")))
	a.TargetLang = "qps"
	require.NoError(t, cmd.Flags().Set("output", filepath.Join(dir, "out2", "newdir", "x.json")))

	err = a.RunSingleFile(context.Background(), cmd, "pseudo-translate", filepath.Join(dir, "src", "en.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out2")
	assert.NoDirExists(t, filepath.Join(outside, "newdir"), "no directory is made through the link")
}
