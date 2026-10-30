package profile

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// entity is a character reference as the HTML reader keeps it: an inline code
// carrying the reference's bytes.
func entity(id, data string) model.Run {
	return model.PhR(model.PlaceholderRun{ID: id, Type: entityCodeType, Data: data})
}

// A character reference beside a word ends the word, as the page shows it,
// and every hit indexes the block's text, where the reference has no width.
// Other codes keep their zero width: a bold word is still the word. A hit
// whose first or last character a reference spells is reported as the page
// shows it ("spelled:" below), because no span of the block's text holds it.
func TestMatchTermRulesInRunsReadsCharacterReferences(t *testing.T) {
	rules := []TermRuleSet{{Rules: []TermRule{{Term: "utilize", Replacement: "use"}, {Term: "café", Replacement: "coffee shop"}, {Term: "école", Replacement: "school"}}}}
	bold := func(id string) (model.Run, model.Run) {
		return model.PcOpenR(model.PcOpenRun{ID: id, Type: "fmt:bold", Data: "<b>"}),
			model.PcCloseR(model.PcCloseRun{ID: id, Type: "fmt:bold", Data: "</b>"})
	}
	open, closing := bold("9")
	cases := []struct {
		name string
		runs []model.Run
		want []string // each hit: its text in the block's text, or "spelled:" and the page's spelling
		at   []int    // the start of each hit in the block's text
	}{
		{"before a reference", []model.Run{model.TextR("We utilize"), entity("1", "&amp;"), model.TextR("more")}, []string{"utilize"}, []int{3}},
		{"after a reference", []model.Run{model.TextR("Say"), entity("1", "&nbsp;"), model.TextR("utilize it")}, []string{"utilize"}, []int{3}},
		{"between references", []model.Run{model.TextR("a"), entity("1", "&mdash;"), model.TextR("utilize"), entity("2", "&#160;"), model.TextR("b")}, []string{"utilize"}, []int{1}},
		{"a reference that spells a letter continues the word", []model.Run{model.TextR("We utiliz"), entity("1", "&#101;"), model.TextR("d it")}, nil, nil},
		{"a reference that spells the last letter", []model.Run{model.TextR("the caf"), entity("1", "&eacute;"), model.TextR(" menu")}, []string{"spelled:café"}, []int{4}},
		{"a numeric reference that spells the last letter", []model.Run{model.TextR("We utiliz"), entity("1", "&#101;"), model.TextR(" it")}, []string{"spelled:utilize"}, []int{3}},
		{"a reference that spells the first letter", []model.Run{model.TextR("the "), entity("1", "&Eacute;"), model.TextR("cole")}, []string{"spelled:École"}, []int{4}},
		{"an unknown reference ends a word", []model.Run{model.TextR("utilize"), entity("1", "&nosuch;"), model.TextR("x")}, []string{"utilize"}, []int{0}},
		{"a bold word keeps its zero-width codes", []model.Run{model.TextR("We "), open, model.TextR("utilize"), closing, model.TextR(" it"), entity("1", "&amp;")}, []string{"utilize"}, []int{3}},
		{"codes that are not references join the text", []model.Run{model.TextR("util"), open, model.TextR("ize"), closing, model.TextR("more")}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := model.RunsText(tc.runs)
			hits := MatchTermRulesInRuns(rules, text, tc.runs)
			var got []string
			var at []int
			for _, h := range hits {
				if h.Spelled != "" {
					got = append(got, "spelled:"+h.Spelled)
				} else {
					got = append(got, text[h.Start:h.End])
				}
				at = append(at, h.Start)
			}
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.at, at)
		})
	}
}

// A hit's run anchor and the span a fix replaces come from the block's runs:
// the word before a reference is a range inside its own text run. A hit a
// reference spells an edge of is quoted as the page shows it with no position,
// so no replacement is built over the reference.
func TestMatchTermRulesInRunsAnchorsInTheTextRun(t *testing.T) {
	rules := []TermRuleSet{{Rules: []TermRule{{Term: "utilize", Replacement: "use"}, {Term: "café", Replacement: "coffee shop"}}}}
	runs := []model.Run{model.TextR("and   utilize"), entity("1", "&amp;"), model.TextR("more caf"), entity("2", "&eacute;")}
	text := model.RunsText(runs)
	findings := HitsToFindings(MatchTermRulesInRuns(rules, text, runs), text, runs)
	require.Len(t, findings, 2)
	assert.Equal(t, "utilize", findings[0].OriginalText)
	assert.Equal(t, model.RangeAnchorForBytes(runs, 6, 13), findings[0].Position)
	assert.Equal(t, "café", findings[1].OriginalText)
	assert.Equal(t, model.Anchor{}, findings[1].Position)
}

// Text the runs do not flatten to is searched as given.
func TestSearchTextKeepsTextTheRunsDoNotSpell(t *testing.T) {
	runs := []model.Run{model.TextR("a"), entity("1", "&amp;"), model.TextR("b")}
	s := NewSearchText("something else", runs)
	assert.Equal(t, "something else", s.Text)
	assert.Equal(t, 3, s.ToText(3))

	s = NewSearchText("ab", runs)
	assert.Equal(t, "a&b", s.Text)
	assert.Equal(t, []int{0, 1, 1, 2}, []int{s.ToText(0), s.ToText(1), s.ToText(2), s.ToText(3)})
	assert.True(t, s.SpelledAtEdge(1, 2))
	assert.False(t, s.SpelledAtEdge(0, 1))
}

// referenceRules are the rules the reference cases below are matched under.
var referenceRules = []TermRuleSet{{Rules: []TermRule{
	{Term: "utilize", Replacement: "use"},
	{Term: "next", Replacement: "following"},
	{Term: "café", Replacement: "coffee shop"},
}}}

// hitView names each hit by the text it covers, or by "spelled:" and the
// page's spelling, with its start.
func hitView(text string, hits []VocabHit) []string {
	var out []string
	for _, h := range hits {
		if h.Spelled != "" {
			out = append(out, fmt.Sprintf("spelled:%s@%d", h.Spelled, h.Start))
			continue
		}
		out = append(out, fmt.Sprintf("%s@%d", text[h.Start:h.End], h.Start))
	}
	return out
}

// Text with no runs is read as written, as `kapi check` reads a block of one
// text run: a reference written in it is its own characters, so it ends a
// word with its "&" and ";" and spells nothing.
func TestMatchTermRulesInTextReadsTextAsWritten(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"before a reference", "utilize&amp;more", []string{"utilize@0"}},
		{"after a reference", "word&nbsp;next", []string{"next@10"}},
		{"between references", "&amp;utilize&nbsp;", []string{"utilize@5"}},
		{"a reference that would spell the last letter", "caf&eacute;", nil},
		{"a reference that would spell a letter inside a word", "utiliz&#101;d", nil},
		{"an ampersand that is no reference", "R&D: utilize & next", []string{"utilize@5", "next@15"}},
		{"no reference", "We utilize the next café", []string{"utilize@3", "next@15", "café@20"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := MatchTermRulesInText(referenceRules, tc.text)
			assert.Equal(t, tc.want, hitView(tc.text, hits))
			assert.Equal(t, MatchTermRulesInRuns(referenceRules, tc.text, []model.Run{model.TextR(tc.text)}), hits,
				"the plain form is the runs form over one text run")
		})
	}
}
