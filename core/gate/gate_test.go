package gate

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
	require.NoError(t, Gate{"translated": {Pct: 100}, "reviewed": {Pct: 80}}.Validate(l))
	require.Error(t, Gate{"bogus": {Pct: 100}}.Validate(l), "unknown state")
	require.Error(t, Gate{"reviewed": {Pct: 101}}.Validate(l), "out of range high")
	require.Error(t, Gate{"reviewed": {Pct: -1}}.Validate(l), "out of range low")
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
		{When: Selector{Collections: []string{"docs"}}, Gate: Gate{"translated": {Pct: 100}, "reviewed": {Pct: 50}}},
		{When: Selector{Locales: []string{"ja"}}, Gate: Gate{"translated": {Pct: 100}, "reviewed": {Pct: 0}}},
		{When: Selector{Collections: []string{"legal"}, Locales: []string{"nb"}}, Gate: Gate{"signed-off": {Pct: 100}}},
		{When: Selector{}, Gate: Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100}}}, // default
	}}
}

func TestResolve_MostSpecificWins(t *testing.T) {
	rs := ruleSet()

	// docs in nb → docs rule (1 axis) beats default (0).
	g, ok := rs.Resolve("docs", "nb")
	require.True(t, ok)
	assert.Equal(t, Gate{"translated": {Pct: 100}, "reviewed": {Pct: 50}}, g)

	// legal in nb → the 2-axis rule wins over default.
	g, ok = rs.Resolve("legal", "nb")
	require.True(t, ok)
	assert.Equal(t, Gate{"signed-off": {Pct: 100}}, g)

	// ui in de → only the default matches.
	g, ok = rs.Resolve("ui", "de")
	require.True(t, ok)
	assert.Equal(t, Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100}}, g)

	// ja anything → the locale rule.
	g, ok = rs.Resolve("ui", "ja")
	require.True(t, ok)
	assert.Equal(t, Gate{"translated": {Pct: 100}, "reviewed": {Pct: 0}}, g)
}

func TestResolve_TieBreaksBySourceOrder(t *testing.T) {
	// Two single-axis rules both match docs-in-ja; the earlier-listed wins.
	rs := RuleSet{Rules: []Rule{
		{When: Selector{Collections: []string{"docs"}}, Gate: Gate{"reviewed": {Pct: 50}}},
		{When: Selector{Locales: []string{"ja"}}, Gate: Gate{"reviewed": {Pct: 0}}},
	}}
	g, ok := rs.Resolve("docs", "ja")
	require.True(t, ok)
	assert.Equal(t, Gate{"reviewed": {Pct: 50}}, g, "first matching same-specificity rule wins")
}

func TestResolve_NoMatch(t *testing.T) {
	rs := RuleSet{Rules: []Rule{
		{When: Selector{Locales: []string{"ja"}}, Gate: Gate{"translated": {Pct: 100}}},
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
	res := Evaluate(Gate{"translated": {Pct: 100}, "reviewed": {Pct: 40}}, c, l)
	assert.False(t, res.Pass)
	require.Len(t, res.Shortfalls, 1)
	assert.Equal(t, "translated", res.Shortfalls[0].State)
	assert.Equal(t, 100, res.Shortfalls[0].Required)
	assert.InDelta(t, 80, res.Shortfalls[0].Actual, 1e-9)

	// A gate it satisfies.
	assert.True(t, Evaluate(Gate{"translated": {Pct: 80}, "reviewed": {Pct: 40}}, c, l).Pass)
}

func TestEvaluate_ZeroThresholdNotRequired(t *testing.T) {
	l := TargetLadder()
	c := NewCoverage([]string{"translated", "translated"}) // 0% reviewed
	// ja gate: machine ships — reviewed:0 must pass despite 0% review.
	res := Evaluate(Gate{"translated": {Pct: 100}, "reviewed": {Pct: 0}}, c, l)
	assert.True(t, res.Pass)
	assert.Empty(t, res.Shortfalls)
}

func TestEvaluate_ExactPercentageNoFloatTrip(t *testing.T) {
	l := TargetLadder()
	// 2/3 reviewed = 66.66…%; a 66% gate must pass, a 67% must fail.
	c := NewCoverage([]string{"reviewed", "reviewed", "translated"})
	assert.True(t, Evaluate(Gate{"reviewed": {Pct: 66}}, c, l).Pass)
	assert.False(t, Evaluate(Gate{"reviewed": {Pct: 67}}, c, l).Pass)
	// 80% reviewed over 5 units = exactly 80; an 80 gate must pass.
	c5 := NewCoverage([]string{"reviewed", "reviewed", "reviewed", "reviewed", "translated"})
	assert.True(t, Evaluate(Gate{"reviewed": {Pct: 80}}, c5, l).Pass)
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

// --- Approver class (by: human|any) -----------------------------------------

// aiCoverage builds a scope of 4 units: 2 human-reviewed, 1 AI-approved
// (reads reviewed, baseline translated), 1 translated.
func aiCoverage() Coverage {
	var c Coverage
	c.Add("reviewed")
	c.Add("reviewed")
	c.AddAIDecided("reviewed", "translated")
	c.Add("translated")
	return c
}

func TestEvaluate_ShortFormExcludesAIDecisions(t *testing.T) {
	l := TargetLadder()
	c := aiCoverage()

	// Display view: all four read as translated-or-better, three as reviewed.
	assert.InDelta(t, 75.0, c.AtLeastPct(l, "reviewed"), 1e-9)
	assert.InDelta(t, 100.0, c.AtLeastPct(l, "translated"), 1e-9)

	// The legacy short form (reviewed: 75) defaults to human class: the
	// AI-approved unit does NOT count, so only 50% is human-reviewed.
	res := Evaluate(Gate{"reviewed": {Pct: 75}}, c, l)
	require.False(t, res.Pass, "short form requires human review — ai/ decisions must not satisfy it")
	require.Len(t, res.Shortfalls, 1)
	assert.InDelta(t, 50.0, res.Shortfalls[0].Actual, 1e-9)

	// The AI-approved unit still counts as translated for a translated
	// threshold (its human-class baseline).
	assert.True(t, Evaluate(Gate{"translated": {Pct: 100}}, c, l).Pass)
}

func TestEvaluate_ExplicitHumanMatchesShortForm(t *testing.T) {
	l := TargetLadder()
	c := aiCoverage()
	res := Evaluate(Gate{"reviewed": {Pct: 75, By: ByHuman}}, c, l)
	require.False(t, res.Pass)
	assert.Equal(t, ByHuman, res.Shortfalls[0].By)
}

func TestEvaluate_ByAnyAdmitsAIDecisions(t *testing.T) {
	l := TargetLadder()
	c := aiCoverage()
	assert.True(t, Evaluate(Gate{"reviewed": {Pct: 75, By: ByAny}}, c, l).Pass,
		"by: any admits ai/ approvals")
	assert.False(t, Evaluate(Gate{"reviewed": {Pct: 100, By: ByAny}}, c, l).Pass,
		"the plain translated unit is still short of reviewed")
}

func TestEvaluate_NoAIDecisionsHumanEqualsAny(t *testing.T) {
	l := TargetLadder()
	c := NewCoverage([]string{"reviewed", "reviewed"})
	assert.True(t, Evaluate(Gate{"reviewed": {Pct: 100}}, c, l).Pass)
	assert.True(t, Evaluate(Gate{"reviewed": {Pct: 100, By: ByAny}}, c, l).Pass)
	assert.Nil(t, c.HumanCounts, "no AI decisions → no separate human view materialized")
}

func TestValidate_ApproverClass(t *testing.T) {
	l := TargetLadder()
	require.NoError(t, Gate{"reviewed": {Pct: 100, By: ByHuman}}.Validate(l))
	require.NoError(t, Gate{"reviewed": {Pct: 100, By: ByAny}}.Validate(l))
	require.Error(t, Gate{"reviewed": {Pct: 100, By: "robot"}}.Validate(l), "unknown class")
}

func TestThreshold_YAMLRoundTrip(t *testing.T) {
	// Short form decodes to a bare percent and re-encodes as one.
	var g Gate
	require.NoError(t, yaml.Unmarshal([]byte("reviewed: 100\ntranslated: 80\n"), &g))
	assert.Equal(t, Gate{"reviewed": {Pct: 100}, "translated": {Pct: 80}}, g)
	out, err := yaml.Marshal(g)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "pct", "short form round-trips as a scalar")

	// Extended form carries the approver class.
	var g2 Gate
	require.NoError(t, yaml.Unmarshal([]byte("reviewed: {pct: 100, by: human}\ntranslated: 100\n"), &g2))
	assert.Equal(t, Gate{"reviewed": {Pct: 100, By: ByHuman}, "translated": {Pct: 100}}, g2)
	out2, err := yaml.Marshal(g2)
	require.NoError(t, err)
	assert.Contains(t, string(out2), "by: human")
}

func TestThreshold_JSONRoundTrip(t *testing.T) {
	var g Gate
	require.NoError(t, json.Unmarshal([]byte(`{"reviewed": 100, "signed-off": {"pct": 50, "by": "any"}}`), &g))
	assert.Equal(t, Gate{"reviewed": {Pct: 100}, "signed-off": {Pct: 50, By: ByAny}}, g)
	data, err := json.Marshal(g)
	require.NoError(t, err)
	assert.JSONEq(t, `{"reviewed": 100, "signed-off": {"pct": 50, "by": "any"}}`, string(data))
}

// TestProgress pins the distance-to-gate derivation: the mean fractional
// attainment of the gate's required thresholds, each capped at its requirement.
// It is what the `kapi status` pipeline column renders, so the numbers matter.
func TestProgress(t *testing.T) {
	ladder := TargetLadder()

	cov := func(states ...string) Coverage {
		return NewCoverage(states)
	}
	const (
		none       = ""
		translated = "translated"
		reviewed   = "reviewed"
		signedOff  = "signed-off"
	)

	tests := []struct {
		name string
		gate Gate
		cov  Coverage
		want int
	}{
		{
			name: "nothing required is vacuously complete",
			gate: Gate{},
			cov:  cov(none),
			want: 100,
		},
		{
			name: "zero-percent thresholds do not count as requirements",
			gate: Gate{translated: {Pct: 0}},
			cov:  cov(none),
			want: 100,
		},
		{
			name: "gate met",
			gate: Gate{translated: {Pct: 100}},
			cov:  cov(translated, translated),
			want: 100,
		},
		{
			name: "nothing done",
			gate: Gate{translated: {Pct: 100}},
			cov:  cov(none, none),
			want: 0,
		},
		{
			name: "half the units translated against a single 100% bar",
			gate: Gate{translated: {Pct: 100}},
			cov:  cov(translated, none),
			want: 50,
		},
		{
			name: "fully translated, unreviewed, two equal bars: half the gate",
			gate: Gate{translated: {Pct: 100}, reviewed: {Pct: 100}},
			cov:  cov(translated, translated),
			want: 50,
		},
		{
			name: "fully reviewed clears both bars",
			gate: Gate{translated: {Pct: 100}, reviewed: {Pct: 100}},
			cov:  cov(reviewed, reviewed),
			want: 100,
		},
		{
			name: "exceeding one bar never compensates for missing another",
			gate: Gate{translated: {Pct: 50}, reviewed: {Pct: 100}},
			cov:  cov(translated, translated),
			want: 50,
		},
		{
			name: "three bars, the top one 40% attained",
			gate: Gate{translated: {Pct: 100}, reviewed: {Pct: 100}, signedOff: {Pct: 100}},
			cov:  cov(reviewed, reviewed, reviewed, reviewed, reviewed, signedOff, signedOff, signedOff, signedOff, reviewed),
			want: 80,
		},
		{
			name: "a vacuous scope satisfies every bar",
			gate: Gate{translated: {Pct: 100}, reviewed: {Pct: 100}},
			cov:  Coverage{},
			want: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Progress(tt.gate, tt.cov, ladder))
			// Evaluate must agree: the status view reads Progress off the Result.
			assert.Equal(t, tt.want, Evaluate(tt.gate, tt.cov, ladder).Progress)
		})
	}
}

// TestEvaluateBlockingIsTheLowestUnmetRung: a ship verdict names the gate to
// clear next, so it must be the lowest unmet rung — pointing at "sign-off"
// while translation is also short would send someone to the wrong work.
func TestEvaluateBlockingIsTheLowestUnmetRung(t *testing.T) {
	ladder := TargetLadder()
	g := Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100}, "signed-off": {Pct: 100}}

	res := Evaluate(g, NewCoverage([]string{"", "translated"}), ladder)
	assert.False(t, res.Pass)
	assert.Equal(t, "translated", res.Blocking, "translation is short, so it blocks first")

	res = Evaluate(g, NewCoverage([]string{"reviewed", "reviewed"}), ladder)
	assert.False(t, res.Pass)
	assert.Equal(t, "signed-off", res.Blocking, "only the top rung is left")

	res = Evaluate(g, NewCoverage([]string{"signed-off"}), ladder)
	assert.True(t, res.Pass)
	assert.Empty(t, res.Blocking, "a passing gate blocks on nothing")
}

// TestProgressWithApproverClass: an AI-promoted unit does not advance a
// human-class bar, so progress must read it at its baseline too — the number and
// the verdict cannot disagree.
func TestProgressWithApproverClass(t *testing.T) {
	ladder := TargetLadder()
	var cov Coverage
	cov.AddAIDecided("reviewed", "translated")
	cov.AddAIDecided("reviewed", "translated")

	human := Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100, By: ByHuman}}
	assert.Equal(t, 50, Progress(human, cov, ladder),
		"AI review does not advance a human-class bar")

	any := Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100, By: ByAny}}
	assert.Equal(t, 100, Progress(any, cov, ladder),
		"`by: any` admits the AI decision")
}
