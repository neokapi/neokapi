package server

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShipManifestFromStatsMapsTheTwoGates(t *testing.T) {
	stats := &store.TranslationDashboardStats{
		LocaleStats: []store.LocaleTranslationStats{
			{Locale: "nb", ShipState: store.ShipStateGoverned},
			{Locale: "de", ShipState: store.ShipStateAIShippable},
			{Locale: "ja", ShipState: store.ShipStatePending},
			{Locale: "fr", ShipState: ""}, // no state derived yet
		},
	}

	m := shipManifestFromStats(stats)

	// governed: shippable AND verified (human-reviewed).
	assert.Equal(t, shipManifestEntry{Shippable: true, Verified: true}, m["nb"])
	// ai_shippable: shippable but unverified — the picker badges it "ai".
	assert.Equal(t, shipManifestEntry{Shippable: true, Verified: false}, m["de"])
	// pending: not shippable — the picker hides it.
	assert.Equal(t, shipManifestEntry{Shippable: false, Verified: false}, m["ja"])
	// empty (unshippable until derived): not shippable.
	assert.Equal(t, shipManifestEntry{Shippable: false, Verified: false}, m["fr"])
}

func TestShipManifestIsShapeIdenticalToShipJSON(t *testing.T) {
	// The public feed's body must be byte-shape-identical to the CLI's ship.json
	// (host.ShipManifest / host.ShipEntry) so the i18n-react picker consumes it
	// with no second code path: an object keyed by locale, each value
	// {"shippable":bool,"verified":bool}.
	stats := &store.TranslationDashboardStats{
		LocaleStats: []store.LocaleTranslationStats{
			{Locale: "nb", ShipState: store.ShipStateGoverned},
			{Locale: "ja", ShipState: store.ShipStatePending},
		},
	}
	body, err := json.Marshal(shipManifestFromStats(stats))
	require.NoError(t, err)

	var round map[string]map[string]any
	require.NoError(t, json.Unmarshal(body, &round))
	require.Contains(t, round, "nb")
	assert.Equal(t, true, round["nb"]["shippable"])
	assert.Equal(t, true, round["nb"]["verified"])
	assert.Equal(t, false, round["ja"]["shippable"])
	// Exactly the two keys ship.json carries — no extra fields leak in.
	assert.Len(t, round["nb"], 2)
}

func TestShipETagIsDeterministicAndBodySensitive(t *testing.T) {
	a := shipETag([]byte(`{"nb":{"shippable":true,"verified":true}}`))
	again := shipETag([]byte(`{"nb":{"shippable":true,"verified":true}}`))
	different := shipETag([]byte(`{"nb":{"shippable":true,"verified":false}}`))

	assert.Equal(t, a, again, "the same body yields the same validator")
	assert.NotEqual(t, a, different, "a changed manifest yields a new validator")
	assert.True(t, len(a) > 2 && a[0] == '"' && a[len(a)-1] == '"', "a quoted strong validator")
}
