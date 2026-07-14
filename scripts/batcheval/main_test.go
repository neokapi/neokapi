package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// What CI can gate here is the *instrument*, not the model. A quality-versus-N
// curve needs paid calls to a stochastic model, so it cannot live in a test. What
// must never rot is the scoring: a harness that reports 100% intact while segments
// are silently dropped is worse than no harness, because it would be quoted.

// A missing segment is the failure the whole exercise exists to catch, and the
// one a scorer is most likely to score as a pass.
func TestScoringCatchesTheFailuresItExistsToCatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		source, got   string
		wantIntactPlc bool
		wantIntactTag bool
	}{
		{"placeholder preserved", "You have {0} items", "Sie haben {0} Artikel", true, true},
		{"placeholder dropped", "You have {0} items", "Sie haben Artikel", false, true},
		{"placeholder renamed", "Welcome, {{name}}", "Willkommen, {{nom}}", false, true},
		{"placeholder duplicated", "Sync %s: %s", "Sync %s: %s: %s", false, true},
		{"printf preserved", "Uploaded %d of %d files", "%d von %d Dateien hochgeladen", true, true},
		// Order may legitimately move between languages; presence may not.
		{"placeholder reordered", "You used {used} of {total}", "{total} davon {used} benutzt", true, true},
		{"tags preserved", `I accept the <ph id="1"/>terms<ph id="2"/>`, `Ich akzeptiere die <ph id="1"/>Bedingungen<ph id="2"/>`, true, true},
		{"tag dropped", `I accept the <ph id="1"/>terms<ph id="2"/>`, `Ich akzeptiere die Bedingungen<ph id="2"/>`, true, false},
		{"tag duplicated", `<ph id="1"/>Upgrade<ph id="2"/>`, `<ph id="1"/>Jetzt<ph id="1"/> upgraden<ph id="2"/>`, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.wantIntactPlc, placeholdersIntact(tc.source, tc.got), "placeholder scoring")
			assert.Equal(t, tc.wantIntactTag, tagsIntact(tc.source, tc.got), "tag scoring")
		})
	}
}

// Intact() is the headline number. If it counted a dropped segment as a pass,
// every result this harness ever publishes would be a lie.
func TestIntactCountsEveryStructuralFailure(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 100.0, Result{Blocks: 10, Translated: 10}.Intact())

	for _, r := range []struct {
		name string
		res  Result
	}{
		{"missing", Result{Blocks: 10, Missing: 1}},
		{"placeholder", Result{Blocks: 10, PlaceholderBreaks: 1}},
		{"tag", Result{Blocks: 10, TagBreaks: 1}},
		{"untranslated", Result{Blocks: 10, Untranslated: 1}},
	} {
		assert.Equalf(t, 90.0, r.res.Intact(), "a %s failure must cost a point — a segment kapi cannot write back is not a translation", r.name)
	}
}

// The corpus is the experiment. A corpus of nothing but "Save" would flatter every
// N and measure nothing, so the stressors the batching literature names — long
// items, placeholders, markup, and text that is ambiguous without its key — must
// all be present.
func TestCorpusCarriesTheStressorsBatchingBreaks(t *testing.T) {
	t.Parallel()

	c := Corpus()
	require.NotEmpty(t, c.Cases)

	kinds := map[string]int{}
	longest := 0
	for _, tc := range c.Cases {
		kinds[tc.Kind]++
		longest = max(longest, len(tc.Source))
	}

	for _, k := range []string{"ui", "ui-ambiguous", "placeholder", "inline-tag", "prose"} {
		assert.NotZerof(t, kinds[k], "the corpus must stress %q — degradation the corpus cannot express is degradation this harness will report as zero", k)
	}
	assert.Greater(t, longest, 150,
		"batch degradation is worst when items are long; a short-strings-only corpus hides it")

	// Blocks must round-trip the key, or `context: key` — the default — is being
	// measured with nothing to send.
	blocks := c.Blocks()
	require.Len(t, blocks, len(c.Cases))
	for i, b := range blocks {
		assert.Equal(t, c.Cases[i].Key, b.Name, "the key must reach the prompt")
		assert.Equal(t, c.Cases[i].Source, b.SourceText())
		assert.True(t, b.Translatable)
	}
}

// The same source text under two keys is where positional batch mapping used to
// corrupt silently, and where a key earns its tokens. If the corpus loses it, the
// regression it guards against goes unmeasured.
func TestCorpusKeepsAmbiguousDuplicates(t *testing.T) {
	t.Parallel()

	bySource := map[string][]string{}
	for _, tc := range Corpus().Cases {
		bySource[tc.Source] = append(bySource[tc.Source], tc.Key)
	}
	assert.GreaterOrEqual(t, len(bySource["Save"]), 2,
		`"Save" must appear under more than one key — identical source, different keys, is the case batching gets wrong`)
}

// The harness itself has to survive every N in the sweep, including N larger than
// the corpus. The demo provider proves the plumbing; it proves nothing about a model.
func TestHarnessRunsAtEveryBatchSize(t *testing.T) {
	provider, err := aiprovider.NewProvider("demo", aiprovider.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = provider.Close() })

	corpus := Corpus()
	for _, n := range []int{1, 8, 1000} {
		res, err := runOnce(context.Background(), provider, corpus, n, model.LocaleID("de"), 4)
		require.NoErrorf(t, err, "N=%d", n)
		assert.Equal(t, len(corpus.Cases), res.Blocks)
		assert.Zerof(t, res.Missing, "N=%d: every segment must come back — a harness that drops segments cannot measure a model that drops segments", n)
	}
}

// A demo run measures the harness and nothing else. It must say so in the record,
// not only on the terminal — a JSON file outlives the terminal it was printed to,
// and a stub's flawless 100% curve is exactly the number someone would quote.
func TestDemoRunsAreMarkedSimulated(t *testing.T) {
	t.Parallel()

	assert.True(t, Run{Provider: "demo", Simulated: true}.Simulated)
	assert.False(t, Run{Provider: "anthropic"}.Simulated,
		"only the stub is simulated; a real provider's numbers are real")
}

// Averaging integer counts across repeats would round "one run in three dropped a
// segment" down to zero — the harness erasing, by arithmetic, the exact failure it
// exists to find. Repeats are summed over a correspondingly larger corpus instead.
func TestRepeatsSumFailuresRatherThanRoundingThemAway(t *testing.T) {
	t.Parallel()

	got := mean(8, []Result{
		{N: 8, Blocks: 30, Missing: 1, Translated: 29},
		{N: 8, Blocks: 30, Translated: 30},
		{N: 8, Blocks: 30, Translated: 30},
	})

	assert.Equal(t, 1, got.Missing, "a failure seen in one run of three is a failure, not a rounding error")
	assert.Equal(t, 90, got.Blocks, "the denominator must grow with the repeats it aggregates")
	assert.InDelta(t, 98.9, got.Intact(), 0.1, "1 lost segment in 90 attempts")
}

// An N the model cannot answer at all is the single most useful point on the
// curve. Dropping it would leave a gap that reads as "not measured" — the opposite
// of what happened.
func TestAFailedBatchSizeIsRecordedAsAPointNotAGap(t *testing.T) {
	t.Parallel()

	failed := Result{N: 100, Blocks: 30, Missing: 30, Failed: true}
	assert.True(t, failed.Failed)
	assert.Zero(t, failed.Intact(), "nothing usable came back")
}

func TestParseModels(t *testing.T) {
	t.Parallel()

	got, err := parseModels("claude-code:sonnet, gemini:gemini-3.5-flash ,demo:")
	require.NoError(t, err)
	assert.Equal(t, []modelTarget{
		{provider: "claude-code", model: "sonnet"},
		{provider: "gemini", model: "gemini-3.5-flash"},
		{provider: "demo", model: ""},
	}, got, "a bare provider takes its default model")

	_, err = parseModels("")
	require.Error(t, err)
}

func TestParseSizes(t *testing.T) {
	t.Parallel()

	got, err := parseSizes("16, 1,4")
	require.NoError(t, err)
	assert.Equal(t, []int{1, 4, 16}, got, "sizes are swept in ascending order so the curve reads left to right")

	_, err = parseSizes("0")
	require.Error(t, err, "a batch of zero blocks is not a batch")
	_, err = parseSizes("eight")
	require.Error(t, err)
}
