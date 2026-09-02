package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// The staleness gate's input, written by a real convergence.
//
// The gate compares what a producer stamped against what the context in force
// would stamp now, and it reads that stamp out of the project state store. A
// JSON catalog has nowhere to keep an origin, so the delivered file says
// nothing about what governed it and the record the loop writes is where the
// statement lives. These tests drive a convergence over the demo provider and
// hold the whole path together: the run records what governed each target it
// produced, moving the terminology under it makes the gate fail, and `kapi
// check` prints the failure.

// stalenessConvergeVoice is the profile the converged fixture binds. It stays
// put across the run, so terminology is the only input that moves.
const stalenessConvergeVoice = `name: Converge Voice
version: 1
tone:
  formality: neutral
`

// writeStalenessTerms writes the project's committed terms source with one
// concept, rendered in fr as the caller says. Rewriting it moves the
// terminology out from under a produced target.
func writeStalenessTerms(t *testing.T, root, frTerm string) {
	t.Helper()
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	concepts := []terms.Concept{{
		ID:     "term:content-memory",
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "content memory", Locale: "en", Status: model.TermPreferred},
			{Text: frTerm, Locale: "fr", Status: model.TermPreferred},
		},
		CreatedAt: stamp,
		UpdatedAt: stamp,
	}}
	data, err := ktb.Marshal(ktb.FromConcepts(concepts))
	require.NoError(t, err)
	path := filepath.Join(root, project.RelStatePath(ktb.ConventionalName))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// newConvergedStalenessProject writes a governed one-locale JSON project and
// converges it with the deterministic demo provider, returning the app that ran
// it, the recipe path and the project root.
//
// It runs under the dogfood isolation contract (CLAUDE.md): every root this run
// could otherwise inherit is pinned to a throwaway dir and project discovery is
// off, so the repo's own recipe can never be found.
func newConvergedStalenessProject(t *testing.T) (*App, string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"),
		[]byte(`{"greeting":"Utilize the content memory","farewell":"Goodbye now"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(stalenessConvergeVoice), 0o644))
	writeStalenessTerms(t, root, "mémoire de contenu")

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "StalenessConverge",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Flow:            "converge",
			Voice:           &project.VoiceBinding{ProfileFile: "voice.yaml"},
			TermsSource:     project.RelStatePath(ktb.ConventionalName),
		},
		Collections: []project.Collection{
			{Name: "app", Path: "src/en.json", Target: "src/{lang}.json"},
		},
		Flows: map[string]*flow.StepsSpec{
			"converge": {Steps: []flow.FlowStep{
				{Tool: "translate", Config: map[string]any{"provider": "demo"}},
			}},
		},
	}
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	t.Chdir(root)

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	// Discovery is off under the isolation contract, and an explicit -p still
	// wins. Without it the run resolves no recipe of its own, so it reads its
	// terminology out of the compiled working index instead of the committed
	// source, and rewriting that source moves nothing.
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	runConverge(t, a, cmd, recipe)
	return a, recipe, root
}

// freshStalenessCheck is the project as a later command reaches it: a new app,
// so every input is resolved from the tree again rather than from what the run
// happened to hold.
func freshStalenessCheck(t *testing.T, recipe, root string) (*App, *EnvCommand, *project.KapiProject, []VerifyUnit) {
	t.Helper()
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	cmd := NewEnvCommand(context.Background(), "check")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.NotEmpty(t, units)
	return a, cmd, proj, units
}

// TestStalenessGate_ConvergenceRecordsWhatGovernedIt is the defect #2344
// reports: the gate read a stamp nothing wrote. A convergence over a JSON
// catalog records the producer's origin for every target it produced, so the
// stamp the gate compares exists and holds the context the run resolved.
func TestStalenessGate_ConvergenceRecordsWhatGovernedIt(t *testing.T) {
	a, recipe, root := newConvergedStalenessProject(t)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.NotEmpty(t, units)

	fpsCmd := NewEnvCommand(context.Background(), "check")
	AddProjectFlag(fpsCmd)
	AddVerifyFlags(fpsCmd)
	require.NoError(t, fpsCmd.Flags().Set("project", recipe))
	fps, err := newContextFingerprints(a, fpsCmd, proj, root)
	require.NoError(t, err)
	defer fps.close()
	want, err := fps.at(a.unitGovernancePoint(root, units[0]), units[0].Locale)
	require.NoError(t, err)
	require.NotEmpty(t, want.fingerprint, "a governed project stamps a fingerprint")

	ctx := context.Background()
	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	recorded, err := st.All(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, recorded, "the run records a basis for what it produced")

	for _, u := range recorded {
		assert.Equal(t, want.fingerprint, u.Origin.ContextFingerprint,
			"unit %s carries the fingerprint the run's producers resolved", u.Unit)
		assert.Equal(t, model.OriginAI, u.Origin.Kind, "the demo provider is an AI producer")
	}
}

// TestStalenessGate_TermsMoveUnderAConvergedProject drives the whole verb: a
// real convergence, then a change to the terminology governing it, and the gate
// failing over the targets the run produced. With no origin in the state store
// this branch was reachable only by seeding a record by hand.
func TestStalenessGate_TermsMoveUnderAConvergedProject(t *testing.T) {
	_, recipe, root := newConvergedStalenessProject(t)

	a, cmd, proj, units := freshStalenessCheck(t, recipe, root)
	gate, judged, err := a.verifyStaleness(cmd, proj, root, units)
	require.NoError(t, err)
	require.True(t, judged, "produced targets are judged")
	assert.True(t, gate.Pass, "the run's own output matches the context it ran under")
	assert.Empty(t, gate.Findings)

	// The wording approved for fr changes. The voice profile is untouched, so
	// terminology is what moved.
	writeStalenessTerms(t, root, "mémoire du contenu")

	a, cmd, proj, units = freshStalenessCheck(t, recipe, root)
	gate, judged, err = a.verifyStaleness(cmd, proj, root, units)
	require.NoError(t, err)
	require.True(t, judged)
	assert.False(t, gate.Pass, "targets produced under the old terminology are behind it")
	require.NotEmpty(t, gate.Findings)
	assert.Equal(t, "error", gate.Findings[0].Severity)
	assert.Equal(t, "fr", gate.Findings[0].Locale)
	assert.Contains(t, gate.Findings[0].Message, "superseded context")
	assert.Contains(t, gate.Findings[0].Message, "terms moved")
	assert.Contains(t, gate.Findings[0].Message, "the terminology in force has changed")
}

// TestStalenessGate_CheckPrintsTheSupersededTargets: the finding has to reach
// the surface a person runs. `kapi check` carries the gate by default, so the
// rendered report names it and says what to do about it.
func TestStalenessGate_CheckPrintsTheSupersededTargets(t *testing.T) {
	_, recipe, root := newConvergedStalenessProject(t)
	writeStalenessTerms(t, root, "mémoire du contenu")

	a := &App{SourceLang: "en"}
	a.InitRegistries()
	cmd := NewEnvCommand(context.Background(), "check")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))

	out, err := a.computeVerify(cmd, nil)
	require.NoError(t, err)

	gate, ok := gateByName(out, gateStaleness)
	require.True(t, ok, "the default gate set carries the staleness gate")
	assert.False(t, gate.Pass)
	assert.False(t, out.Pass, "a superseded stamp fails the check")

	var buf bytes.Buffer
	require.NoError(t, out.FormatText(&buf))
	assert.Contains(t, buf.String(), gateStaleness)
	assert.Contains(t, buf.String(), "superseded context")
	assert.Contains(t, buf.String(), "kapi up")
}
