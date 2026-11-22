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

// An embedded run (the desktop's, and every caller of RunFlowAllLocales)
// names no destination of its own: it writes the recipe's targets, and a
// target beneath a link out of the project is refused with nothing written.
func TestRunFlowAllLocalesWritesNothingOutsideTheProject(t *testing.T) {
	a, _, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	outside := t.TempDir()
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "out")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "out")))

	_, err = a.RunFlowAllLocales(context.Background(), FlowRunOptions{
		FlowName:    "translate",
		Project:     proj,
		ProjectPath: recipe,
		InputPaths:  []string{filepath.Join(dir, "src", "en.json")},
	}, func(FlowRunEvent) {})
	require.Error(t, err)
	require.ErrorContains(t, err, "leads outside the root")
	entries, rerr := os.ReadDir(outside)
	require.NoError(t, rerr)
	assert.Empty(t, entries, "nothing is written through the link")
}
