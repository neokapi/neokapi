package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommittedReportIsCurrent is the guard that keeps the dashboard honest.
//
// Every number on /coordinate is computed by calling the real resolver, corpus
// and prompt builder. That only means anything while the committed file is what
// the current code produces — otherwise the page keeps asserting a finding long
// after the code stopped supporting it, which is the exact failure the
// generated-not-typed discipline exists to prevent.
//
// The comparison takes the committed file's provenance rather than the fresh
// run's. Generated and Commit record WHEN the data was last built, not WHAT it
// says, and the report is deterministic — so comparing them asserts only that
// the file was regenerated at the commit under test, which no commit but the
// one that last touched it can satisfy. A gate that fails on every merge
// regardless of drift reports nothing about drift, and teaches its readers to
// regenerate on reflex instead of asking what changed.
//
// Everything else is compared byte for byte, formatting included, so a hand
// edit to the file is still caught.
func TestCommittedReportIsCurrent(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join("..", "..", DefaultOut))
	require.NoError(t, err, "the dashboard data is missing — run: go run ./scripts/coordinatereport")

	var stamp struct {
		Generated string `json:"generated"`
		Commit    string `json:"commit"`
	}
	require.NoError(t, json.Unmarshal(committed, &stamp),
		"the committed dashboard data is not valid JSON")

	report, err := Build(t.Context())
	require.NoError(t, err)
	report.Generated = stamp.Generated
	report.Commit = stamp.Commit

	fresh, err := Marshal(report)
	require.NoError(t, err)

	assert.Equal(t, string(committed), string(fresh),
		"the committed dashboard data is stale — regenerate with: go run ./scripts/coordinatereport")
}

// TestCommittedReportSurvivesANewCommit is the regression on the gate itself.
//
// The report stamps the commit it was built at, so a gate that compared the
// stamp went red the moment anything else merged and stayed red until someone
// regenerated — which held only until the next merge. Content equal, provenance
// different, must pass.
func TestCommittedReportSurvivesANewCommit(t *testing.T) {
	report, err := Build(t.Context())
	require.NoError(t, err)

	report.Generated = "2020-01-01T00:00:00Z"
	report.Commit = "0000000"
	stamped, err := Marshal(report)
	require.NoError(t, err)

	report.Generated = "2026-12-31T23:59:59Z"
	report.Commit = "fffffff"
	restamped, err := Marshal(report)
	require.NoError(t, err)

	assert.NotEqual(t, string(stamped), string(restamped),
		"the provenance is in the file, which is what made the naive gate fail")

	var a, b map[string]any
	require.NoError(t, json.Unmarshal(stamped, &a))
	require.NoError(t, json.Unmarshal(restamped, &b))
	delete(a, "generated")
	delete(a, "commit")
	delete(b, "generated")
	delete(b, "commit")
	assert.Equal(t, a, b, "and everything the report says is the same either way")
}

// TestReportProvesTheGateBothWays: the report is only worth publishing if it
// shows the gate refusing as well as allowing. A page of green rows proves
// nothing about a gate.
func TestReportProvesTheGateBothWays(t *testing.T) {
	report, err := Build(t.Context())
	require.NoError(t, err)

	var offered, withheld int
	for _, c := range report.Chains {
		if c.Offered != nil {
			offered++
			assert.Empty(t, c.Withheld, "an offered answer needs no excuse")
			continue
		}
		withheld++
		assert.NotEmpty(t, c.Withheld, "a refusal must say why, or the page teaches nothing")
	}

	assert.Positive(t, offered, "the gate must let a governed answer through")
	assert.Positive(t, withheld, "and must be seen refusing one")
}

// TestPromptPairsDifferWhenAReferenceIsOffered: the side-by-side has to show a
// difference where there is one, and none where there is not.
//
// The digest check is the load-bearing half. A prior version that reached the
// prompt without moving the cache key would be served to a block whose chain
// had since moved — the reference would be right once and stale forever after.
func TestPromptPairsDifferWhenAReferenceIsOffered(t *testing.T) {
	report, err := Build(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, report.Prompts)

	for _, p := range report.Prompts {
		if p.Withheld != "" {
			assert.Len(t, p.With, len(p.Without),
				"%s: nothing was offered, so the two prompts are the same prompt", p.Case)
			assert.Equal(t, p.Digests.Without, p.Digests.With,
				"%s: and the cache key does not move", p.Case)
			continue
		}
		assert.Greater(t, len(p.With), len(p.Without),
			"%s: an offered reference is a section the model actually receives", p.Case)
		assert.NotEqual(t, p.Digests.Without, p.Digests.With,
			"%s: and it must move the cache key", p.Case)
	}
}
