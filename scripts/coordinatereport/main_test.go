package main

import (
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
func TestCommittedReportIsCurrent(t *testing.T) {
	report, err := Build(t.Context())
	require.NoError(t, err)
	fresh, err := Marshal(report)
	require.NoError(t, err)

	committed, err := os.ReadFile(filepath.Join("..", "..", DefaultOut))
	require.NoError(t, err, "the dashboard data is missing — run: go run ./scripts/coordinatereport")

	assert.Equal(t, string(committed), string(fresh),
		"the committed dashboard data is stale — regenerate with: go run ./scripts/coordinatereport")
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
