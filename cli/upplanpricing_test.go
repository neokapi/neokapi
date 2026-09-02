package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
// The plan prices those units as provider work, which is the upper bound on
// what the run can spend. The run itself finds the block store already holding
// an answer for each and serves them as drafts, so the figures it reports are
// the same units under a cheaper heading.
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

	// The record has been read now, so the plan judges every produced unit — and
	// the units the record declined to pair are the ones the next run drafts.
	priced := planJSON(t)
	assert.Zero(t, priced.Totals.UnreadTargets, "every committed target has been read")
	assert.Equal(t, 3, priced.Totals.Unanswered,
		"an identical translation with no approval behind it is drafted, not recycled")
	assert.Equal(t, 3, priced.Totals.AIRemaining)
	assert.Positive(t, priced.Totals.TokenEstimate, "priced work carries a token estimate")
	// The axes partition the work: everything counted is either recycled or drafted.
	assert.Equal(t,
		priced.Totals.MissingTarget+priced.Totals.Stale+priced.Totals.Unanswered,
		priced.Totals.MemoryExact+priced.Totals.AIRemaining)

	second := demoProviderApp(t)
	out2, err := runUp(t, second, proj)
	require.NoError(t, err, out2)
	_, drafts, ai := runCounts(t, out2)
	assert.Equal(t, priced.Totals.AIRemaining, drafts+ai,
		"the dry run counted the units the run that followed it produced: %s", out2)
	assert.Zero(t, ai,
		"and the store answered every one of them, so the run called no provider: %s", out2)
}
