package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The freshness reader exists because of a real failure: /pseudobench rendered
// results measured on 2026-05-20 from a file committed on 2026-07-03, and
// nothing on the page said so. Three other datasets carried no date at all.
//
// A stale dataset reads exactly like a fresh one, so these tests hold the
// reader to the two things that make it useful: it must find a date wherever a
// harness chose to put one, and it must say plainly when there is none.

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

func TestReadFreshnessFindsEverySpelling(t *testing.T) {
	dir := t.TempDir()
	cases := []struct{ name, body, wantDate string }{
		{"generated.json", `{"generated":"2026-08-20T10:00:00Z"}`, "2026-08-20T10:00:00Z"},
		{"camel.json", `{"generatedAt":"2026-08-20T10:00:00Z"}`, "2026-08-20T10:00:00Z"},
		{"snake.json", `{"generated_at":"2026-07-25"}`, "2026-07-25"},
		{"ranat.json", `{"ranAt":"2026-08-26T20:30:24Z"}`, "2026-08-26T20:30:24Z"},
		{"bare.json", `{"timestamp":"2026-05-20T21:11:31Z"}`, "2026-05-20T21:11:31Z"},
		// A wrapper holding reports by key, which is the shape skilleval writes.
		{"nested.json", `{"skill:trigger":{"generated":"2026-08-26T22:45:38Z"}}`, "2026-08-26T22:45:38Z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			write(t, dir, c.name, c.body)
			f := readFreshness(dir, c.name)
			assert.False(t, f.Undated)
			assert.Equal(t, c.wantDate, f.Date)
		})
	}
}

// TestTheIndexDoesNotBakeInToday.
//
// The reader records a date and never an age. An age would be computed at
// generate time and baked into a committed file, so the drift test that keeps
// this index honest would fail every morning and be muted inside a week.
// Building twice must produce identical bytes.
func TestTheIndexDoesNotBakeInToday(t *testing.T) {
	first, err := Build()
	require.NoError(t, err)
	a, err := Marshal(first)
	require.NoError(t, err)

	second, err := Build()
	require.NoError(t, err)
	b, err := Marshal(second)
	require.NoError(t, err)

	assert.Equal(t, string(a), string(b), "the index must be a pure function of the repository")
}

// TestReadFreshnessTakesTheNewest.
//
// A dataset that appends runs holds many dates. Reporting the first would call
// a suite stale on the strength of its oldest surviving entry, which is exactly
// backwards: what a reader wants to know is when it was last refreshed.
func TestReadFreshnessTakesTheNewest(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "history.json", `{
	  "runs": [
	    {"date": "2026-05-01"},
	    {"date": "2026-08-20"},
	    {"date": "2026-06-15"}
	  ]
	}`)
	f := readFreshness(dir, "history.json")
	assert.Equal(t, "2026-08-20", f.Date, "the newest entry is when this was last refreshed")
}

// TestUndatedIsReportedNotGuessed: a file with no date must say so rather than
// fall back to the file's mtime, which a fresh clone resets and a rebase
// rewrites. A wrong date is worse than a missing one.
func TestUndatedIsReportedNotGuessed(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "nodate.json", `{"generated_note":"Run the thing to regenerate.","total":{"f1":1.0}}`)
	f := readFreshness(dir, "nodate.json")
	assert.True(t, f.Undated)
	assert.Empty(t, f.Date)

	missing := readFreshness(dir, "not-here.json")
	assert.True(t, missing.Undated, "a dataset that is not there certainly cannot say how old it is")
}

// TestDeepDatesAreNotMistakenForTheDataset.
//
// The search is bounded on purpose. A per-case record several levels down
// carries its own date, and treating that as the dataset's would report a
// suite's age from whichever case happened to be first.
func TestDeepDatesAreNotMistakenForTheDataset(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "deep.json", `{"a":{"b":{"c":{"d":{"generated":"2020-01-01"}}}}}`)
	f := readFreshness(dir, "deep.json")
	assert.True(t, f.Undated, "a date four levels down is a record's, not the dataset's")
}

// TestCoverageCountsStaleAndUndated: these are the two numbers the audit turned
// up, so the summary has to carry them or the page is back where it started.
func TestCoverageCountsStaleAndUndated(t *testing.T) {
	index, err := Build()
	require.NoError(t, err)

	withData, undated := 0, 0
	for _, e := range index.Evals {
		if e.Data == "" {
			continue
		}
		withData++
		if e.Fresh.Undated {
			undated++
			continue
		}
		assert.NotEmpty(t, e.Fresh.Date, "%s is dated, so it must carry the date", e.ID)
	}
	assert.Positive(t, withData, "some evals publish data, or there is nothing to age")
	assert.Equal(t, undated, index.Coverage.Undated, "the summary must match the cards")
}
