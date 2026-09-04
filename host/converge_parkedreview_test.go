package host

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// #2356. Under `materialize: on-converge` a pass drafts every locale and
// delivers only the locales that cleared their ship gate. A parked locale's
// draft tree goes with the run, and its work stays in the project block store as
// `targets/<locale>` overlays.
//
// These tests hold the whole cycle together over that store: the run's own
// report, `kapi status` coverage and the review queue all read it, a decision
// taken on a stored draft counts toward the gate, and the pass after serves the
// draft instead of paying a provider for it again.

// parkedReviewSource is four strings, so approving two of them is exactly the
// 50% `reviewed` bar the fixture's ship gate asks for.
const parkedReviewSource = `{
  "title": "Tide window",
  "subtitle": "When the forecast allows this movement",
  "cta": "Plan a crossing",
  "footer": "Readings update every six minutes"
}
`

// parkedReviewProject writes a two-locale project that drafts through the
// deterministic demo provider and delivers under a gate no unattended run can
// clear: `reviewed: 50` needs a person. Both locales therefore park on the first
// run, with their drafts in the store and nothing on disk.
func parkedReviewProject(t *testing.T) (*App, *EnvCommand, string, string) {
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"), []byte(parkedReviewSource), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "ParkedReviewTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb", "nl"},
			Flow:            "translate",
			SourceGate:      string(model.SourceGateNone),
			Materialize:     project.MaterializeOnConverge,
		},
		Collections: []project.Collection{
			{Name: "site", Path: "src/en.json", Target: "site/locales/{lang}.json"},
		},
		Flows: map[string]*flow.StepsSpec{
			"translate": {Steps: []flow.FlowStep{
				{Tool: "translate", Config: map[string]any{"provider": "demo"}},
			}},
		},
		ShipGate: gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 50}},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))
	t.Chdir(dir)

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	return a, cmd, recipe, dir
}

// parkedReviewPass runs one convergence pass and returns its output and events.
// One pass, because the gate is out of reach and every further pass would only
// re-read the same store.
func parkedReviewPass(t *testing.T, a *App, cmd *EnvCommand, recipe string) (ConvergeOutput, []convergence.Event) {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	var out ConvergeOutput
	var events []convergence.Event
	require.NoError(t, a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		MaxPasses: 1,
		noExtract: true,
		noChecks:  true,
		capture:   &out,
		onEvent:   func(ev convergence.Event) { events = append(events, ev) },
	}))
	return out, events
}

// parkedLocaleDone returns the locale_done event for one locale of the last
// pass. A locale the loop found already at its gate is produced by no pass and
// reports none, which is what found=false says.
func parkedLocaleDone(events []convergence.Event, locale string) (convergence.Event, bool) {
	var found convergence.Event
	ok := false
	for _, ev := range events {
		if ev.Type == convergence.EventLocaleDone && ev.Locale == locale {
			found, ok = ev, true
		}
	}
	return found, ok
}

// parkedProduced returns one locale's production for the last pass of a run,
// requiring that a pass produced it.
func parkedProduced(t *testing.T, events []convergence.Event, locale string) convergence.Event {
	t.Helper()
	ev, ok := parkedLocaleDone(events, locale)
	require.True(t, ok, "a pass must have produced %s", locale)
	return ev
}

// parkedProviderCalls totals the units one run sent to the provider for a
// locale, across every pass.
func parkedProviderCalls(events []convergence.Event, locale string) int {
	n := 0
	for _, ev := range events {
		if ev.Type == convergence.EventLocaleDone && ev.Locale == locale {
			n += ev.ViaAI
		}
	}
	return n
}

// parkedLocaleResult returns one locale's row of the run's closing report.
func parkedLocaleResult(t *testing.T, out ConvergeOutput, locale string) ConvergeLocaleResult {
	t.Helper()
	for _, lc := range out.Locales {
		if lc.Locale == locale {
			return lc
		}
	}
	t.Fatalf("the run reported no result for %s", locale)
	return ConvergeLocaleResult{}
}

// parkedCoverage returns one locale's `kapi status` coverage row, derived the
// way the status command derives it: over the units the recipe resolves, with
// no draft tree in play.
func parkedCoverage(t *testing.T, a *App, cmd *EnvCommand, recipe, dir, locale string) convergence.LocaleCoverage {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, dir, "")
	require.NoError(t, err)
	cov, err := a.ComputeShipCoverage(cmd.Context(), proj, dir, units, nil)
	require.NoError(t, err)
	for _, c := range cov {
		if c.Locale == locale {
			return c
		}
	}
	t.Fatalf("status derived no coverage for %s", locale)
	return convergence.LocaleCoverage{}
}

// parkedQueueKeys lists the review queue's unit keys for one locale, sorted.
func parkedQueueKeys(t *testing.T, a *App, recipe, locale string) []string {
	t.Helper()
	q, err := a.ReviewQueue(context.Background(), recipe, "en", ReviewQueueOptions{Languages: []string{locale}})
	require.NoError(t, err)
	var keys []string
	for _, it := range q.Pending {
		if it.Locale == locale && !it.IsSource {
			keys = append(keys, it.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

// TestConverge_ParkedLocaleIsReviewableThenShips is the issue's cycle end to
// end: the first pass parks both locales with nothing on disk, status and the
// review queue read the drafts out of the store, two approvals put nl at its
// `reviewed: 50` bar, and the pass after serves nl's four units from the store
// and delivers the locale.
func TestConverge_ParkedLocaleIsReviewableThenShips(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)

	out, events := parkedReviewPass(t, a, cmd, recipe)
	require.False(t, out.Converged, "an unattended run cannot reach `reviewed: 50`")
	for _, loc := range []string{"nb", "nl"} {
		assert.False(t, parkedLocaleResult(t, out, loc).Shippable, "%s must park", loc)
		_, err := os.Stat(filepath.Join(dir, "site", "locales", loc+".json"))
		assert.True(t, os.IsNotExist(err), "%s is parked, so it reaches no disk", loc)
	}
	nl := parkedProduced(t, events, "nl")
	assert.Equal(t, 4, nl.Done)
	assert.Equal(t, 4, nl.ViaAI, "the first pass pays a provider for every unit")
	assert.Zero(t, nl.ViaDraft)

	// The read surfaces find the parked work where it lives.
	cov := parkedCoverage(t, a, cmd, recipe, dir, "nl")
	assert.Equal(t, 100, cov.Pct["translated"], "the store holds a translation for every nl unit")
	assert.Zero(t, cov.Pct["reviewed"])
	assert.False(t, cov.Shippable, "translated is not the whole gate")
	assert.Equal(t, cov.Pct["translated"], parkedLocaleResult(t, out, "nl").Pct["translated"],
		"up and status publish one translated figure for one locale")

	keys := parkedQueueKeys(t, a, recipe, "nl")
	require.Len(t, keys, 4, "every parked nl unit is reviewable")

	// A person approves half of them, against the stored draft.
	for _, key := range keys[:2] {
		changed, err := a.ApplyReviewDecisionAs(cmd.Context(), recipe, "en",
			ReviewUnitRef{File: filepath.Join("site", "locales", "nl.json"), Key: key, Locale: "nl"},
			ReviewDecisionApproved, "", "")
		require.NoError(t, err)
		assert.True(t, changed, "approving a stored draft records a decision")
	}
	assert.Equal(t, 50, parkedCoverage(t, a, cmd, recipe, dir, "nl").Pct["reviewed"],
		"the decisions count where the gate reads them")

	// The next run delivers the locale that is now at its bar, and pays a
	// provider for none of it: the loop finds nl already at its gate from the
	// store, and any pass it did run would serve the same stored drafts.
	out2, events2 := parkedReviewPass(t, a, cmd, recipe)
	assert.Zero(t, parkedProviderCalls(events2, "nl"), "the run sends no nl unit to the provider")

	nlResult := parkedLocaleResult(t, out2, "nl")
	assert.True(t, nlResult.Shippable, "nl is at its gate")
	body, err := os.ReadFile(filepath.Join(dir, "site", "locales", "nl.json"))
	require.NoError(t, err, "a locale at its gate is delivered")
	assert.Contains(t, string(body), "⟦nl⟧", "the delivered file carries the drafted translations")

	assert.False(t, parkedLocaleResult(t, out2, "nb").Shippable, "nb has no decisions and stays parked")
	_, err = os.Stat(filepath.Join(dir, "site", "locales", "nb.json"))
	assert.True(t, os.IsNotExist(err), "delivery still respects the gate")
}

// TestConverge_ParkedDecisionSurvivesDelivery: the decision a reviewer took on
// the stored draft still describes the delivered file. The runs are the same
// runs, so the target hash the decision bound matches what lands on disk, and
// the locale reads as reviewed against its own file.
func TestConverge_ParkedDecisionSurvivesDelivery(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)

	keys := parkedQueueKeys(t, a, recipe, "nl")
	require.Len(t, keys, 4)
	for _, key := range keys[:2] {
		_, err := a.ApplyReviewDecisionAs(cmd.Context(), recipe, "en",
			ReviewUnitRef{File: filepath.Join("site", "locales", "nl.json"), Key: key, Locale: "nl"},
			ReviewDecisionApproved, "", "")
		require.NoError(t, err)
	}
	parkedReviewPass(t, a, cmd, recipe)
	require.FileExists(t, filepath.Join(dir, "site", "locales", "nl.json"))

	cov := parkedCoverage(t, a, cmd, recipe, dir, "nl")
	assert.Equal(t, 100, cov.Pct["translated"])
	assert.Equal(t, 50, cov.Pct["reviewed"],
		"the approvals still count once the same runs are on disk")
	assert.True(t, cov.Shippable)
}

// TestConverge_ParkedDraftsAreRecycledNotRedrafted: a locale that stays parked
// is drafted once. The run after finds the same source under the same governing
// context and serves every unit from the store, which is the difference between
// a locale that costs one set of provider calls and one that costs a set per
// run.
func TestConverge_ParkedDraftsAreRecycledNotRedrafted(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)

	_, events := parkedReviewPass(t, a, cmd, recipe)
	for _, loc := range []string{"nb", "nl"} {
		assert.Equal(t, 4, parkedProduced(t, events, loc).ViaAI, "%s is drafted from scratch", loc)
	}

	out2, events2 := parkedReviewPass(t, a, cmd, recipe)
	require.False(t, out2.Converged, "nothing was reviewed, so both locales stay parked")
	for _, loc := range []string{"nb", "nl"} {
		ev := parkedProduced(t, events2, loc)
		assert.Equal(t, 4, ev.Done, "%s still carries a translation for every unit", loc)
		assert.Equal(t, 4, ev.ViaDraft, "%s is served from the store", loc)
		assert.Zero(t, ev.ViaAI, "%s costs no provider call", loc)
		assert.Zero(t, ev.ViaMemory, "a stored draft is not reviewed wording, so it is not a memory match")
		_, err := os.Stat(filepath.Join(dir, "site", "locales", loc+".json"))
		assert.True(t, os.IsNotExist(err), "%s is still parked, so it still reaches no disk", loc)
	}
}

// TestUpPlan_ParkedDraftsAreReuse: `kapi up --plan` on a project whose locales
// parked with their drafts in the store counts those drafts as reuse rather
// than as provider work (#2369). A parked draft has no file on disk, so there
// is no committed translation for the record absorber to read, and the plan
// judges it at once rather than reporting it as unread.
func TestUpPlan_ParkedDraftsAreReuse(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	plan, err := a.computeProjectPlan(cmd.Context(), proj, recipe)
	require.NoError(t, err)
	assert.Zero(t, plan.Totals.UnreadTargets, "a parked draft is not a committed translation the store has yet to read")
	assert.Equal(t, 8, plan.Totals.Unanswered, "the record stands behind none of the drafts")
	assert.Equal(t, 8, plan.Totals.Drafts, "and the store answers every one of them")
	assert.Zero(t, plan.Totals.AIRemaining, "so the pass calls no provider")
	assert.Zero(t, plan.Totals.TokenEstimate)
	var text strings.Builder
	require.NoError(t, plan.FormatText(&text))
	assert.Contains(t, text.String(), "8 unit(s) served from stored drafts")
	assert.NotContains(t, text.String(), "not priced")

	// One rewritten string is provider work in every locale; the other three
	// units of each stay reuse, and the pass that follows agrees.
	edited := strings.Replace(parkedReviewSource,
		"When the forecast allows this movement",
		"When the forecast allows this crossing", 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"), []byte(edited), 0o644))
	plan, err = a.computeProjectPlan(cmd.Context(), proj, recipe)
	require.NoError(t, err)
	assert.Equal(t, 6, plan.Totals.Drafts, "the untouched strings are still answered by the store")
	assert.Equal(t, 2, plan.Totals.AIRemaining, "the rewritten one is drafted again in each locale")
	assert.Positive(t, plan.Totals.TokenEstimate)

	_, events := parkedReviewPass(t, a, cmd, recipe)
	for _, loc := range []string{"nb", "nl"} {
		ev := parkedProduced(t, events, loc)
		assert.Equal(t, 1, ev.ViaAI, "%s pays for the unit the plan priced", loc)
		assert.Equal(t, 3, ev.ViaDraft, "%s serves the units the plan counted as stored drafts", loc)
	}
}

// TestConverge_EditedSourceRedraftsOnlyThatUnit is the negative: recycling a
// stored draft is right only while the source it answers stands. Rewriting one
// string sends that unit to the provider again and leaves the other three
// served from the store.
func TestConverge_EditedSourceRedraftsOnlyThatUnit(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	_, events := parkedReviewPass(t, a, cmd, recipe)
	require.Equal(t, 4, parkedProduced(t, events, "nl").ViaAI)

	// One string rewritten, every other key left where it was: a block is
	// addressed by its position in the document, so a reordering file would
	// move every unit off its stored answer and prove nothing about the edit.
	edited := strings.Replace(parkedReviewSource,
		"When the forecast allows this movement",
		"When the forecast allows this crossing", 1)
	require.NotEqual(t, parkedReviewSource, edited)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"), []byte(edited), 0o644))

	_, events2 := parkedReviewPass(t, a, cmd, recipe)
	nl := parkedProduced(t, events2, "nl")
	assert.Equal(t, 4, nl.Done)
	assert.Equal(t, 1, nl.ViaAI, "only the rewritten string is drafted again")
	assert.Equal(t, 3, nl.ViaDraft, "the untouched strings are served from the store")
}
