package main

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCorpus(t *testing.T) {
	units, err := loadCorpus("../..")
	require.NoError(t, err)

	counts := map[string]int{}
	byID := map[string]Unit{}
	for _, u := range units {
		counts[u.Corpus+"/"+string(u.Target)]++
		byID[u.Corpus+"/"+string(u.Target)+"/"+u.ID] = u
	}
	// The dogfood memory grows with every convergence, so its size is bounded
	// below rather than pinned.
	assert.Greater(t, counts["dogfood/nb"], 2000)
	assert.Positive(t, counts["compass/nb"])
	assert.Positive(t, counts["compass/de"])
	assert.Positive(t, counts["tidewatch/nb"])

	occupied, ok := byID["compass/nb/berths.occupied"]
	require.True(t, ok, "compass catalogs pair by key path")
	assert.Contains(t, model.RunsText(occupied.Source), "{vessel}",
		"a catalog read with no code finder keeps the ICU argument as text, which is the case #2674 describes")
}
