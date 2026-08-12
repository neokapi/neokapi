package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRecordConvergeProject writes a fresh-clone-shaped project with NO seed
// bundle at all: one collection whose target is committed (the record), and one
// whose target is not yet written but whose source repeats the same strings. The
// second collection can only be filled from what the first one's committed
// translation teaches, so it is the evidence that the record — not a seed —
// rebuilt the memory.
//
// It runs under the dogfood isolation contract (CLAUDE.md): every root this run
// could otherwise inherit is pinned to a throwaway dir and project discovery is
// off, so the repo's own recipe can never be found.
func newRecordConvergeProject(t *testing.T) (*App, *EnvCommand, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "rec"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "new"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rec", "en.json"),
		[]byte(`{"greeting":"Hello world","farewell":"Goodbye"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rec", "nb.json"),
		[]byte(`{"greeting":"Hei verden","farewell":"Ha det"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new", "en.json"),
		[]byte(`{"hello":"Hello world","bye":"Goodbye"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "RecordConvergeTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
			Flow:            "recycle-only",
			SourceGate:      string(model.SourceGateNone),
			Materialize:     project.MaterializeOnConverge,
		},
		Collections: []project.Collection{
			{Name: "committed", Path: "rec/en.json", Target: "rec/{lang}.json"},
			{Name: "pending", Path: "new/en.json", Target: "new/{lang}.json"},
		},
		Flows: map[string]*flow.StepsSpec{
			"recycle-only": {Steps: []flow.FlowStep{
				{Tool: "recycle", Config: map[string]any{"fillTarget": true, "fillTargetThreshold": 100}},
			}},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))

	t.Chdir(dir)
	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddUpFlags(cmd)
	// Discovery is off under the isolation contract, so the fixture names its
	// own recipe explicitly — an explicit -p wins over KAPI_NO_PROJECT.
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	return a, cmd, recipe
}

// TestConverge_RebuildsFromTheCommittedRecord is the other half of the
// fresh-clone criterion: a checkout with no credentials, no store and no seed
// converges off the committed translations alone. Before they were absorbed, the
// reviewed wording in `rec/nb.json` was readable by a person and by nothing in
// the pipeline: the run found an empty store, recycled nothing, and left the
// second collection at source fallback.
func TestConverge_RebuildsFromTheCommittedRecord(t *testing.T) {
	a, cmd, recipe := newRecordConvergeProject(t)
	out := runFreshConverge(t, a, cmd, recipe)

	require.Len(t, out.Locales, 1)
	assert.Equal(t, 100, out.Locales[0].Pct["translated"],
		"the committed translations filled both collections")

	target, err := os.ReadFile(filepath.Join(filepath.Dir(recipe), "new", "nb.json"))
	require.NoError(t, err)
	assert.Contains(t, string(target), "Hei verden")
	assert.Contains(t, string(target), "Ha det")
}

// TestConverge_LeavesACompleteTargetAloneWhenTheStoreHasNothing is the
// fresh-clone catastrophe as a regression test. A checkout whose targets are
// complete gives the loop nothing pending, so it correctly runs no pass — and
// the block store, being cold, holds no target for the locale. Materializing
// from it then writes source fallback over every committed translation: the
// whole locale, erased by a run that reported itself up to date.
//
// Materializing means writing what the store produced. A store with nothing to
// say writes nothing.
func TestConverge_LeavesACompleteTargetAloneWhenTheStoreHasNothing(t *testing.T) {
	a, cmd, recipe := newRecordConvergeProject(t)
	root := filepath.Dir(recipe)
	// Give the second collection its target too, so nothing at all is pending.
	require.NoError(t, os.WriteFile(filepath.Join(root, "new", "nb.json"),
		[]byte(`{"hello":"Hei verden","bye":"Ha det"}`), 0o644))
	before, err := os.ReadFile(filepath.Join(root, "rec", "nb.json"))
	require.NoError(t, err)

	out := runFreshConverge(t, a, cmd, recipe)
	assert.Zero(t, out.Passes, "nothing is pending, so no pass runs")
	assert.Zero(t, out.MaterializedFiles, "a store with no target for the locale writes nothing")

	after, err := os.ReadFile(filepath.Join(root, "rec", "nb.json"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the committed translation is untouched")
}

// TestConverge_DoesNotAbsorbItsOwnOutput: the targets a run materializes are the
// store's own output, and reading them back as if a person had committed them
// would let a stale materialization overwrite whatever the store learned since —
// a seed edit pulled in the meantime, an approval. The run stamps what it wrote,
// so the next pass sees nothing new in it.
func TestConverge_DoesNotAbsorbItsOwnOutput(t *testing.T) {
	a, cmd, recipe := newRecordConvergeProject(t)
	runFreshConverge(t, a, cmd, recipe)
	require.FileExists(t, filepath.Join(filepath.Dir(recipe), "new", "nb.json"),
		"the run materialized a target that did not exist when it started")

	res, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Zero(t, res.Record.Documents, "nothing the run wrote is read back")
	assert.Equal(t, 2, res.Record.Skipped, "both targets are stamped as already absorbed")
}

// TestConverge_CommittedRecordSurvivesAWipedStore: the store is derived state, so
// deleting it must cost a rebuild and nothing else. A second converge over a
// wiped store reproduces the same targets from the same committed documents.
func TestConverge_CommittedRecordSurvivesAWipedStore(t *testing.T) {
	a, cmd, recipe := newRecordConvergeProject(t)
	runFreshConverge(t, a, cmd, recipe)

	root := filepath.Dir(recipe)
	pending := filepath.Join(root, "new", "nb.json")
	first, err := os.ReadFile(pending)
	require.NoError(t, err)

	a.Shutdown()
	require.NoError(t, os.RemoveAll(project.LayoutAt(root).WorkDir()))
	require.NoError(t, os.Remove(pending))

	b := &App{}
	t.Cleanup(b.Shutdown)
	b.InitRegistries()
	b.SourceLang = "en"
	out := runFreshConverge(t, b, cmd, recipe)
	require.Len(t, out.Locales, 1)
	assert.Equal(t, 100, out.Locales[0].Pct["translated"])

	second, err := os.ReadFile(pending)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second), "a wiped store is a rebuild, not data loss")
}
