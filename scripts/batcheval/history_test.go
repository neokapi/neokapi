package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleRun(date, provider, mdl string) Run {
	return Run{
		Date: date, Provider: provider, Model: mdl, Target: "de",
		CorpusDigest: Corpus().Digest(),
		Results:      []Result{{N: 16, Blocks: 30, Translated: 30}},
	}
}

// Re-running a sweep on the same day corrects it. If it appended instead, the same
// model would appear twice for one date and the second point would read as change
// over time when nothing changed but the operator pressing enter again.
func TestSameDayRerunCorrectsRatherThanAccumulates(t *testing.T) {
	t.Parallel()

	var h History
	h.Upsert(sampleRun("2026-07-14", "claude-code", "sonnet"))

	second := sampleRun("2026-07-14", "claude-code", "sonnet")
	second.Results = []Result{{N: 16, Blocks: 30, Missing: 3, Translated: 27}}
	h.Upsert(second)

	require.Len(t, h.Runs, 1, "one model, one target, one day = one point")
	assert.Equal(t, 3, h.Runs[0].Results[0].Missing, "the rerun must win")

	// A different day, or a different model, is a genuinely different point.
	h.Upsert(sampleRun("2026-08-01", "claude-code", "sonnet"), sampleRun("2026-07-14", "gemini", "gemini-3.5-flash"))
	assert.Len(t, h.Runs, 3)
}

// Same model, same day, different corpus = two experiments. Keying without the
// corpus digest made the 600-block sweep silently overwrite the 30-block one that
// had run an hour earlier, destroying the very comparison the big corpus was built
// to make.
func TestASweepOverADifferentCorpusIsADifferentRun(t *testing.T) {
	t.Parallel()

	small := sampleRun("2026-07-14", "gemini", "gemini-3.5-flash")

	big := sampleRun("2026-07-14", "gemini", "gemini-3.5-flash")
	big.CorpusDigest = CorpusN(600).Digest()
	big.CorpusBlocks = 600

	var h History
	h.Upsert(small, big)

	require.Len(t, h.Runs, 2, "the small corpus must survive the big one")
	assert.Equal(t, 600, h.Runs[0].CorpusBlocks, "the larger corpus sorts first")
}

// The history is only meaningful if runs are comparable. Change the corpus and
// they are not — so the digest must move, and the dashboard can then refuse to
// draw one line through two different experiments.
func TestCorpusDigestMovesWithTheCorpus(t *testing.T) {
	t.Parallel()

	base := Corpus()
	assert.Equal(t, base.Digest(), Corpus().Digest(), "the digest must be stable across runs, or every sweep looks incomparable")

	changed := Corpus()
	changed.Cases = append(changed.Cases, Case{Key: "new.string", Source: "Something new", Kind: "ui"})
	assert.NotEqual(t, base.Digest(), changed.Digest(),
		"a changed corpus is a different experiment; runs measured on it must not be plotted as a trend")

	edited := Corpus()
	edited.Cases[0].Source = "Reopen"
	assert.NotEqual(t, base.Digest(), edited.Digest(), "editing a case changes the corpus too")
}

// Runs are sorted by date so the dashboard can read the history in order without
// re-sorting, and so the committed JSON has a stable diff.
func TestHistoryIsOrderedByDate(t *testing.T) {
	t.Parallel()

	var h History
	h.Upsert(
		sampleRun("2026-09-01", "gemini", "gemini-3.5-flash"),
		sampleRun("2026-07-14", "claude-code", "sonnet"),
		sampleRun("2026-07-14", "claude-code", "opus"),
	)

	assert.Equal(t, []string{"2026-07-14", "2026-07-14", "2026-09-01"},
		[]string{h.Runs[0].Date, h.Runs[1].Date, h.Runs[2].Date})
	assert.Equal(t, "opus", h.Runs[0].Model, "same day: ordered by model, so the diff is stable")
}

// The first sweep has to be able to create the file it appends to.
func TestMissingHistoryIsEmptyNotAnError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "batcheval.json")
	h, err := LoadHistory(path)
	require.NoError(t, err)
	assert.Empty(t, h.Runs)

	h.Upsert(sampleRun("2026-07-14", "claude-code", "sonnet"))
	require.NoError(t, h.Save(path))

	back, err := LoadHistory(path)
	require.NoError(t, err)
	require.Len(t, back.Runs, 1)
	assert.Equal(t, Corpus().Digest(), back.Runs[0].CorpusDigest, "the corpus a run was measured on must survive the round trip")
}
