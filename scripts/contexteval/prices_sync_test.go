package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPricesMatchBatcheval pins the two embedded rate tables to each other.
// batcheval's prices.json is canonical (refreshed by `make update-model-prices`,
// which also copies it here); two silently diverging tables would price the
// same run differently depending on which eval measured it.
func TestPricesMatchBatcheval(t *testing.T) {
	canonical, err := os.ReadFile("../batcheval/prices.json")
	require.NoError(t, err)
	require.Equal(t, string(canonical), string(pricesJSON),
		"scripts/contexteval/prices.json has drifted from scripts/batcheval/prices.json — run `make update-model-prices` (or copy the file)")
}
