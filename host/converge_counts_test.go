package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
)

// A unit the gate holds waits for review whatever produced it: a locale whose
// every held unit came from the content memory reports them, as a drafted
// locale does. A locale that ships holds nothing.
func TestHeldDraftsCountsRecycledUnits(t *testing.T) {
	cov := []LocaleCoverage{
		{Locale: "nb", Total: 10, Shippable: false},
		{Locale: "de", Total: 8, Shippable: false},
		{Locale: "fr", Total: 5, Shippable: true},
	}
	production := map[string]convergence.PassProduction{
		"nb": {Done: 10, ViaMemory: 10},
		"de": {Done: 8, ViaMemory: 3, ViaAI: 4, ViaDraft: 1},
		"fr": {Done: 5, ViaAI: 5},
	}
	assert.Equal(t, map[string]int{"nb": 10, "de": 8}, heldDrafts(cov, production))
}

// The summary refuses counts above a language's units: the written and the
// held units are disjoint subsets of it.
func TestCheckUnitCounts(t *testing.T) {
	nb := editionAt{locale: "nb", edition: "nb"}
	nl := editionAt{locale: "nl", edition: "nl"}
	cov := []LocaleCoverage{{Locale: "nb", Total: 60}, {Locale: "nl", Total: 60}}
	require.NoError(t, checkUnitCounts(cov, map[editionAt]int{nb: 60}, map[string]int{"nl": 60}))

	err := checkUnitCounts(cov, map[editionAt]int{nb: 61}, nil)
	require.ErrorContains(t, err, "nb counts 61 produced and 0 waiting for review for 60 unit(s)")

	require.Error(t, checkUnitCounts(cov, map[editionAt]int{nl: 1}, map[string]int{"nl": 60}))
	require.Error(t, checkUnitCounts(cov, map[editionAt]int{{locale: "de", edition: "de"}: 1}, nil), "a language the run has no units for")
	require.Error(t, checkUnitCounts(cov, nil, map[string]int{"nb": 61}), "held units alone above the units")
}

// Two editions of one language (a tone variant beside the plain one) each
// translate the language's units: a block written in both counts once in
// each, and a correct run is not refused.
func TestCheckUnitCountsCountsEachEditionAgainstTheUnits(t *testing.T) {
	cov := []LocaleCoverage{{Locale: "nb", Total: 2}}
	var tally unitWriteTally
	formal, err := model.ParseEditionKey("nb;tone=formal")
	require.NoError(t, err)
	plain, err := model.ParseEditionKey("nb")
	require.NoError(t, err)
	for _, block := range []string{"a", "b"} {
		tally.add(
			unitOf(change.Ref{Doc: "src/app.json", Block: block, Edition: plain}),
			unitOf(change.Ref{Doc: "src/app.json", Block: block, Edition: formal}),
		)
	}
	require.Equal(t, 4, tally.total())
	require.NoError(t, checkUnitCounts(cov, tally.byEdition(), nil))

	tally.add(unitOf(change.Ref{Doc: "src/other.json", Block: "c", Edition: formal}))
	require.ErrorContains(t, checkUnitCounts(cov, tally.byEdition(), nil), "nb;tone=formal counts 3 produced")
}
