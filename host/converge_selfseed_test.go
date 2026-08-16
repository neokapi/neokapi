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

// newSelfSeedProject writes a fresh-clone-shaped project: a source file, a
// committed content-memory bundle holding the reviewed target for every string
// in it, `materialize: on-converge`, and a recycle-only flow (no provider, no
// key, no spend). Nothing is compiled — the store is as empty as it is after
// `git clone`.
//
// It runs under the dogfood isolation contract (CLAUDE.md): every root this run
// could otherwise inherit is pinned to a throwaway dir and project discovery is
// off, so the repo's own recipe can never be found.
func newSelfSeedProject(t *testing.T) (*App, *EnvCommand, string) {
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
	memDir := project.LayoutAt(dir).MemoryDir()
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

// TestConverge_SelfSeedsCommittedContext is the fresh-clone criterion: a
// checkout with no credentials and no store converges off the committed content
// memory alone, and `materialize: on-converge` writes the target file. Before
// the up path compiled the committed sources, the same run found an empty store,
// recycled nothing, and left the locale at source fallback.
func TestConverge_SelfSeedsCommittedContext(t *testing.T) {
	a, cmd, recipe := newSelfSeedProject(t)
	out := runFreshConverge(t, a, cmd, recipe)

	require.Len(t, out.Locales, 1)
	assert.Equal(t, 100, out.Locales[0].Pct["translated"], "the committed content memory filled the locale")
	assert.Positive(t, out.MaterializedFiles, "materialize: on-converge wrote the target file")

	target, err := os.ReadFile(filepath.Join(filepath.Dir(recipe), "src", "nb.json"))
	require.NoError(t, err)
	assert.Contains(t, string(target), "Hei verden")
}

// TestUpPlan_DoesNotCreateTheStore: `kapi up --plan` is a dry run, so it must
// not leave a project store behind. Seeding on the plan path is therefore
// conditional on a store already being there — computeProjectPlan stats rather
// than opens for the same reason, because opening runs every subsystem's
// migrations and creates the file.
func TestUpPlan_DoesNotCreateTheStore(t *testing.T) {
	a, cmd, recipe := newSelfSeedProject(t)
	require.NoError(t, cmd.Flags().Set("plan", "true"))

	require.NoError(t, a.ExecuteUp(cmd, recipe))

	assert.NoFileExists(t, project.LayoutAt(filepath.Dir(recipe)).StorePath(),
		"a dry run created a project store")
}

// TestUpPlan_ColdCheckoutPricesTheCommittedCorpus: the leverage a plan reports
// is the number a reviewer approves spend against, and on a fresh checkout the
// corpus the run will recycle from is the one git carries, not the one the
// store holds — the store does not exist yet. A plan that read only the store
// reported no recycling at all and priced every unit as AI work, on precisely
// the leg (a pull-request CI job) that never runs anything before it.
func TestUpPlan_ColdCheckoutPricesTheCommittedCorpus(t *testing.T) {
	a, _, recipe := newSelfSeedProject(t)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	plan, err := a.computeProjectPlan(context.Background(), proj, recipe)
	require.NoError(t, err)

	assert.Equal(t, 1, plan.Totals.MissingTarget)
	assert.Equal(t, 1, plan.Totals.MemoryExact, "the committed bundle answers the only unit")
	assert.Equal(t, 0, plan.Totals.AIRemaining, "so nothing is left for a provider")
	assert.Zero(t, plan.Totals.TokenEstimate, "and there are no tokens to quote")
	assert.NoFileExists(t, project.LayoutAt(filepath.Dir(recipe)).StorePath(),
		"the committed corpus was read without creating a project store")
}

// TestUpPlan_ColdCheckoutLeavesNoTraceOfTheCorpusItRead: the scratch corpus the
// cold plan assembles is discarded with the call. Nothing under `.kapi/` moves,
// so the next command finds the checkout exactly as git left it.
func TestUpPlan_ColdCheckoutLeavesNoTraceOfTheCorpusItRead(t *testing.T) {
	a, cmd, recipe := newSelfSeedProject(t)
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

// TestUpPlan_SeedsAnExistingStore: once a store is there, the plan seeds it, so
// the leverage figure a user approves spend against reflects what git carries
// rather than what the last run happened to leave behind.
func TestUpPlan_SeedsAnExistingStore(t *testing.T) {
	a, cmd, recipe := newSelfSeedProject(t)
	root := filepath.Dir(recipe)
	ctx := context.Background()

	// A store exists but predates the committed bundle.
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	require.NoError(t, db.PutMeta(ctx, "seed-test", "1"))
	before, err := db.Memory().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, before)

	require.NoError(t, cmd.Flags().Set("plan", "true"))
	require.NoError(t, a.ExecuteUp(cmd, recipe))

	after, err := db.Memory().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, after, "the plan seeded the existing store from the committed bundle")
}

// TestConverge_SeedsAPulledSourceEdit: a `git pull` that changes a committed
// bundle reaches the store on the next run, without an explicit import.
func TestConverge_SeedsAPulledSourceEdit(t *testing.T) {
	a, cmd, recipe := newSelfSeedProject(t)
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
		filepath.Join(project.LayoutAt(root).MemoryDir(), "app-nb.memory.json"), data, 0o644))

	seeded, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, seeded.MemoryFiles, "the pulled edit recompiled")

	db, err := a.ProjectDB(context.Background(), root)
	require.NoError(t, err)
	entry, found, err := db.Memory().GetEntry(context.Background(), "reviewed:greeting")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "God dag verden", entry.VariantText("nb"))
}
