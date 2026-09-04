package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/config"
)

// What a convergence pass spends a provider call on is a unit the project
// content memory does not answer — the pipeline reads the SOURCE files, so
// `recycle` fills what the corpus holds and `translate` drafts the rest. Pricing
// a produced unit by whether a target file exists therefore quoted zero for work
// the run went on to do, and the two numbers a reader compares — the plan header
// and the run's own per-locale counts — described different runs.

// runSourceLines totals the counts the run reports per locale, from the
// renderer's plain-stream lines: "  nb  2/2 units  (content memory 1 · AI 1)",
// with the "drafts N" segment the renderer adds for the units a pass served
// from the block store rather than a provider.
var runSourceLines = regexp.MustCompile(`content memory (\d+) · (?:drafts (\d+) · )?AI (\d+)`)

func runCounts(t *testing.T, out string) (viaMemory, viaDraft, viaAI int) {
	t.Helper()
	matches := runSourceLines.FindAllStringSubmatch(out, -1)
	require.NotEmpty(t, matches, "the run must report its per-locale counts: %s", out)
	atoi := func(s string) int {
		if s == "" {
			return 0
		}
		n, err := strconv.Atoi(s)
		require.NoError(t, err)
		return n
	}
	for _, m := range matches {
		viaMemory += atoi(m[1])
		viaDraft += atoi(m[2])
		viaAI += atoi(m[3])
	}
	return viaMemory, viaDraft, viaAI
}

// TestUpPlan_PricesWhatTheRunDrafts is the whole verb over the three-locale
// project: the plan header and the run agree on how many provider calls the run
// makes, in both the fresh-clone case and after a source rewrite that only one
// locale holds a decision for.
func TestUpPlan_PricesWhatTheRunDrafts(t *testing.T) {
	root := writeThreeLocaleProject(t)
	proj := filepath.Join(root, "kapi.yaml")

	// A fresh clone. Every locale's committed translation of "Apple" is absorbed
	// and recycled; "Compass" is identical in all four languages with no approval
	// behind it, so the record declines the pairing and the pass drafts it — one
	// provider call per locale, quoted before it is spent.
	first := demoProviderApp(t)
	out, err := runUp(t, first, proj)
	require.NoError(t, err, out)
	_, _, ai := runCounts(t, out)
	assert.Equal(t, 3, ai, "one draft per locale for the unit the record declines: %s", out)
	assert.Contains(t, out, "3 AI", "the header prices the run it is about to start: %s", out)
	assert.Contains(t, out, "drafting 3 unit(s) the content memory does not answer")

	// One locale's translation is approved, and then the source sentence it was
	// written against is rewritten. The key survives, so every locale's old
	// translation sits beside a sentence it never translated. Each locale holds
	// the basis the loop recorded when it wrote that translation, so the rewrite
	// reads as drift in all three rather than as an unanswered pairing in two.
	approveUnit(t, root, "de", "Apple")
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apricot","b":"Compass"}`), 0o644))

	second := demoProviderApp(t)
	out2, err := runUp(t, second, proj)
	require.NoError(t, err, out2)
	_, _, ai2 := runCounts(t, out2)
	assert.Equal(t, 3, ai2, "the rewritten unit is drafted in every locale: %s", out2)
	assert.Contains(t, out2, "3 AI", "and the header says so before the tokens burn: %s", out2)
	assert.Contains(t, out2, "re-drafting 3 stale unit(s)",
		"every locale recorded what it translated, so the rewrite is drift in all three")
	assert.NotContains(t, out2, "does not answer",
		"drift is named as drift, not as a pairing the corpus happens not to hold")
}

// TestUpPlan_DryRunPricesTheRunThatFollows: `kapi up --plan` counts the same
// units on its own, so the dry run and the run it describes are about the same
// work. On a project whose store has never read the committed translations it
// says that, rather than quoting a zero it cannot stand behind.
//
// Once the store holds the first run's drafts, the plan asks the drafting step's
// own reuse question of each unit and counts the ones it answers as stored
// drafts, so the run that follows serves exactly those from the store and calls
// the provider for exactly what the plan priced (#2369).
func TestUpPlan_DryRunPricesTheRunThatFollows(t *testing.T) {
	root := writeThreeLocaleProject(t)
	proj := filepath.Join(root, "kapi.yaml")

	planJSON := func(t *testing.T) UpPlanOutput {
		t.Helper()
		a := demoProviderApp(t)
		out, err := runUp(t, a, proj, "--plan", "--json")
		require.NoError(t, err, out)
		var plan UpPlanOutput
		require.NoError(t, json.Unmarshal([]byte(out), &plan), "plan --json must parse: %s", out)
		return plan
	}

	// A dry run on a fresh clone must not create the project store, so it has no
	// corpus to judge a produced unit against. It declines to price them and says
	// why: quoting zero here promised a free run that then spent three provider
	// calls, and quoting the units would have billed for translations the run
	// recycles.
	fresh := planJSON(t)
	assert.Empty(t, fresh.Scopes)
	assert.Equal(t, 6, fresh.Totals.UnreadTargets, "two units in three locales, none of them read yet")
	assert.Zero(t, fresh.Totals.AIRemaining)

	first := demoProviderApp(t)
	out, err := runUp(t, first, proj)
	require.NoError(t, err, out)

	// The record has been read now, so the plan judges every produced unit. The
	// units the record declined to pair are the ones the next run drafts, and
	// the store already holds the first run's answer for each, made from this
	// source under the configuration the run would send now: they are stored
	// drafts, and none of them is provider work.
	priced := planJSON(t)
	assert.Zero(t, priced.Totals.UnreadTargets, "every committed target has been read")
	assert.Equal(t, 3, priced.Totals.Unanswered,
		"an identical translation with no approval behind it is drafted, not recycled")
	assert.Equal(t, 3, priced.Totals.Drafts, "the store answers each of them")
	assert.Zero(t, priced.Totals.AIRemaining, "so the run calls no provider for them")
	assert.Zero(t, priced.Totals.TokenEstimate, "and a stored draft costs no tokens")
	// The axes partition the work: everything counted is recycled, served from a
	// stored draft, or drafted.
	assert.Equal(t,
		priced.Totals.MissingTarget+priced.Totals.Stale+priced.Totals.Unanswered,
		priced.Totals.MemoryExact+priced.Totals.Drafts+priced.Totals.AIRemaining)

	second := demoProviderApp(t)
	out2, err := runUp(t, second, proj)
	require.NoError(t, err, out2)
	_, drafts, ai := runCounts(t, out2)
	assert.Equal(t, priced.Totals.Drafts, drafts,
		"the run served from the store the units the dry run counted as stored drafts: %s", out2)
	assert.Equal(t, priced.Totals.AIRemaining, ai,
		"and called the provider for what the dry run priced, which is nothing: %s", out2)
	assert.Contains(t, out2, "3 drafts · 0 AI", "the run's own header says so before it starts: %s", out2)
}

// houseVoice is a voice profile at the conventional path, enough to give the
// project a governing context where it had none.
const houseVoice = `id: house
name: House
tone:
  formality: formal
  guidelines: Lead with the fact, then the consequence.
`

// TestUpPlan_StoredDraftsAreReuseOnlyWhileTheirAnswerStands: the plan asks the
// drafting step the question it asks at run time, so a stored draft counts as
// reuse under the configuration and governing context it was made under, and
// as provider work under any other, and a unit the store has never answered is
// quoted in full beside the ones it has.
func TestUpPlan_StoredDraftsAreReuseOnlyWhileTheirAnswerStands(t *testing.T) {
	root := writeThreeLocaleProject(t)
	proj := filepath.Join(root, "kapi.yaml")

	// plan reads the structured plan off stdout alone: the seed phase that
	// precedes a plan reports what it compiled on stderr, and a project that
	// just gained a voice profile has something to compile.
	plan := func(t *testing.T, a *App) host.UpPlanScope {
		t.Helper()
		cmd := NewUpCmd(a)
		cmd.SetArgs([]string{"--project", proj, "--plan", "--json"})
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		require.NoError(t, cmd.Execute(), errOut.String())
		var p UpPlanOutput
		require.NoError(t, json.Unmarshal(out.Bytes(), &p), "plan --json must parse: %s", out.String())
		return p.Totals
	}
	run := func(t *testing.T, a *App) (drafts, ai int) {
		t.Helper()
		out, err := runUp(t, a, proj, "--no-checks")
		require.NoError(t, err, out)
		_, drafts, ai = runCounts(t, out)
		return drafts, ai
	}
	// largeModel pins a model the first run did not use: the same provider, a
	// different configuration fingerprint.
	largeModel := func(t *testing.T) *App {
		t.Helper()
		a := demoProviderApp(t)
		a.Config.Set(config.KeyAIModel, "demo-large")
		return a
	}

	// The first run drafts the three units the record declines to pair and
	// leaves its answers in the store.
	_, ai := run(t, demoProviderApp(t))
	require.Equal(t, 3, ai)

	// A changed model is a changed configuration: the stored answers were made
	// under another one, so the plan quotes them again and the run pays.
	totals := plan(t, largeModel(t))
	assert.Zero(t, totals.Drafts, "a draft made under another model is not reused")
	assert.Equal(t, 3, totals.AIRemaining)
	assert.Positive(t, totals.TokenEstimate)
	drafts, ai := run(t, largeModel(t))
	assert.Zero(t, drafts)
	assert.Equal(t, 3, ai, "the run re-drafts under the new model")

	// That run stored its answers under the new model, so the same
	// configuration reuses them.
	totals = plan(t, largeModel(t))
	assert.Equal(t, 3, totals.Drafts)
	assert.Zero(t, totals.AIRemaining)

	// A voice profile at the conventional path moves the governing context the
	// stored answers were made under, whichever model made them.
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapi", "voice.yaml"), []byte(houseVoice), 0o644))
	totals = plan(t, largeModel(t))
	assert.Zero(t, totals.Drafts, "a draft made under another context is not reused")
	assert.Equal(t, 3, totals.AIRemaining)
	drafts, ai = run(t, largeModel(t))
	assert.Zero(t, drafts)
	assert.Equal(t, 3, ai, "the run re-drafts under the new context")

	// A fresh unit has no stored answer and is quoted in full.
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Compass","c":"Harbour"}`), 0o644))
	totals = plan(t, largeModel(t))
	assert.Equal(t, 3, totals.MissingTarget, "the new unit in every locale")
	assert.Equal(t, 3, totals.AIRemaining, "is provider work")
	assert.Positive(t, totals.TokenEstimate)
	assert.Zero(t, totals.Drafts, "nothing stored answers it")
	_, ai = run(t, largeModel(t))
	assert.Equal(t, 3, ai, "the run pays for the new unit, as quoted")

	// The run stored the new unit's draft and delivered it, so the next plan
	// finds nothing left to spend.
	totals = plan(t, largeModel(t))
	assert.Zero(t, totals.AIRemaining)
	assert.Zero(t, totals.TokenEstimate)
}
