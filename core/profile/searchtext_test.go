package profile

import (
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
// Other codes keep their zero width: a bold word is still the word.
func TestMatchTermRulesInRunsReadsCharacterReferences(t *testing.T) {
	rules := []TermRuleSet{{Rules: []TermRule{{Term: "utilize", Replacement: "use"}, {Term: "café"}}}}
	bold := func(id string) (model.Run, model.Run) {
		return model.PcOpenR(model.PcOpenRun{ID: id, Type: "fmt:bold", Data: "<b>"}),
			model.PcCloseR(model.PcCloseRun{ID: id, Type: "fmt:bold", Data: "</b>"})
	}
	open, closing := bold("9")
	cases := []struct {
		name string
		runs []model.Run
		want []string // the text of each hit, from the block's text
		at   []int    // the start of each hit in the block's text
	}{
		{"before a reference", []model.Run{model.TextR("We utilize"), entity("1", "&amp;"), model.TextR("more")}, []string{"utilize"}, []int{3}},
		{"after a reference", []model.Run{model.TextR("Say"), entity("1", "&nbsp;"), model.TextR("utilize it")}, []string{"utilize"}, []int{3}},
		{"between references", []model.Run{model.TextR("a"), entity("1", "&mdash;"), model.TextR("utilize"), entity("2", "&#160;"), model.TextR("b")}, []string{"utilize"}, []int{1}},
		{"a reference that spells a letter continues the word", []model.Run{model.TextR("We utiliz"), entity("1", "&#101;"), model.TextR("d it")}, nil, nil},
		{"a reference inside a term is part of it", []model.Run{model.TextR("the caf"), entity("1", "&eacute;"), model.TextR(" menu")}, []string{"caf"}, []int{4}},
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
				got = append(got, text[h.Start:h.End])
				at = append(at, h.Start)
			}
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.at, at)
		})
	}
}

// A hit's run anchor and the span a fix replaces come from the block's runs:
// the word before a reference is a range inside its own text run.
func TestMatchTermRulesInRunsAnchorsInTheTextRun(t *testing.T) {
	runs := []model.Run{model.TextR("and   utilize"), entity("1", "&amp;"), model.TextR("more")}
	text := model.RunsText(runs)
	hits := MatchTermRulesInRuns([]TermRuleSet{{Rules: []TermRule{{Term: "utilize", Replacement: "use"}}}}, text, runs)
	require.Len(t, hits, 1)
	findings := HitsToFindings(hits, text, runs)
	require.Len(t, findings, 1)
	assert.Equal(t, "utilize", findings[0].OriginalText)
	assert.Equal(t, model.RangeAnchorForBytes(runs, 6, 13), findings[0].Position)
}

// Text the runs do not flatten to is searched as given.
func TestTermSearchTextKeepsTextTheRunsDoNotSpell(t *testing.T) {
	runs := []model.Run{model.TextR("a"), entity("1", "&amp;"), model.TextR("b")}
	search, toText := TermSearchText("something else", runs)
	assert.Equal(t, "something else", search)
	assert.Nil(t, toText)

	search, toText = TermSearchText("ab", runs)
	assert.Equal(t, "a&b", search)
	require.NotNil(t, toText)
	assert.Equal(t, []int{0, 1, 1, 2}, []int{toText(0), toText(1), toText(2), toText(3)})
}
