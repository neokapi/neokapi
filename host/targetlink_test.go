//go:build unix

package host

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// A target that is a link out of the project stops neither the plan, the
// convergence, status nor the merge: kapi reads and writes nothing through
// it, every other file is written, the source is named with
// check.WarningTargetOutside, and kapi merge exits non-zero once the rest is
// written. The file the link names keeps what it held.
func TestATargetThatLinksOutIsNamedAndTheRestIsWritten(t *testing.T) {
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{
		"app.json":  `{"greeting":"Hello world"}`,
		"menu.json": `{"open":"Open the file"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Collections[0].Target = "out/{lang}/{name}.json"
	require.NoError(t, project.Save(recipe, proj))

	outside := filepath.Join(t.TempDir(), "kept.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"greeting":"kept"}`), 0o644))
	link := filepath.Join(dir, "out", "nb", "app.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(outside, link))

	named := func(ws []check.Warning) bool {
		for _, w := range ws {
			if w.Code == check.WarningTargetOutside && w.Source == filepath.Join("src", "app.json") {
				return true
			}
		}
		return false
	}

	t.Run("status and the plan name the file", func(t *testing.T) {
		ws := a.unsafeTargetWarnings(proj, dir, "")
		assert.True(t, named(ws), "%+v", ws)
		plan, err := a.UpPlan(context.Background(), recipe, "")
		require.NoError(t, err, "the plan is computed over the other files")
		assert.True(t, named(plan.Warnings), "%+v", plan.Warnings)
	})

	out := converge(t, a, cmd, recipe)
	t.Run("the convergence writes the rest and names the file", func(t *testing.T) {
		assert.True(t, named(out.Warnings), "%+v", out.Warnings)
		assert.FileExists(t, filepath.Join(dir, "out", "nb", "menu.json"))
		assert.FileExists(t, filepath.Join(dir, "out", "de", "app.json"))
	})

	t.Run("merge writes the rest and fails naming the file", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(filepath.Join(dir, "out", "de")))
		require.NoError(t, os.Remove(filepath.Join(dir, "out", "nb", "menu.json")))
		written, stale, merr := a.materializeProject(context.Background(), io.Discard, proj, recipe, []model.LocaleID{"nb", "de"}, true, nil)
		require.NoError(t, merr)
		assert.Equal(t, 3, written)
		assert.True(t, named(stale.Unnamed), "%+v", stale.Unnamed)
		require.ErrorContains(t, unsafeTargetsError(stale.Unnamed), "app.json")
	})

	got, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.JSONEq(t, `{"greeting":"kept"}`, string(got), "nothing was written through the link")
}
