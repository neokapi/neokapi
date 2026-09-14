package server

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/stretchr/testify/assert"
)

// TestShipManifestFromStatsStatesEveryLocale: the public feed carries the state
// ship.json carries. The server holds every locale to the bar
// store.DeriveShipState applies (fully translated and clear of the checks), so
// a locale reads shippable or withheld. None reads not_gated: an untranslated
// locale is pending, and pending is withheld.
func TestShipManifestFromStatsStatesEveryLocale(t *testing.T) {
	m := shipManifestFromStats(&store.TranslationDashboardStats{
		LocaleStats: []store.LocaleTranslationStats{
			{Locale: "nb", ShipState: store.ShipStateGoverned},
			{Locale: "sv", ShipState: store.ShipStateApproved},
			{Locale: "de", ShipState: store.ShipStateAIShippable},
			{Locale: "ja", ShipState: store.ShipStatePending},
			{Locale: "fr", ShipState: ""}, // no state derived yet
		},
	})

	for _, loc := range []string{"nb", "sv", "de"} {
		assert.True(t, m[loc].Shippable, loc)
		assert.Equal(t, convergence.ShipStateShippable, m[loc].State, loc)
	}
	for _, loc := range []string{"ja", "fr"} {
		assert.False(t, m[loc].Shippable, loc)
		assert.Equal(t, convergence.ShipStateWithheld, m[loc].State, loc)
	}
}
