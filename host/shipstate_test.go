package host

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/gate"
)

// A scope has one of three ship states. These tests pin how each reaches the
// surfaces a release decision reads (ship.json, the `kapi up` summary and the
// `kapi status` grid), and above all that a scope no ship gate matches is
// reported as not gated rather than shippable.

// shipRow is a coverage row in the given ship state, with the two-field reading
// RollupGates gives it: Shippable is false only for a withheld scope.
func shipRow(locale, collection string, gated bool, state ShipState) LocaleCoverage {
	return LocaleCoverage{
		Locale: locale, Collection: collection, Total: 1, Pct: map[string]int{},
		Gated: gated, Shippable: state != ShipStateWithheld, ShipState: state,
	}
}

// shipLineFor returns the first line of text whose first field is locale.
func shipLineFor(text, locale string) string {
	for ln := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), locale+" ") {
			return ln
		}
	}
	return ""
}

// TestBuildShipManifest_StatesEachLocale: ship.json carries the state, and a
// locale takes the weakest of its scopes, in the order withheld, not_gated,
// shippable. Shippable keeps its two-field meaning, so a picker that reads only
// it offers the locales it offered before.
func TestBuildShipManifest_StatesEachLocale(t *testing.T) {
	m := BuildShipManifest([]LocaleCoverage{
		shipRow("fr", "app", true, ShipStateShippable),
		shipRow("de", "app", true, ShipStateWithheld),
		shipRow("nb", "app", false, ShipStateNotGated),
		shipRow("sv", "app", true, ShipStateShippable),
		shipRow("sv", "docs", false, ShipStateNotGated),
		shipRow("ja", "app", false, ShipStateNotGated),
		shipRow("ja", "docs", false, ShipStateWithheld),
	})

	assert.Equal(t, ShipEntry{Shippable: true, State: ShipStateShippable}, m["fr"], "gated and clears its gate")
	assert.Equal(t, ShipEntry{State: ShipStateWithheld}, m["de"], "gated and short of its gate")
	assert.Equal(t, ShipEntry{Shippable: true, State: ShipStateNotGated}, m["nb"],
		"no gate matched: offered, and marked not gated")
	assert.Equal(t, ShipEntry{Shippable: true, State: ShipStateNotGated}, m["sv"],
		"part of sv has no gate, so sv makes no shippable claim")
	assert.Equal(t, ShipEntry{State: ShipStateWithheld}, m["ja"], "one withheld scope withholds the locale")
}

func convergeText(t *testing.T, out ConvergeOutput) string {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, out.FormatText(&b))
	return b.String()
}

// TestConvergeText_AllUngatedProjectClaimsNothing: a project that declares no
// ship gate converges, and the summary says no gate is declared. No row reads
// shippable, including a language at 0% translated.
func TestConvergeText_AllUngatedProjectClaimsNothing(t *testing.T) {
	text := convergeText(t, ConvergeOutput{Flow: "recycle-only", Passes: 1, Converged: true,
		Locales: []ConvergeLocaleResult{
			{Locale: "nb", Shippable: true, ShipState: ShipStateNotGated, Pct: map[string]int{"draft": 0, "translated": 0}},
			{Locale: "de", Shippable: true, ShipState: ShipStateNotGated, Pct: map[string]int{"draft": 100, "translated": 100}},
		}})

	assert.Contains(t, shipLineFor(text, "nb"), "not gated", text)
	assert.Contains(t, shipLineFor(text, "de"), "not gated", text)
	assert.NotContains(t, text, "shippable", text)
	assert.Contains(t, text, "Up to date. No ship gates are declared.", text)
}

// TestConvergeText_NamesTheLanguagesNoGateMatches: in a project that gates some
// languages, the gated ones keep their verdict and the rest are named as not
// gated.
func TestConvergeText_NamesTheLanguagesNoGateMatches(t *testing.T) {
	full := map[string]int{"draft": 100, "translated": 100}
	text := convergeText(t, ConvergeOutput{Flow: "translate", Passes: 1, Converged: true,
		Locales: []ConvergeLocaleResult{
			{Locale: "fr", Shippable: true, Gated: true, ShipState: ShipStateShippable, Pct: full},
			{Locale: "nb", Shippable: true, ShipState: ShipStateNotGated, Pct: full},
		}})

	assert.Contains(t, shipLineFor(text, "fr"), "✓ shippable", text)
	assert.Contains(t, shipLineFor(text, "nb"), "not gated", text)
	assert.NotContains(t, shipLineFor(text, "nb"), "shippable", text)
	assert.Contains(t, text, "Up to date: every gated scope is shippable. Not gated: nb.", text)
}

// TestStatusUngatedScopeReadsNotGated: the ship column names a scope no gate
// matches as not gated, and the summary says when no gate is declared at all.
func TestStatusUngatedScopeReadsNotGated(t *testing.T) {
	text := renderStatus(t, StatusOutput{Locales: []LocaleCoverage{covRow("de", nil, rep("", 2)...)}})

	assert.Contains(t, shipLineFor(text, "de"), "not gated", text)
	assert.NotContains(t, text, "ready", text)
	assert.Contains(t, text, "No ship gates are declared.", text)
}

// TestStatusSummaryCountsTheScopesNoGateMatches: the ready count stays a count
// of gated scopes, with the ungated ones counted beside it.
func TestStatusSummaryCountsTheScopesNoGateMatches(t *testing.T) {
	g := gate.Gate{"translated": {Pct: 100}}
	text := renderStatus(t, StatusOutput{Locales: []LocaleCoverage{
		covRow("fr", g, rep("translated", 2)...),
		covRow("de", nil, rep("translated", 2)...),
	}})

	assert.Contains(t, shipLineFor(text, "de"), "not gated", text)
	assert.Contains(t, text, "1 of 1 scopes ready to ship · 1 not gated", text)
}
