package convergence

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoverageTally_ScopesAndStale(t *testing.T) {
	tally := NewCoverageTally()
	tally.Add(Scope{Collection: "docs", Locale: "fr"}, "translated")
	tally.AddStale(Scope{Collection: "docs", Locale: "fr"}, false)
	tally.Add(Scope{Collection: "app", Locale: "nb"}, "")
	tally.Add(Scope{Collection: "docs", Locale: "de"}, "draft")

	assert.Equal(t, []Scope{
		{Collection: "app", Locale: "nb"},
		{Collection: "docs", Locale: "de"},
		{Collection: "docs", Locale: "fr"},
	}, tally.Scopes())
	assert.Equal(t, 1, tally.Stale(Scope{Collection: "docs", Locale: "fr"}))
	assert.Equal(t, 0, tally.Stale(Scope{Collection: "docs", Locale: "de"}))
	assert.Equal(t, 0, tally.Stale(Scope{Collection: "none", Locale: "xx"}))
	cov, _ := tally.Coverage(Scope{Collection: "docs", Locale: "fr"})
	assert.Equal(t, 1, cov.Counts["draft"], "a stale unit is counted at draft too")
}
