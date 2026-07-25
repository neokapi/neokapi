package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1449. A target path that cannot hold a localized file used to be measured as
// a COMPLETE translation: a directory named `fr-FR.json` stats clean, the format
// read then failed, that failure was classified as "target format is not
// readable", and coverage counts an unreadable-but-present target as fully
// `translated` by file-presence. So the locale read 100%, never became pending,
// the convergence loop never ran it, no write was ever attempted — and `kapi up`
// reported "Up to date: every gated scope is shippable" over an empty directory
// with no translation anywhere. This is the fixture the #1442 convergence-state
// work could not build, because the failure was not observable.

// newBlockedTargetProject writes a two-locale project whose flow translates via
// the deterministic demo provider (no network, no key, no spend), and returns
// the app, a flag-carrying command, the recipe path, and the project root.
func newBlockedTargetProject(t *testing.T, targetTemplate string) (*App, *EnvCommand, string, string) {
	t.Helper()
	dir := t.TempDir()
	// Dogfood isolation contract (CLAUDE.md): this test runs a real converge from
	// a cwd inside a temp dir, so pin every root it could otherwise inherit — the
	// developer's ~/.config/kapi, the user/system plugin roots, and the shared
	// caches. The recipe is always passed explicitly, so the upward walk that
	// would find the repo's own dogfood kapi.yaml never runs; KAPI_NO_PROJECT is
	// belt-and-braces for any path that falls back to discovery.
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
		Name:    "BlockedTargetTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr", "de"},
			Flow:            "translate",
			SourceGate:      string(model.SourceGateNone),
		},
		Content: []project.ContentCollection{
			{Name: "app", Path: "src/en.json", Target: targetTemplate},
		},
		Flows: map[string]*flow.StepsSpec{
			"translate": {Steps: []flow.FlowStep{
				{Tool: "translate", Config: map[string]any{"provider": "demo"}},
			}},
		},
		ShipGate: gate.Gate{"translated": {Pct: 100}},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	t.Chdir(dir)

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	return a, cmd, recipe, dir
}

// runBlockedConverge drives one converge run and returns its error (if any) plus
// the captured structured output.
func runBlockedConverge(t *testing.T, a *App, cmd *EnvCommand, recipe string) (ConvergeOutput, error) {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	var out ConvergeOutput
	rerr := a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate: true,
		MaxPasses: 3,
		noChecks:  true,
		capture:   &out,
	})
	return out, rerr
}

// TestConverge_BlockedTargetPath_FailsAndNeverReportsConverged is the core
// regression: with a directory occupying fr's target path the run must NOT
// report complete, and must say what is in the way.
func TestConverge_BlockedTargetPath_FailsAndNeverReportsConverged(t *testing.T) {
	a, cmd, recipe, dir := newBlockedTargetProject(t, "src/{lang}.json")
	blocked := filepath.Join(dir, "src", "fr.json")
	require.NoError(t, os.Mkdir(blocked, 0o755), "a directory takes fr's target path")

	out, err := runBlockedConverge(t, a, cmd, recipe)

	require.Error(t, err, "a target path that cannot be written must fail the run")
	var ope *flow.OutputPathError
	require.ErrorAs(t, err, &ope, "the failure must be typed so a UI can classify it")
	assert.Equal(t, flow.OutputPathIsDir, ope.Kind)
	assert.Contains(t, err.Error(), blocked, "the message must name the blocked path")
	assert.Contains(t, err.Error(), "a directory occupies that path")

	assert.False(t, out.Converged, "a failed run must not report convergence")
	assert.Zero(t, out.MaterializedFiles)

	// The obstruction is untouched and nothing was written into it.
	entries, rerr := os.ReadDir(blocked)
	require.NoError(t, rerr)
	assert.Empty(t, entries, "the run must not stage output inside the blocking directory")
}

// TestCoverage_BlockedTargetPath_ReadsUntranslated is the measurement half of
// the defect, asserted directly: the blocked locale must read 0% translated and
// must not clear its ship gate. Before the fix it read 100% / shippable.
func TestCoverage_BlockedTargetPath_ReadsUntranslated(t *testing.T) {
	a, _, recipe, dir := newBlockedTargetProject(t, "src/{lang}.json")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "src", "fr.json"), 0o755))
	// de is genuinely translated, so the test distinguishes the two halves
	// rather than asserting a blanket zero.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "de.json"),
		[]byte(`{"greeting":"Hallo Welt","farewell":"Auf Wiedersehen"}`), 0o644))

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, dir, "")
	require.NoError(t, err)
	cov, err := a.ComputeShipCoverage(context.Background(), proj, dir, units, nil)
	require.NoError(t, err, "one blocked path must not blind the whole report")

	byLocale := map[string]LocaleCoverage{}
	for _, c := range cov {
		byLocale[c.Locale] = c
	}
	require.Contains(t, byLocale, "fr")
	require.Contains(t, byLocale, "de")

	assert.Equal(t, 0, byLocale["fr"].Pct["translated"],
		"a directory at the target path is not a translation")
	assert.False(t, byLocale["fr"].Shippable, "the blocked locale cannot clear translated:100")

	assert.Equal(t, 100, byLocale["de"].Pct["translated"], "de really is translated")
	assert.True(t, byLocale["de"].Shippable)
}

// TestCoverage_UnreadableTargetStillCountsByPresence guards the behaviour the
// fix must NOT break: a real, written target whose format cannot be read back
// (a compiled catalog) still counts as present. Only an obstruction is excluded.
func TestCoverage_UnreadableTargetStillCountsByPresence(t *testing.T) {
	a, _, recipe, dir := newBlockedTargetProject(t, "src/{lang}.bin")
	// No reader claims `.bin`, so the target cannot be measured per unit — but
	// it is a real file the pipeline wrote.
	for _, loc := range []string{"fr", "de"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", loc+".bin"),
			[]byte("\x00compiled catalog"), 0o644))
	}

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, dir, "")
	require.NoError(t, err)
	cov, err := a.ComputeShipCoverage(context.Background(), proj, dir, units, nil)
	require.NoError(t, err)

	require.NotEmpty(t, cov)
	for _, c := range cov {
		assert.Equal(t, 100, c.Pct["translated"],
			"an unreadable but genuinely written target still counts by file-presence")
	}
}

// TestConverge_UnwritableTargetDirectory_FailsTheRun: the other blocked-path
// shape — the destination is free but its directory denies writing.
func TestConverge_UnwritableTargetDirectory_FailsTheRun(t *testing.T) {
	requireHostNonRoot(t)
	a, cmd, recipe, dir := newBlockedTargetProject(t, "out/{lang}/app.json")
	outDir := filepath.Join(dir, "out")
	require.NoError(t, os.Mkdir(outDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(outDir, 0o755) })

	out, err := runBlockedConverge(t, a, cmd, recipe)

	require.Error(t, err, "an unwritable target directory must fail the run")
	var ope *flow.OutputPathError
	require.ErrorAs(t, err, &ope)
	assert.Equal(t, flow.OutputPathPermission, ope.Kind)
	assert.Contains(t, err.Error(), "permission denied")
	assert.False(t, out.Converged)
}

// TestConverge_ClearedObstructionConverges: with nothing in the way the same
// project converges — the guard refuses blocked paths, not ordinary runs.
func TestConverge_ClearedObstructionConverges(t *testing.T) {
	a, cmd, recipe, dir := newBlockedTargetProject(t, "src/{lang}.json")
	out, err := runBlockedConverge(t, a, cmd, recipe)
	require.NoError(t, err)
	assert.True(t, out.Converged, "an unobstructed project converges")
	assert.FileExists(t, filepath.Join(dir, "src", "fr.json"))
	assert.FileExists(t, filepath.Join(dir, "src", "de.json"))
}

// TestMaterialize_BlockedTargetPath_WritesNothing: the materialize step refuses
// the whole pass when any destination is blocked, rather than writing some
// locales and then failing — and it names the obstruction.
func TestMaterialize_BlockedTargetPath_WritesNothing(t *testing.T) {
	a, cmd, recipe, dir := newBlockedTargetProject(t, "src/{lang}.json")
	// First converge cleanly so the block store holds real target overlays.
	_, err := runBlockedConverge(t, a, cmd, recipe)
	require.NoError(t, err)

	// Now block fr and re-materialize from the store.
	require.NoError(t, os.Remove(filepath.Join(dir, "src", "fr.json")))
	require.NoError(t, os.Remove(filepath.Join(dir, "src", "de.json")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "src", "fr.json"), 0o755))

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	written, merr := a.materializeFromProjectStore(context.Background(), os.Stderr, proj, recipe,
		[]model.LocaleID{"fr", "de"}, true)

	require.Error(t, merr, "a blocked destination must fail the materialize pass")
	assert.Zero(t, written, "nothing may be written when a destination is blocked")
	assert.Contains(t, merr.Error(), "a directory occupies that path")
	assert.NoFileExists(t, filepath.Join(dir, "src", "de.json"),
		"the pass is refused up front, so no locale is half-materialized")
}

// TestCheckTargetPathsWritable_NamesEveryBlockedPath: the admission check
// reports all obstructions at once, so they can be cleared in one pass.
func TestCheckTargetPathsWritable_NamesEveryBlockedPath(t *testing.T) {
	dir := t.TempDir()
	fr := filepath.Join(dir, "fr.json")
	de := filepath.Join(dir, "de.json")
	ok := filepath.Join(dir, "es.json")
	require.NoError(t, os.Mkdir(fr, 0o755))
	require.NoError(t, os.Mkdir(de, 0o755))

	err := CheckTargetPathsWritable([]VerifyUnit{
		{TargetPath: fr, Locale: "fr"},
		{TargetPath: de, Locale: "de"},
		{TargetPath: ok, Locale: "es"},
		{TargetPath: fr, Locale: "fr"}, // duplicate: reported once
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), fr)
	assert.Contains(t, err.Error(), de)
	assert.NotContains(t, err.Error(), ok)
	assert.Contains(t, err.Error(), "2 target paths cannot be written")
	var ope *flow.OutputPathError
	require.ErrorAs(t, err, &ope, "the aggregate keeps each member classifiable")

	assert.NoError(t, CheckTargetPathsWritable([]VerifyUnit{{TargetPath: ok}, {TargetPath: ""}}))
}

// requireHostNonRoot skips a permission test where mode bits do not deny writes,
// so it cannot pass vacuously in a root CI container.
func requireHostNonRoot(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits do not gate writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: mode bits do not deny writes, so this test cannot fail")
	}
	probe := filepath.Join(t.TempDir(), "probe")
	require.NoError(t, os.Mkdir(probe, 0o555))
	t.Cleanup(func() { _ = os.Chmod(probe, 0o755) })
	if f, err := os.Create(filepath.Join(probe, "x")); err == nil {
		_ = f.Close()
		t.Skip("this filesystem ignores directory mode bits")
	} else if !errors.Is(err, os.ErrPermission) {
		t.Skipf("unexpected probe error, cannot rely on mode bits: %v", err)
	}
}
