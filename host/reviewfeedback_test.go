package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mttools "github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/state"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/venue"
)

// The next draft of a unit a reviewer rejected: what the run tells the model,
// what it stamps, and what it never hands back.

// feedbackProject is a two-unit project converged once into French by the
// demo provider, with provider as the drafting step's provider.
func feedbackProject(t *testing.T, provider string) (*App, *EnvCommand, string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"),
		[]byte(`{"greeting":"Hello there","farewell":"Goodbye now"}`), 0o644))
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "Feedback",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Flow:            "converge",
		},
		Collections: []project.Collection{{Name: "docs", Path: "src/en.json", Target: "src/{lang}.json"}},
		Flows: map[string]*flow.StepsSpec{
			"converge": {Steps: []flow.FlowStep{
				{Tool: "recycle"},
				{Tool: "translate", Config: map[string]any{"provider": provider, "skip_matched": true}},
			}},
		},
	}
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	t.Chdir(root)
	a, cmd := feedbackApp(t, root)
	runConverge(t, a, cmd, recipe)
	return a, cmd, recipe, root
}

func feedbackApp(t *testing.T, root string) (*App, *EnvCommand) {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	readProjectContext(t, root)
	return a, cmd
}

func frenchTargets(t *testing.T, root string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "src", "fr.json"))
	require.NoError(t, err)
	var out map[string]string
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func rejectGreeting(t *testing.T, a *App, recipe, note string) {
	t.Helper()
	changed, err := rejectUnitWithReasons(t.Context(), a, recipe, ReviewUnitRef{File: filepath.Join("src", "fr.json"), Key: "greeting", Locale: "fr"}, note, "tone")
	require.NoError(t, err)
	require.True(t, changed)
}

// A rejected unit is drafted again: the call that drafts it carries the
// rejected draft and the reviewer's note in the prompt the exchange records,
// the rejected wording does not come back, the other unit is not drafted
// again, and the run owes nothing afterwards.
func TestReviewFeedback_TheNextDraftIsToldWhy(t *testing.T) {
	a, _, recipe, root := feedbackProject(t, "demo")
	first := frenchTargets(t, root)
	rejectGreeting(t, a, recipe, "Too formal for a greeting.")

	sent := exchangesSent(t)
	a, cmd := feedbackApp(t, root)
	out, _ := runConverge(t, a, cmd, recipe)

	calls := sent()
	require.NotEmpty(t, calls, "the rejected unit is drafted again")
	all := strings.Join(calls, "\n")
	assert.Contains(t, all, "Too formal for a greeting.", "the note reaches the prompt record")
	assert.Contains(t, all, tool.RejectionReasonPhrase("tone"), "the reason reaches the prompt record as words")
	assert.Contains(t, all, first["greeting"], "the rejected draft reaches the prompt record")
	assert.NotContains(t, all, "Goodbye now", "the unit nobody rejected is not drafted again")

	after := frenchTargets(t, root)
	assert.NotEqual(t, first["greeting"], after["greeting"], "the rejected wording is not returned")
	assert.Equal(t, first["farewell"], after["farewell"])
	assert.Zero(t, out.RejectedUnits(), "nothing is owed after a run with a provider")
}

// The rejected wording is not recycled: the content memory no longer answers
// the unit with it, and the run asks the provider rather than serving it.
func TestReviewFeedback_TheRejectedPairIsNotRecycled(t *testing.T) {
	a, _, recipe, root := feedbackProject(t, "demo")
	first := frenchTargets(t, root)
	// The content memory learned the draft from the committed file before
	// anyone rejected it, so it could answer the unit with it.
	_, err := a.AbsorbProjectRecord(t.Context(), recipe)
	require.NoError(t, err)
	rejectGreeting(t, a, recipe, "")

	sent := exchangesSent(t)
	a, cmd := feedbackApp(t, root)
	runConverge(t, a, cmd, recipe)
	require.NotEmpty(t, sent(), "the provider is asked for the rejected unit")
	assert.NotEqual(t, first["greeting"], frenchTargets(t, root)["greeting"])
}

// The absorber does not learn a translation a reviewer rejected as that
// unit's answer.
func TestReviewFeedback_TheAbsorberSkipsARejectedPair(t *testing.T) {
	a, _, recipe, root := feedbackProject(t, "demo")
	rejectGreeting(t, a, recipe, "")
	// The same translations laid out afresh, so the absorber reads the file
	// again rather than passing over a digest it has seen.
	targets := frenchTargets(t, root)
	raw, err := json.MarshalIndent(targets, "", "    ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "fr.json"), raw, 0o644))

	res, err := a.AbsorbProjectRecord(t.Context(), recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Rejected)
	assert.Equal(t, 1, res.Pairs, "the unit nobody rejected is learned")
}

// A changed note makes exactly that unit stale: its stamp folds in the
// feedback, and the other unit's does not move.
func TestReviewFeedback_AChangedNoteMakesOnlyThatUnitStale(t *testing.T) {
	a, _, recipe, root := feedbackProject(t, "demo")
	rejectGreeting(t, a, recipe, "Too formal.")
	a, cmd := feedbackApp(t, root)
	runConverge(t, a, cmd, recipe)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	fb, err := a.reviewFeedbackFor(t.Context(), proj, root, "fr", []string{filepath.Join(root, "src", "en.json")})
	require.NoError(t, err)
	key := tool.FeedbackUnitKey("src/en.json", "greeting")
	require.Contains(t, fb, key)
	require.Len(t, fb[key].Rejections, 1)
	assert.Equal(t, "Too formal.", fb[key].Rejections[0].Note)

	before := tool.FeedbackFingerprint("ctx", fb[key])
	edited := fb[key]
	edited.Rejections = []tool.ReviewRejection{edited.Rejections[0]}
	edited.Rejections[0].Note = "Too formal; use tu."
	assert.NotEqual(t, before, tool.FeedbackFingerprint("ctx", edited), "a changed note moves the unit's stamp")
	assert.Equal(t, "ctx", tool.FeedbackFingerprint("ctx", fb[tool.FeedbackUnitKey("src/en.json", "farewell")]),
		"a unit nobody rejected keeps the governing stamp")
}

// The staleness gate reads the redraft as current, and a changed note on the
// rejection it answered makes that unit stale and no other.
func TestReviewFeedback_TheGateFoldsInTheFeedback(t *testing.T) {
	a, _, recipe, root := feedbackProject(t, "demo")
	rejectGreeting(t, a, recipe, "Too formal.")
	a, cmd := feedbackApp(t, root)
	runConverge(t, a, cmd, recipe)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, root, "fr")
	require.NoError(t, err)
	gate, _, err := a.verifyStaleness(cmd, proj, root, units, nil)
	require.NoError(t, err)
	assert.True(t, gate.Pass, "the redraft was made under the feedback it carries: %v", gate.Findings)

	st, err := a.OpenProjectState(t.Context(), root)
	require.NoError(t, err)
	entries, err := st.EntriesInScope(t.Context(), a.documentIndexOrEmpty(t.Context(), root).Scope(root, filepath.Join(root, "src", "en.json")))
	require.NoError(t, err)
	var rejection state.UnitState
	for _, e := range entries {
		if e.State.Unit == "greeting" && e.State.Decision.ReviewState == ReviewDecisionRejected {
			rejection = e.State
			break
		}
	}
	require.NotEmpty(t, rejection.Unit)
	rejection.Decision.Note = "Too formal; use tu."
	rejection.Decision.At = "2099-01-01T00:00:00Z"
	require.NoError(t, st.Put(t.Context(), rejection))

	gate, _, err = a.verifyStaleness(cmd, proj, root, units, nil)
	require.NoError(t, err)
	require.False(t, gate.Pass, "a changed note makes the unit stale")
	require.Len(t, gate.Findings, 1)
	assert.Contains(t, gate.Findings[0].Message, "1 of 2 targets", "only the unit whose note changed")
}

// Machine translation is sent no feedback: the run says so once, the unit is
// still drafted again, and nothing of the note reaches an AI provider.
func TestReviewFeedback_MachineTranslationIsSentNone(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	a.flowReviewFeedback = tool.ReviewFeedback{"src/en.json#greeting": {Rejections: []tool.ReviewRejection{{Note: "Too formal."}}}}
	cmd := NewEnvCommand(t.Context(), "up")
	var stderr strings.Builder
	cmd.SetErr(&stderr)
	// The framework ships no engine; a plugin appends one, as this does.
	const mtProvider = "test-mt"
	saved := mttools.Providers
	mttools.Providers = append(slices.Clone(saved), mttools.Provider{ID: mtProvider, Label: "Test MT"})
	t.Cleanup(func() { mttools.Providers = saved })
	cfg := a.applyReviewFeedback("translate", map[string]any{"provider": mtProvider}, cmd)
	assert.Contains(t, cfg, tool.ReviewFeedbackKey, "the engine stamps the feedback it is handed")
	a.applyReviewFeedback("translate", map[string]any{"provider": mtProvider}, cmd)
	assert.Equal(t, 1, strings.Count(stderr.String(), "machine translation takes no instructions"))
}

// A unit rejected tool.MaxRejections times is drafted no more.
func TestReviewFeedback_StopsAfterThreeRejections(t *testing.T) {
	a, _, recipe, root := feedbackProject(t, "demo")
	for i := range tool.MaxRejections - 1 {
		rejectGreeting(t, a, recipe, "Not right yet.")
		var cmd *EnvCommand
		a, cmd = feedbackApp(t, root)
		out, _ := runConverge(t, a, cmd, recipe)
		assert.Zero(t, out.LeftForPersonUnits(), "rejection %d is drafted again", i+1)
		assert.Zero(t, out.RejectedUnits(), "rejection %d", i+1)
	}
	rejectGreeting(t, a, recipe, "Not right yet.")
	held := frenchTargets(t, root)["greeting"]

	sent := exchangesSent(t)
	a, cmd := feedbackApp(t, root)
	out, _ := runConverge(t, a, cmd, recipe)
	assert.Empty(t, sent(), "nothing is drafted for a unit left for a person")
	assert.Equal(t, held, frenchTargets(t, root)["greeting"])
	assert.Equal(t, 1, out.LeftForPersonUnits())
}

// Every reason code a reviewer chooses reaches a prompt as words.
func TestReviewFeedback_EveryReasonHasAPhrase(t *testing.T) {
	for _, code := range venue.DecisionReasons {
		assert.NotEqual(t, "another reason", tool.RejectionReasonPhrase(code), code)
		assert.NotContains(t, tool.RejectionReasonPhrase(code), "_", code)
	}
}
