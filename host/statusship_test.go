package host

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/gate"
)

// The ship column is a gate verdict, not a percentage. These tests pin both
// halves of that: the verdict names the first unmet gate, and the pipeline
// column reports distance to the bar — derived from the existing gate model, so
// the number and the verdict can never disagree.

// covRow builds a LocaleCoverage the way RollupGates would, from a gate and a
// state distribution, so the rendering tests exercise the real derivation rather
// than hand-set fields.
func covRow(locale string, g gate.Gate, states ...string) LocaleCoverage {
	ladder := gate.TargetLadder()
	cov := gate.NewCoverage(states)
	lc := LocaleCoverage{Locale: locale, Total: cov.Total, Pct: map[string]int{}}
	for _, rung := range ladder {
		lc.Pct[rung] = int(cov.AtLeastPct(ladder, rung))
	}
	if len(g) == 0 {
		lc.Shippable = true
		lc.ShipProgress = 100
		return lc
	}
	res := gate.Evaluate(g, cov, ladder)
	lc.Gated = true
	lc.Shippable = res.Pass
	lc.Pending = res.Shortfalls
	lc.ShipProgress = res.Progress
	lc.Blocking = res.Blocking
	return lc
}

func renderStatus(t *testing.T, out StatusOutput) string {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, out.FormatText(&b))
	return b.String()
}

// TestStatusShipColumnIsAVerdict is the shape of the reworked view: every gated
// scope reads `ready` or `blocked: <gate>`, and never a percentage.
func TestStatusShipColumnIsAVerdict(t *testing.T) {
	twoBar := gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100}}
	threeBar := gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100}, "signed-off": {Pct: 100}}

	out := StatusOutput{Locales: []LocaleCoverage{
		covRow("de", twoBar, rep("translated", 4)...),
		covRow("fr", twoBar, rep("reviewed", 4)...),
		covRow("ja", twoBar, append(rep("translated", 1), rep("", 3)...)...),
		covRow("nb", threeBar, rep("reviewed", 4)...),
	}}

	text := renderStatus(t, out)
	lines := map[string]string{}
	for ln := range strings.SplitSeq(text, "\n") {
		for _, loc := range []string{"de", "fr", "ja", "nb"} {
			if strings.HasPrefix(strings.TrimSpace(ln), loc+" ") {
				lines[loc] = ln
			}
		}
	}
	require.Len(t, lines, 4, "one row per scope:\n%s", text)

	assert.Contains(t, lines["de"], "blocked: review", "translated but unreviewed")
	assert.Contains(t, lines["fr"], "ready", "clears every bar")
	assert.Contains(t, lines["ja"], "blocked: translate", "the lowest unmet gate, not review")
	assert.Contains(t, lines["nb"], "blocked: sign-off", "only the top bar is left")

	assert.NotContains(t, text, "shippable", "the verdict is `ready`, not a percentage-adjacent word")
	assert.Contains(t, text, "1 of 4 scopes ready to ship")
}

// TestStatusPipelineColumnTracksTheGate: the bar is distance to the ship bar, so
// its percentage must equal the gate progress and read 100 exactly when ready.
func TestStatusPipelineColumnTracksTheGate(t *testing.T) {
	twoBar := gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100}}

	de := covRow("de", twoBar, rep("translated", 4)...)
	assert.Equal(t, 50, de.ShipProgress, "half the bar cleared")
	assert.Equal(t, "reviewed", de.Blocking)

	fr := covRow("fr", twoBar, rep("reviewed", 4)...)
	assert.Equal(t, 100, fr.ShipProgress)
	assert.True(t, fr.Shippable)
	assert.Empty(t, fr.Blocking)

	text := renderStatus(t, StatusOutput{Locales: []LocaleCoverage{de, fr}})
	assert.Contains(t, text, "50%")
	assert.Contains(t, text, "100%")
	assert.Contains(t, text, "█", "a bar renders on a plain writer (UTF-8, not an escape sequence)")
	assert.Contains(t, text, "░")
}

// TestStatusUngatedScopeHasNoBarAndNoVerdict: with no gate there is nothing to
// be ready for, so both columns read as not-applicable rather than inventing a
// pass.
func TestStatusUngatedScopeHasNoBarAndNoVerdict(t *testing.T) {
	row := covRow("de", nil, rep("translated", 2)...)
	assert.False(t, row.Gated)
	assert.Equal(t, "—", pipelineCell(row, true))

	text := renderStatus(t, StatusOutput{Locales: []LocaleCoverage{row}})
	assert.NotContains(t, text, "ready to ship", "no gated scope means no ship summary")
	assert.NotContains(t, text, "blocked")
}

// TestStatusPipelineDegradesInANarrowTerminal keeps the number when the bar will
// not fit: the percentage is the information, the bar is only the affordance.
func TestStatusPipelineDegradesInANarrowTerminal(t *testing.T) {
	row := covRow("de", gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100}}, rep("translated", 4)...)

	wide := pipelineCell(row, true)
	assert.Contains(t, wide, "█")
	assert.Contains(t, wide, "50%")

	narrow := pipelineCell(row, false)
	assert.NotContains(t, narrow, "█")
	assert.NotContains(t, narrow, "░")
	assert.Contains(t, narrow, "50%")
}

// TestStatusAllReadySummary: the closing line is the number a release decision
// reads, so "all ready" must be stated as such.
func TestStatusAllReadySummary(t *testing.T) {
	g := gate.Gate{"translated": {Pct: 100}}
	out := StatusOutput{Locales: []LocaleCoverage{
		covRow("de", g, rep("translated", 2)...),
		covRow("fr", g, rep("reviewed", 2)...),
	}}
	assert.Contains(t, renderStatus(t, out), "2 of 2 scopes ready to ship")
}

// TestStatusBlockingLabelsAreActions: the ladder rungs are lifecycle states, but
// a verdict reads as the work that clears them.
func TestStatusBlockingLabelsAreActions(t *testing.T) {
	tests := map[string]string{
		"draft":      "draft",
		"translated": "translate",
		"reviewed":   "review",
		"signed-off": "sign-off",
	}
	for rung, want := range tests {
		t.Run(rung, func(t *testing.T) {
			assert.Equal(t, want, shipBlockingLabel(LocaleCoverage{Gated: true, Blocking: rung}))
		})
	}
	assert.Equal(t, "gate", shipBlockingLabel(LocaleCoverage{Gated: true}),
		"a blocked scope with no recorded rung still says something true")
	assert.Equal(t, "review", shipBlockingLabel(LocaleCoverage{
		Gated:   true,
		Pending: []gate.Shortfall{{State: "reviewed"}},
	}), "the label falls back to the first pending shortfall")
}

// TestStatusJSONCarriesTheVerdict: the machine-readable form must expose both
// the distance and the blocking gate, so a CI job or a dashboard can render the
// same verdict without re-deriving it.
func TestStatusJSONCarriesTheVerdict(t *testing.T) {
	t.Chdir(writeStatusProject(t))
	out := runStatusJSON(t)

	nb, ok := localeCoverage(out, "nb")
	require.True(t, ok)
	assert.True(t, nb.Gated)
	assert.False(t, nb.Shippable)
	assert.Equal(t, "translated", nb.Blocking, "nb is 67% translated against a 100% bar")
	assert.Greater(t, nb.ShipProgress, 0)
	assert.Less(t, nb.ShipProgress, 100)

	ja, ok := localeCoverage(out, "ja")
	require.True(t, ok)
	assert.Equal(t, "translated", ja.Blocking)
	assert.Equal(t, 0, ja.ShipProgress, "no ja file at all")
}

func rep(state string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = state
	}
	return out
}
