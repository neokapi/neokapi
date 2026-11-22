package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/convergence"
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
	cov := []LocaleCoverage{{Locale: "nb", Total: 60}, {Locale: "nl", Total: 60}}
	require.NoError(t, checkUnitCounts(cov, map[string]int{"nb": 60}, map[string]int{"nl": 60}))

	err := checkUnitCounts(cov, map[string]int{"nb": 61}, nil)
	require.ErrorContains(t, err, "nb counts 61 produced and 0 waiting for review for 60 unit(s)")

	require.Error(t, checkUnitCounts(cov, map[string]int{"nl": 1}, map[string]int{"nl": 60}))
	require.Error(t, checkUnitCounts(cov, map[string]int{"de": 1}, nil), "a language the run has no units for")
}
