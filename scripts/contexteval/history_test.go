package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertReplacesSameDaySameExperiment(t *testing.T) {
	h := History{}
	h.Upsert(Run{Date: "2026-07-17", Provider: "gemini", Model: "m", Target: "de", CorpusDigest: "abc"})
	h.Upsert(Run{Date: "2026-07-17", Provider: "gemini", Model: "m", Target: "de", CorpusDigest: "abc", Repeat: 3})
	require.Len(t, h.Runs, 1)
	assert.Equal(t, 3, h.Runs[0].Repeat, "a same-day re-run corrects its entry")
}

func TestADifferentCorpusIsADifferentRun(t *testing.T) {
	h := History{}
	h.Upsert(Run{Date: "2026-07-17", Provider: "gemini", Model: "m", Target: "de", CorpusDigest: "abc"})
	h.Upsert(Run{Date: "2026-07-17", Provider: "gemini", Model: "m", Target: "de", CorpusDigest: "def"})
	assert.Len(t, h.Runs, 2, "a changed corpus is a different experiment, not a correction")
}

func TestADifferentTargetIsADifferentRun(t *testing.T) {
	h := History{}
	h.Upsert(Run{Date: "2026-07-17", Provider: "gemini", Model: "m", Target: "de", CorpusDigest: "abc"})
	h.Upsert(Run{Date: "2026-07-17", Provider: "gemini", Model: "m", Target: "fr", CorpusDigest: "abc"})
	assert.Len(t, h.Runs, 2)
}

func TestValidationUpsertReplacesSameJudgeAndRubric(t *testing.T) {
	h := History{}
	h.UpsertValidation(JudgeValidation{Provider: "gemini", Model: "g", RubricDigest: "r1", Kappa: 0.4})
	h.UpsertValidation(JudgeValidation{Provider: "gemini", Model: "g", RubricDigest: "r1", Kappa: 0.7})
	require.Len(t, h.JudgeValidations, 1)
	assert.Equal(t, 0.7, h.JudgeValidations[0].Kappa, "a re-validation supersedes")

	h.UpsertValidation(JudgeValidation{Provider: "gemini", Model: "g", RubricDigest: "r2", Kappa: 0.5})
	assert.Len(t, h.JudgeValidations, 2, "a new rubric needs its own validation")
}

func TestHistoryRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist.json")
	h := History{}
	h.Upsert(Run{
		Date: "2026-07-17", Provider: "gemini", Model: "m", Target: "de", CorpusDigest: "abc",
		Bare:    &PassRecord{InputTokens: 10},
		Steered: &PassRecord{InputTokens: 30},
		Dimensions: []DimensionRecord{
			{Dimension: DimTerminology, Bare: Counts{Scored: 10, Passed: 5}, Steered: Counts{Scored: 10, Passed: 9}},
		},
	})
	require.NoError(t, h.Save(path))
	got, err := LoadHistory(path)
	require.NoError(t, err)
	assert.Equal(t, h, got)

	lift, ok := got.Runs[0].Dimensions[0].Lift()
	require.True(t, ok)
	assert.InDelta(t, 40.0, lift, 1e-9)
}

func TestMissingHistoryIsEmptyNotError(t *testing.T) {
	h, err := LoadHistory(filepath.Join(t.TempDir(), "nope.json"))
	require.NoError(t, err)
	assert.Empty(t, h.Runs)
}

func TestSaveWithoutRunsWritesAnArrayNotNull(t *testing.T) {
	// -judge-validate against a fresh file saves a history that never had a
	// run appended; `"runs": null` would crash the dashboard's h.runs.filter.
	path := filepath.Join(t.TempDir(), "hist.json")
	h := History{}
	h.UpsertValidation(JudgeValidation{Provider: "gemini", Model: "g", RubricDigest: "r1"})
	require.NoError(t, h.Save(path))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"runs": []`)
	assert.NotContains(t, string(raw), `"runs": null`)
}

func TestLiftNeedsBothSides(t *testing.T) {
	d := DimensionRecord{Dimension: DimVoice, Steered: Counts{Scored: 5, Passed: 5}}
	_, ok := d.Lift()
	assert.False(t, ok, "half a differential is not a differential")
}
