package host

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFreshCheckoutProject writes a fresh-clone-shaped project: a source file, a
// committed content-memory bundle holding the reviewed target for every string
// in it, `materialize: on-converge`, and a recycle-only flow (no provider, no
// key, no spend). The bundle is read by nobody, so the store is as empty as it
// is after `git clone`.
//
// It runs under the dogfood isolation contract (CLAUDE.md): every root this run
// could otherwise inherit is pinned to a throwaway dir and project discovery is
// off, so the repo's own recipe can never be found.
func newFreshCheckoutProject(t *testing.T) (*App, *EnvCommand, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"greeting":"Hello world"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "SelfSeedTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
			Flow:            "recycle-only",
			SourceGate:      string(model.SourceGateNone),
			Materialize:     project.MaterializeOnConverge,
		},
		Collections: []project.Collection{
			{Name: "app", Path: "src/en.json", Target: "src/{lang}.json"},
		},
		Flows: map[string]*flow.StepsSpec{
			"recycle-only": {Steps: []flow.FlowStep{
				{Tool: "recycle", Config: map[string]any{"fillTarget": true, "fillTargetThreshold": 100}},
			}},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))

	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	data, err := kmb.Marshal(kmb.FromModel([]memory.Entry{{
		ID:          "reviewed:greeting",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello world"}}},
			"nb": {{Text: &model.TextRun{Text: "Hei verden"}}},
		},
		CreatedAt: stamp,
		UpdatedAt: stamp,
	}}, nil))
	require.NoError(t, err)
	memDir := project.LayoutAt(dir).Export().MemoryDir()
	require.NoError(t, os.MkdirAll(memDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(memDir, "app-nb.memory.json"), data, 0o644))

	t.Chdir(dir)
	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddUpFlags(cmd)
	// Discovery is off under the isolation contract, so the fixture names its
	// own recipe explicitly — an explicit -p wins over KAPI_NO_PROJECT, and the
	// recycle tool resolves the project store from it exactly as a real run does.
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	return a, cmd, recipe
}

// runFreshConverge drives one converge run with extraction ON — the shape a
// fresh clone runs, where the block store has to be built from the working tree
// before anything can be recycled into it.
func runFreshConverge(t *testing.T, a *App, cmd *EnvCommand, recipe string) ConvergeOutput {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	var out ConvergeOutput
	require.NoError(t, a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate: true,
		MaxPasses: 3,
		noChecks:  true,
		capture:   &out,
	}))
	return out
}

// TestConverge_RecyclesWhatTheStoreHolds is the fresh-clone criterion under the
// store-only contract: a checkout with no credentials converges off the content
// memory its store holds, and `materialize: on-converge` writes the target
// file. A bundle git carries reaches that store through `kapi context import`.
func TestConverge_RecyclesWhatTheStoreHolds(t *testing.T) {
	a, cmd, recipe := newFreshCheckoutProject(t)
	readProjectContext(t, filepath.Dir(recipe))

	out := runFreshConverge(t, a, cmd, recipe)

	require.Len(t, out.Locales, 1)
	assert.Equal(t, 100, out.Locales[0].Pct["translated"], "the content memory in the store filled the locale")
	assert.Positive(t, out.MaterializedFiles, "materialize: on-converge wrote the target file")

	target, err := os.ReadFile(filepath.Join(filepath.Dir(recipe), "src", "nb.json"))
	require.NoError(t, err)
	assert.Contains(t, string(target), "Hei verden")
}

// TestConverge_AnUnreadBundleRecyclesNothing is the same checkout with the read
// left out: the run opens no context file, so the reviewed wording git carries
// governs nothing and the locale stays at source fallback.
func TestConverge_AnUnreadBundleRecyclesNothing(t *testing.T) {
	a, cmd, recipe := newFreshCheckoutProject(t)

	runFreshConverge(t, a, cmd, recipe)

	target, err := os.ReadFile(filepath.Join(filepath.Dir(recipe), "src", "nb.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(target), "Hei verden")
	assert.Contains(t, string(target), "Hello world", "the locale falls back to source")
}

// TestUpPlan_DoesNotCreateTheStore: `kapi up --plan` is a dry run, so it must
// leave no project store behind. computeProjectPlan stats rather than opens,
// because opening runs every subsystem's migrations and creates the file.
func TestUpPlan_DoesNotCreateTheStore(t *testing.T) {
	a, cmd, recipe := newFreshCheckoutProject(t)
	require.NoError(t, cmd.Flags().Set("plan", "true"))

	require.NoError(t, a.ExecuteUp(cmd, recipe))

	assert.NoFileExists(t, project.LayoutAt(filepath.Dir(recipe)).StorePath(),
		"a dry run created a project store")
}

// TestUpPlan_PricesWhatTheStoreHolds: the leverage a plan reports is the number
// a reviewer approves spend against, and it is the corpus the run will actually
// recycle from, which is the store's. A checkout whose bundle nobody has read
// in prices every unit as AI work, and reading it in moves the same unit to
// recycling.
func TestUpPlan_PricesWhatTheStoreHolds(t *testing.T) {
	a, _, recipe := newFreshCheckoutProject(t)
	root := filepath.Dir(recipe)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	cold, err := a.computeProjectPlan(context.Background(), proj, recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, cold.Totals.MissingTarget)
	assert.Equal(t, 0, cold.Totals.MemoryExact, "an unread bundle answers nothing")
	assert.Equal(t, 1, cold.Totals.OutOfReach,
		"and this flow drafts nothing, so the unit is out of reach rather than priced")
	assert.NoFileExists(t, project.LayoutAt(root).StorePath(),
		"pricing a checkout creates no project store")

	readProjectContext(t, root)

	warm, err := a.computeProjectPlan(context.Background(), proj, recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, warm.Totals.MissingTarget)
	assert.Equal(t, 1, warm.Totals.MemoryExact, "the content memory in the store answers the only unit")
	assert.Equal(t, 0, warm.Totals.AIRemaining, "so nothing is left for a provider")
	assert.Zero(t, warm.Totals.TokenEstimate, "and there are no tokens to quote")
}

// TestUpPlan_ColdCheckoutLeavesNoTrace: a plan over a checkout with no store
// moves nothing under `.kapi/`, so the next command finds the checkout exactly
// as git left it.
func TestUpPlan_ColdCheckoutLeavesNoTrace(t *testing.T) {
	a, cmd, recipe := newFreshCheckoutProject(t)
	root := filepath.Dir(recipe)
	before := stateTree(t, project.LayoutAt(root).StateDir)

	require.NoError(t, cmd.Flags().Set("plan", "true"))
	require.NoError(t, a.ExecuteUp(cmd, recipe))

	assert.Equal(t, before, stateTree(t, project.LayoutAt(root).StateDir))
}

// stateTree lists the project's `.kapi/` tree as project-relative paths, so a
// test can assert that a dry run moved nothing in it.
func stateTree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return rerr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	require.NoError(t, err)
	sort.Strings(out)
	return out
}

// TestUpPlan_ReadsNoBundleIntoAnExistingStore: a store that is already there is
// left as the plan found it. Pricing a run is a question about the project, and
// answering it must not put a checkout's bundle in force behind the person's
// back.
func TestUpPlan_ReadsNoBundleIntoAnExistingStore(t *testing.T) {
	a, cmd, recipe := newFreshCheckoutProject(t)
	root := filepath.Dir(recipe)
	ctx := context.Background()

	// A store exists and holds nothing the committed bundle carries.
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	require.NoError(t, db.PutMeta(ctx, "plan-test", "1"))
	before, err := db.Memory().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, before)

	require.NoError(t, cmd.Flags().Set("plan", "true"))
	require.NoError(t, a.ExecuteUp(cmd, recipe))

	after, err := db.Memory().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, after, "the plan read no committed bundle into the store")
}

// TestConverge_ReadsAPulledSourceEdit: a `git pull` that changes a committed
// bundle reaches the store when the import is run over the checkout again.
func TestConverge_ReadsAPulledSourceEdit(t *testing.T) {
	a, cmd, recipe := newFreshCheckoutProject(t)
	root := filepath.Dir(recipe)
	runConverge(t, a, cmd, recipe)

	// The reviewed wording changes in git.
	stamp := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	data, err := kmb.Marshal(kmb.FromModel([]memory.Entry{{
		ID:          "reviewed:greeting",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello world"}}},
			"nb": {{Text: &model.TextRun{Text: "God dag verden"}}},
		},
		CreatedAt: stamp,
		UpdatedAt: stamp,
	}}, nil))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(project.LayoutAt(root).Export().MemoryDir(), "app-nb.memory.json"), data, 0o644))

	seeded, err := a.seedContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, seeded.Entries, "the pulled edit was read in")

	db, err := a.ProjectDB(context.Background(), root)
	require.NoError(t, err)
	entry, found, err := db.Memory().GetEntry(context.Background(), "reviewed:greeting")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "God dag verden", entry.VariantText("nb"))
}
