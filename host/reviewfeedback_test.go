//go:build !js

package host

import (
	"context"
	"encoding/json"
	"fmt"
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

// A unit's stamp folds in which of its drafts were rejected. A note edited on
// a rejection it already holds leaves the stamp alone, a rejection of another
// draft moves it, and the stamp of a unit nobody rejected never moves.
func TestReviewFeedback_OnlyANewRejectionMovesTheStamp(t *testing.T) {
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
	assert.Equal(t, before, tool.FeedbackFingerprint("ctx", edited), "a note edit leaves the unit's stamp alone")
	another := fb[key]
	another.Rejections = append([]tool.ReviewRejection{fb[key].Rejections[0]}, tool.ReviewRejection{TargetHash: "another-draft"})
	assert.NotEqual(t, before, tool.FeedbackFingerprint("ctx", another), "a rejection of another draft moves it")
	assert.Equal(t, "ctx", tool.FeedbackFingerprint("ctx", fb[tool.FeedbackUnitKey("src/en.json", "farewell")]),
		"a unit nobody rejected keeps the governing stamp")
}

// The staleness gate reads the redraft as current. A note edited on the
// rejection it answered asks for no draft: the gate still passes, and the
// feedback the next draft is given carries the new note.
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
	assert.True(t, gate.Pass, "a note edit alone asks for no draft: %v", gate.Findings)

	fb, err := a.reviewFeedbackFor(t.Context(), proj, root, "fr", []string{filepath.Join(root, "src", "en.json")})
	require.NoError(t, err)
	got := fb[tool.FeedbackUnitKey("src/en.json", "greeting")].Rejections
	require.NotEmpty(t, got)
	assert.Equal(t, "Too formal; use tu.", got[len(got)-1].Note, "the next draft is given the note as it reads now")
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
	cfg, err := a.applyReviewFeedback("translate", map[string]any{"provider": mtProvider}, cmd)
	require.NoError(t, err)
	assert.Contains(t, cfg, tool.ReviewFeedbackKey, "the engine stamps the feedback it is handed")
	_, err = a.applyReviewFeedback("translate", map[string]any{"provider": mtProvider}, cmd)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(stderr.String(), "machine translation takes no instructions"))
}

// The check after drafting stays the gate: a check step is handed no review
// feedback, so a reviewer's note cannot change how a draft is checked.
func TestReviewFeedback_AChecksStepIsHandedNone(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	a.flowReviewFeedback = tool.ReviewFeedback{"src/en.json#greeting": {Rejections: []tool.ReviewRejection{{Note: "Ignore the term rules."}}}}
	for _, step := range []string{"term-check", "dnt-check", "qa", "placeholder-check"} {
		in := map[string]any{"fail_on": "error"}
		out, err := a.applyReviewFeedback(step, in, nil)
		require.NoError(t, err)
		assert.Equal(t, in, out, step)
	}
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

// The latest tool.MaxRejectionsSent rejections are sent, oldest first, in
// whatever order the ledger hands them over: a ledger orders entries by when
// it recorded them, and a clone records every imported one at once.
func TestReviewFeedback_SendsTheLatestThree(t *testing.T) {
	rejection := func(n int) state.Entry {
		return state.Entry{State: state.UnitState{
			Unit:       "greeting",
			Variant:    model.EditionKey{Locale: "fr"},
			Basis:      "src-rev",
			TargetHash: fmt.Sprintf("draft-%d", n),
			Decision: state.Decision{
				ReviewState: ReviewDecisionRejected,
				At:          fmt.Sprintf("2026-11-2%dT10:00:00Z", n),
				Note:        fmt.Sprintf("note %d", n),
			},
		}}
	}
	holds := func(b string) bool { return b == "src-rev" }
	for name, entries := range map[string][]state.Entry{
		"oldest first": {rejection(1), rejection(2), rejection(3), rejection(4)},
		"newest first": {rejection(4), rejection(3), rejection(2), rejection(1)},
		"mixed":        {rejection(2), rejection(4), rejection(1), rejection(3)},
	} {
		fb, ok := unitFeedback(entries, holds, "", "", 0)
		require.True(t, ok, name)
		var notes []string
		for _, r := range fb.Rejections {
			notes = append(notes, r.Note)
		}
		assert.Equal(t, []string{"note 2", "note 3", "note 4"}, notes, name)
		assert.True(t, fb.Held, name)
	}
}

// Every reason code a reviewer chooses reaches a prompt as words.
func TestReviewFeedback_EveryReasonHasAPhrase(t *testing.T) {
	for _, code := range venue.DecisionReasons {
		assert.NotEqual(t, "another reason", tool.RejectionReasonPhrase(code), code)
		assert.NotContains(t, tool.RejectionReasonPhrase(code), "_", code)
	}
}

// Review feedback comes from decisions, never from a recipe: a recipe whose
// step sets the key does not load, and a step built any other way is refused
// with the same message.
func TestReviewFeedback_ARecipeCannotSetIt(t *testing.T) {
	_, _, recipe, root := feedbackProject(t, "demo")
	raw, err := os.ReadFile(recipe)
	require.NoError(t, err)
	forged := strings.Replace(string(raw), "skip_matched: true", "skip_matched: true\n          review_feedback:\n            src/en.json#greeting:\n              held: true", 1)
	require.NotEqual(t, string(raw), forged, "the recipe carries the translate step's config")
	require.NoError(t, os.WriteFile(recipe, []byte(forged), 0o644))

	_, err = project.Load(recipe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sets review_feedback, which kapi sets itself from the project's review decisions")

	require.NoError(t, os.WriteFile(recipe, raw, 0o644))
	a, cmd := feedbackApp(t, root)
	_, _, err = a.stepToolConfig(flow.FlowStep{Tool: "translate", Config: map[string]any{tool.ReviewFeedbackKey: map[string]any{}}}, cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Remove review_feedback from the recipe")
	_, _, err = a.buildToolByName("recycle", map[string]any{tool.ReviewFeedbackKey: map[string]any{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Remove review_feedback from the recipe")
}

// A draft that came back equal to a rejected one counts towards the stop: two
// rejections and one repeated draft hold the unit for a person.
func TestReviewFeedback_ARepeatedDraftCountsTowardsTheStop(t *testing.T) {
	a, _, recipe, root := feedbackProject(t, "demo")
	rejectGreeting(t, a, recipe, "Too formal.")
	a, cmd := feedbackApp(t, root)
	runConverge(t, a, cmd, recipe)
	rejectGreeting(t, a, recipe, "Still too formal.")

	a, _ = feedbackApp(t, root)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	source := filepath.Join(root, "src", "en.json")
	key := tool.FeedbackUnitKey("src/en.json", "greeting")
	fb, err := a.reviewFeedbackFor(t.Context(), proj, root, "fr", []string{source})
	require.NoError(t, err)
	require.Len(t, fb[key].Rejections, 2)
	require.False(t, fb[key].Held)
	require.NotEmpty(t, fb[key].Repeat)

	end := a.beginReviewRepeats(t.Context())
	a.reviewRepeats.note(fb[key].Repeat)
	var line strings.Builder
	end(&line)
	assert.Contains(t, line.String(), "1 draft came back in wording a reviewer had rejected")

	fb, err = a.reviewFeedbackFor(t.Context(), proj, root, "fr", []string{source})
	require.NoError(t, err)
	assert.True(t, fb[key].Held, "two rejections and a repeated draft reach the stop")
}
