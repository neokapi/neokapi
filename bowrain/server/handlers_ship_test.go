package server

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShipManifestFromStatsMapsTheTwoGates(t *testing.T) {
	stats := &store.TranslationDashboardStats{
		LocaleStats: []store.LocaleTranslationStats{
			{Locale: "nb", ShipState: store.ShipStateGoverned, ComplianceBasis: store.ComplianceBasisChecksTerms},
			{Locale: "sv", ShipState: store.ShipStateApproved, ComplianceBasis: store.ComplianceBasisChecks},
			{Locale: "de", ShipState: store.ShipStateAIShippable, ComplianceBasis: store.ComplianceBasisVoice},
			{Locale: "ja", ShipState: store.ShipStatePending},
			{Locale: "fr", ShipState: ""}, // no state derived yet
		},
	}

	m := shipManifestFromStats(stats)

	// governed: shippable AND verified (human-reviewed), and terminology governs it.
	assert.Equal(t, shipManifestEntry{Shippable: true, Verified: true}, m["nb"])
	// approved: shippable AND verified, and terminology governs nothing there.
	assert.Equal(t, shipManifestEntry{Shippable: true, Verified: true, NotGoverned: []string{"terms"}}, m["sv"])
	// ai_shippable: shippable but unverified, so the picker badges it "ai". Its
	// basis leaves terminology out, and the entry says so.
	assert.Equal(t, shipManifestEntry{Shippable: true, NotGoverned: []string{"terms"}}, m["de"])
	// pending: not shippable, so the picker hides it. No basis was derived, so
	// nothing is claimed about governance.
	assert.Equal(t, shipManifestEntry{}, m["ja"])
	// empty (unshippable until derived): not shippable.
	assert.Equal(t, shipManifestEntry{}, m["fr"])
}

func TestShipManifestIsShapeIdenticalToShipJSON(t *testing.T) {
	// The public feed's body must be shape-identical to the CLI's ship.json
	// (host.ShipManifest / host.ShipEntry) so the i18n-react picker consumes it
	// with no second code path: an object keyed by locale, each value
	// {"shippable":bool,"verified":bool}, plus "not_governed" where a dimension
	// governs nothing in the locale.
	stats := &store.TranslationDashboardStats{
		LocaleStats: []store.LocaleTranslationStats{
			{Locale: "nb", ShipState: store.ShipStateGoverned, ComplianceBasis: store.ComplianceBasisChecksTerms},
			{Locale: "sv", ShipState: store.ShipStateApproved, ComplianceBasis: store.ComplianceBasisChecks},
			{Locale: "ja", ShipState: store.ShipStatePending},
		},
	}
	body, err := json.Marshal(shipManifestFromStats(stats))
	require.NoError(t, err)

	cli, err := json.Marshal(host.ShipManifest{
		"nb": {Shippable: true, Verified: true},
		"sv": {Shippable: true, Verified: true, NotGoverned: []string{"terms"}},
		"ja": {},
	})
	require.NoError(t, err)
	assert.JSONEq(t, string(cli), string(body), "the feed and ship.json carry the same keys and values")
}

func TestShipETagIsDeterministicAndBodySensitive(t *testing.T) {
	a := shipETag([]byte(`{"nb":{"shippable":true,"verified":true}}`))
	again := shipETag([]byte(`{"nb":{"shippable":true,"verified":true}}`))
	different := shipETag([]byte(`{"nb":{"shippable":true,"verified":false}}`))

	assert.Equal(t, a, again, "the same body yields the same validator")
	assert.NotEqual(t, a, different, "a changed manifest yields a new validator")
	assert.True(t, len(a) > 2 && a[0] == '"' && a[len(a)-1] == '"', "a quoted strong validator")
}
