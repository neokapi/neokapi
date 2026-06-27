package gate

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTargetLadder_Order(t *testing.T) {
	l := TargetLadder()
	assert.Equal(t, Ladder{"draft", "translated", "reviewed", "signed-off"}, l)
	assert.True(t, l.Has("reviewed"))
	assert.False(t, l.Has("nope"))
	assert.False(t, l.Has(string(model.TargetStatusNew)), "New is below every rung")
	// rank is monotone
	assert.Less(t, l.rank("draft"), l.rank("translated"))
	assert.Less(t, l.rank("reviewed"), l.rank("signed-off"))
	assert.Equal(t, -1, l.rank("signed_off"), "underscore form is not a rung")
}

func TestGate_Validate(t *testing.T) {
	l := TargetLadder()
	require.NoError(t, Gate{"translated": 100, "reviewed": 80}.Validate(l))
	require.Error(t, Gate{"bogus": 100}.Validate(l), "unknown state")
	require.Error(t, Gate{"reviewed": 101}.Validate(l), "out of range high")
	require.Error(t, Gate{"reviewed": -1}.Validate(l), "out of range low")
}

func TestSelector_MatchesAndSpecificity(t *testing.T) {
	catchAll := Selector{}
	assert.True(t, catchAll.Matches("docs", "nb"))
	assert.Equal(t, 0, catchAll.specificity())

	byLocale := Selector{Locales: []string{"ja", "ko"}}
	assert.True(t, byLocale.Matches("docs", "ja"))
	assert.False(t, byLocale.Matches("docs", "nb"))
	assert.Equal(t, 1, byLocale.specificity())

	both := Selector{Collections: []string{"legal"}, Locales: []string{"nb"}}
	assert.True(t, both.Matches("legal", "nb"))
	assert.False(t, both.Matches("legal", "ja"))
	assert.False(t, both.Matches("docs", "nb"))
	assert.Equal(t, 2, both.specificity())
}

func ruleSet() RuleSet {
	return RuleSet{Rules: []Rule{
		{When: Selector{Collections: []string{"docs"}}, Gate: Gate{"translated": 100, "reviewed": 50}},
		{When: Selector{Locales: []string{"ja"}}, Gate: Gate{"translated": 100, "reviewed": 0}},
		{When: Selector{Collections: []string{"legal"}, Locales: []string{"nb"}}, Gate: Gate{"signed-off": 100}},
		{When: Selector{}, Gate: Gate{"translated": 100, "reviewed": 100}}, // default
	}}
}

func TestResolve_MostSpecificWins(t *testing.T) {
	rs := ruleSet()

	// docs in nb → docs rule (1 axis) beats default (0).
	g, ok := rs.Resolve("docs", "nb")
	require.True(t, ok)
	assert.Equal(t, Gate{"translated": 100, "reviewed": 50}, g)

	// legal in nb → the 2-axis rule wins over default.
	g, ok = rs.Resolve("legal", "nb")
	require.True(t, ok)
	assert.Equal(t, Gate{"signed-off": 100}, g)

	// ui in de → only the default matches.
	g, ok = rs.Resolve("ui", "de")
	require.True(t, ok)
	assert.Equal(t, Gate{"translated": 100, "reviewed": 100}, g)

	// ja anything → the locale rule.
	g, ok = rs.Resolve("ui", "ja")
	require.True(t, ok)
	assert.Equal(t, Gate{"translated": 100, "reviewed": 0}, g)
}

func TestResolve_TieBreaksBySourceOrder(t *testing.T) {
	// Two single-axis rules both match docs-in-ja; the earlier-listed wins.
	rs := RuleSet{Rules: []Rule{
		{When: Selector{Collections: []string{"docs"}}, Gate: Gate{"reviewed": 50}},
		{When: Selector{Locales: []string{"ja"}}, Gate: Gate{"reviewed": 0}},
	}}
	g, ok := rs.Resolve("docs", "ja")
	require.True(t, ok)
	assert.Equal(t, Gate{"reviewed": 50}, g, "first matching same-specificity rule wins")
}

func TestResolve_NoMatch(t *testing.T) {
	rs := RuleSet{Rules: []Rule{
		{When: Selector{Locales: []string{"ja"}}, Gate: Gate{"translated": 100}},
	}}
	_, ok := rs.Resolve("docs", "nb")
	assert.False(t, ok, "no catch-all, no matching locale → no gate")
}

func TestCoverage_AtLeastPct(t *testing.T) {
	l := TargetLadder()
	// 4 units: 1 draft, 2 translated, 1 reviewed.
	c := NewCoverage([]string{"draft", "translated", "translated", "reviewed"})
	assert.Equal(t, 4, c.Total)
	assert.InDelta(t, 100, c.AtLeastPct(l, "draft"), 1e-9, "all ≥ draft")
	assert.InDelta(t, 75, c.AtLeastPct(l, "translated"), 1e-9, "3 of 4 ≥ translated")
	assert.InDelta(t, 25, c.AtLeastPct(l, "reviewed"), 1e-9, "1 of 4 ≥ reviewed")
	assert.InDelta(t, 0, c.AtLeastPct(l, "signed-off"), 1e-9)
}

func TestCoverage_EmptyScopeIsFullyCovered(t *testing.T) {
	l := TargetLadder()
	c := NewCoverage(nil)
	assert.Equal(t, 0, c.Total)
	assert.InDelta(t, 100, c.AtLeastPct(l, "reviewed"), 1e-9, "vacuous scope passes any gate")
}

func TestEvaluate_PassFailAndShortfalls(t *testing.T) {
	l := TargetLadder()
	c := NewCoverage([]string{"translated", "translated", "reviewed", "reviewed", "draft"}) // 5 units

	// 100% translated? 4 of 5 = 80 < 100 → fail. reviewed ≥ 40%? 2/5=40 → pass.
	res := Evaluate(Gate{"translated": 100, "reviewed": 40}, c, l)
	assert.False(t, res.Pass)
	require.Len(t, res.Shortfalls, 1)
	assert.Equal(t, "translated", res.Shortfalls[0].State)
	assert.Equal(t, 100, res.Shortfalls[0].Required)
	assert.InDelta(t, 80, res.Shortfalls[0].Actual, 1e-9)

	// A gate it satisfies.
	assert.True(t, Evaluate(Gate{"translated": 80, "reviewed": 40}, c, l).Pass)
}

func TestEvaluate_ZeroThresholdNotRequired(t *testing.T) {
	l := TargetLadder()
	c := NewCoverage([]string{"translated", "translated"}) // 0% reviewed
	// ja gate: machine ships — reviewed:0 must pass despite 0% review.
	res := Evaluate(Gate{"translated": 100, "reviewed": 0}, c, l)
	assert.True(t, res.Pass)
	assert.Empty(t, res.Shortfalls)
}

func TestEvaluate_ExactPercentageNoFloatTrip(t *testing.T) {
	l := TargetLadder()
	// 2/3 reviewed = 66.66…%; a 66% gate must pass, a 67% must fail.
	c := NewCoverage([]string{"reviewed", "reviewed", "translated"})
	assert.True(t, Evaluate(Gate{"reviewed": 66}, c, l).Pass)
	assert.False(t, Evaluate(Gate{"reviewed": 67}, c, l).Pass)
	// 80% reviewed over 5 units = exactly 80; an 80 gate must pass.
	c5 := NewCoverage([]string{"reviewed", "reviewed", "reviewed", "reviewed", "translated"})
	assert.True(t, Evaluate(Gate{"reviewed": 80}, c5, l).Pass)
}

// TestWorkedExample mirrors the design doc's nb/ja resolution + evaluation.
func TestWorkedExample(t *testing.T) {
	l := TargetLadder()
	rs := ruleSet()

	// A docs scope in nb resolves to {translated:100, reviewed:50}.
	g, _ := rs.Resolve("docs", "nb")
	// 10 docs-nb units: all translated, 6 reviewed → 100% translated, 60% reviewed.
	states := []string{}
	for range 6 {
		states = append(states, "reviewed")
	}
	for range 4 {
		states = append(states, "translated")
	}
	c := NewCoverage(states)
	assert.True(t, Evaluate(g, c, l).Pass, "docs-nb gate (50% review) met by 60% reviewed")

	// The same scope under the default gate (reviewed:100) would not ship.
	def, _ := rs.Resolve("ui", "nb")
	assert.False(t, Evaluate(def, c, l).Pass, "default gate needs 100% reviewed")
}
