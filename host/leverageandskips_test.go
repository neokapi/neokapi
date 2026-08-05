package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1474 item 2. `kapi up --plan` quotes token and cost figures computed against
// the project content memory. A store that would not open left it nil, every unit reported
// zero recycling leverage, and the plan said "everything needs AI translation"
// for a project the memory already largely covered. The user approves a far larger
// and more expensive run than reality warrants, off a number that looks
// authoritative. Costing money on a wrong number is worse than an error.
//
// The discrimination this needs was already sitting in the code, unused: the
// os.Stat guard exists because a plan must not create the project store, so an
// ABSENT store is the legitimate no-leverage case. Past the stat, a failure to
// open can only mean present-but-unreadable.

func newPlanProject(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	// Dogfood isolation contract (CLAUDE.md).
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"greeting":"Hello world","farewell":"Goodbye now"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "PlanLeverageTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			SourceGate:      string(model.SourceGateNone),
		},
		Content: []project.ContentCollection{
			{Name: "app", Path: "src/en.json", Target: "src/{lang}.json"},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	return a, recipe
}

// TestComputeProjectPlan_UnopenableStoreFailsThePlan is the regression: a store that
// exists and cannot be read must not be quoted as "no leverage".
func TestComputeProjectPlan_UnopenableStoreFailsThePlan(t *testing.T) {
	a, recipe := newPlanProject(t)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)

	// A real seam: a file standing where the project store must be, whose bytes
	// are not a database. No production hook, no test-only branch.
	storePath := layout.StorePath()
	require.NoError(t, os.WriteFile(storePath, []byte("this is not a sqlite database at all"), 0o644))

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	_, perr := a.computeProjectPlan(t.Context(), proj, recipe)
	require.Error(t, perr,
		"a content memory that cannot be read must not be reported as a project with no leverage — "+
			"the plan's cost estimate is computed against it")
	assert.Contains(t, perr.Error(), storePath, "the message must name the store")
	assert.Contains(t, perr.Error(), "overstate the spend")
}

// TestComputeProjectPlan_AbsentStorePlansAtZeroLeverage is the discriminating
// control, and it pins the invariant the merged store made sharper: a plan never
// creates `.kapi/store.db`.
//
// Opening the store is what creates it — the handle runs every subsystem's
// migrations at open — so the guard cannot be "open it and see". Most projects
// have no store on their first plan, so "no store present" is the ordinary case,
// and it must stay silent AND leave the state directory as it found it. Without
// this, "fail whenever mem is nil" would pass the test above.
func TestComputeProjectPlan_AbsentStorePlansAtZeroLeverage(t *testing.T) {
	a, recipe := newPlanProject(t)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoFileExists(t, layout.StorePath())

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	plan, perr := a.computeProjectPlan(t.Context(), proj, recipe)
	require.NoError(t, perr, "a project with no store yet must still plan")
	assert.Positive(t, plan.Totals.AIRemaining, "with no content memory every missing unit is AI work")
	assert.Zero(t, plan.Totals.MemoryExact, "no content memory means no exact-hash leverage")

	// And the plan must not have created the store it declined to open.
	assert.NoFileExists(t, layout.StorePath(),
		"a plan must never create the project store — that is why the stat guard exists")
}

// TestComputeProjectPlan_HealthyStoreStillPlans: a usable store is opened and used.
func TestComputeProjectPlan_HealthyStoreStillPlans(t *testing.T) {
	a, recipe := newPlanProject(t)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)

	// A real, empty-but-valid project store, created the way kapi creates it.
	db, derr := projectdb.Open(t.Context(), layout)
	require.NoError(t, derr)
	require.NoError(t, db.Close())
	require.FileExists(t, layout.StorePath())

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	_, perr := a.computeProjectPlan(t.Context(), proj, recipe)
	require.NoError(t, perr, "a readable store must plan normally")
}

// RunExtractKpz's leverage open is the other half of item 2, and it takes the
// opposite decision on purpose: the .kpz packages are still usable without
// leverage, so a store that will not open is REPORTED and the extract continues
// — matching the two siblings on the same call in that file (RunExtract,
// MergeOneKpz), whose convention #1466 established. What was wrong was the
// silence, not the continuing: a translator saw an empty recycling context and
// no vocabulary on a project that has both, with nothing to explain it.
func TestRunExtractKpz_UnopenableMemoryAndTermsAreReported(t *testing.T) {
	a, recipe := newPlanProject(t)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)

	// A real seam: a file standing where the project store must be, whose bytes
	// are not a database. Opening CREATES the store when absent, so a failure to
	// open can only mean present-but-unreadable — which is exactly why the absent
	// case cannot be used as the control here, and why the assertion below is
	// about the message rather than about the extract failing.
	require.NoError(t, os.WriteFile(layout.StorePath(), []byte("not a sqlite database"), 0o644))

	var stderr bytes.Buffer
	cmd := NewEnvCommand(t.Context(), "extract")
	cmd.SetErr(&stderr)
	addKpzExtractFlags(cmd, recipe)

	err = a.RunExtractKpz(cmd)
	require.NoError(t, err, "the packages are still usable without leverage, so the extract continues")

	out := stderr.String()
	assert.Contains(t, out, "open project store", "the store failure must be reported, not swallowed")
	assert.Contains(t, out, "zero recycling",
		"and it must say what the user will see as a result")
	assert.Contains(t, out, "no vocabulary",
		"the vocabulary loss must be named too — one store now carries both")
}

// TestRunExtractKpz_HealthyStoresReportNothing is the control: a project whose
// stores open cleanly must produce no warning at all, so "warn whenever the
// stores are consulted" is not an acceptable substitute.
func TestRunExtractKpz_HealthyStoresReportNothing(t *testing.T) {
	a, recipe := newPlanProject(t)

	var stderr bytes.Buffer
	cmd := NewEnvCommand(t.Context(), "extract")
	cmd.SetErr(&stderr)
	addKpzExtractFlags(cmd, recipe)

	require.NoError(t, a.RunExtractKpz(cmd))
	assert.NotContains(t, stderr.String(), "Warning:",
		"stores that open cleanly must produce no warning")
}

// #1474 item 4. host/inputs.go dropped a glob match whose stat failed without
// calling skip(), unlike expandOne two functions up, which reports an unreadable
// file named explicitly — and resolveFiles turns the first skip into a non-zero
// exit. So whether the user was told depended only on whether they typed a
// wildcard: `kapi stats 'src/**'` silently processed N-1 of N files and exited 0,
// invisible to --on-skip and to the exit code, while `kapi stats src/locked.json`
// failed loudly on the same file.

// TestExpandGlobArg_UnreadableMatchIsReported is the regression.
func TestExpandGlobArg_UnreadableMatchIsReported(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"a":"b"}`), 0o644))
	broken := filepath.Join(dir, "b.json")
	require.NoError(t, os.Symlink(filepath.Join(dir, "never-existed.json"), broken))

	var skipped []string
	files, err := expandGlobArg(filepath.Join(dir, "*.json"), InputOptions{
		OnSkip: func(path string, reason error) { skipped = append(skipped, path) },
	})
	require.NoError(t, err, "one unreadable match must not fail the whole expansion")
	assert.Len(t, files, 1, "the readable file is still processed")
	require.Len(t, skipped, 1,
		"the unreadable match must be reported through the same skip channel an "+
			"explicitly named unreadable file uses")
	assert.Equal(t, broken, skipped[0], "the skip must name the file")
}

// TestExpandGlobArg_OrdinaryGlobReportsNoSkips is the control: expansion of a
// healthy tree must not start reporting skips.
func TestExpandGlobArg_OrdinaryGlobReportsNoSkips(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.json", "b.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(`{"a":"b"}`), 0o644))
	}
	var skipped []string
	files, err := expandGlobArg(filepath.Join(dir, "*.json"), InputOptions{
		OnSkip: func(path string, reason error) { skipped = append(skipped, path) },
	})
	require.NoError(t, err)
	assert.Len(t, files, 2)
	assert.Empty(t, skipped, "a healthy glob reports nothing")
}

// addKpzExtractFlags registers the flags RunExtractKpz reads. The cobra command
// lives in the cli module, so a host-level test wires the same flag set by hand.
func addKpzExtractFlags(cmd *EnvCommand, recipe string) {
	AddProjectFlag(cmd)
	cmd.Flags().Bool("no-tm", false, "")
	cmd.Flags().String("only", "", "")
	cmd.Flags().String("pattern", "", "")
	cmd.Flags().String("out-dir", "out", "")
	cmd.Flags().String("target-lang", "", "")
	_ = cmd.Flags().Set("project", recipe)
}
