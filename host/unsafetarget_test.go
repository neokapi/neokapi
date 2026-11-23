package host

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// unsafeNameProject is a loop project whose target places {name} as a
// directory, holding one source named "...json", whose stem is "..".
func unsafeNameProject(t *testing.T) (*App, *EnvCommand, string, string) {
	t.Helper()
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{
		"app.json":  `{"greeting":"Hello world"}`,
		"...json":   `{"greeting":"Hello there"}`,
		"menu.json": `{"open":"Open the file"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Collections[0].Target = "out/{lang}/{name}/strings.json"
	require.NoError(t, project.Save(recipe, proj))
	return a, cmd, recipe, dir
}

// One source whose name cannot fill the target template stops neither the
// convergence nor the merge: every other file is written, the file is named
// in the run's warnings and in status, and kapi merge exits non-zero once
// the rest is written.
func TestAnUnsafeSourceNameIsNamedAndTheRestIsWritten(t *testing.T) {
	a, cmd, recipe, dir := unsafeNameProject(t)
	out := converge(t, a, cmd, recipe)
	named := func(ws []check.Warning) bool {
		for _, w := range ws {
			if w.Code == check.WarningUnsafeTargetName && w.Source == filepath.Join("src", "...json") {
				return true
			}
		}
		return false
	}
	for _, loc := range []string{"nb", "de"} {
		assert.FileExists(t, filepath.Join(dir, "out", loc, "app", "strings.json"))
		assert.FileExists(t, filepath.Join(dir, "out", loc, "menu", "strings.json"))
	}
	assert.NoFileExists(t, filepath.Join(dir, "out", "nb", "strings.json"), "nothing is written one level up")

	t.Run("merge writes the rest and fails naming the file", func(t *testing.T) {
		proj, err := project.Load(recipe)
		require.NoError(t, err)
		require.NoError(t, os.RemoveAll(filepath.Join(dir, "out")))
		written, stale, merr := a.materializeProject(context.Background(), io.Discard, proj, recipe, []model.LocaleID{"nb", "de"}, true, nil)
		require.NoError(t, merr)
		assert.Equal(t, 4, written)
		assert.True(t, named(stale.Unnamed), "%+v", stale.Unnamed)
		require.Error(t, unsafeTargetsError(stale.Unnamed))
		assert.Contains(t, unsafeTargetsError(stale.Unnamed).Error(), "...json")
	})

	t.Run("status names the file", func(t *testing.T) {
		proj, err := project.Load(recipe)
		require.NoError(t, err)
		ws := a.unsafeTargetWarnings(proj, dir, "")
		assert.True(t, named(ws), "%+v", ws)
		var b strings.Builder
		require.NoError(t, StatusOutput{Warnings: ws, Monolingual: true}.FormatText(&b))
		assert.Contains(t, b.String(), "...json")
	})

	t.Run("the convergence names the file", func(t *testing.T) {
		assert.True(t, named(out.Warnings), "%+v", out.Warnings)
	})
}
