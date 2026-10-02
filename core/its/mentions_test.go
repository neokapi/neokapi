package its

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rule reads an attribute when its selector selects or tests it, or one of
// its pointers names it; a reader takes such an attribute as an instruction.
func TestRuleSetMentionsAttribute(t *testing.T) {
	cases := []struct {
		name, rule string
		reads      []string
		ignores    []string
	}{
		{"a predicate", `<its:translateRule selector="//b[@role='skip']" translate="no"/>`, []string{"role"}, []string{"class"}},
		{"an existence predicate", `<its:withinTextRule selector="//b[@inline]" withinText="yes"/>`, []string{"inline"}, []string{"role"}},
		{"an attribute selector", `<its:translateRule selector="//img/@alt" translate="yes"/>`, []string{"alt"}, []string{"src"}},
		{"every attribute", `<its:translateRule selector="//*/@*" translate="no"/>`, []string{"href", "class"}, nil},
		{"a pointer", `<its:locNoteRule selector="//b" locNotePointer="@note"/>`, []string{"note"}, []string{"href"}},
		{"an element selector", `<its:translateRule selector="//head" translate="no"/>`, nil, []string{"href", "head"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs, _, err := ExtractRules([]byte(`<doc xmlns:its="http://www.w3.org/2005/11/its"><its:rules version="2.0">` + tc.rule + `</its:rules></doc>`))
			require.NoError(t, err)
			require.Len(t, rs.Rules, 1)
			for _, name := range tc.reads {
				assert.True(t, rs.MentionsAttribute(name), "the rule reads %s", name)
				assert.True(t, NewResolver(rs).MentionsAttribute(name))
			}
			for _, name := range tc.ignores {
				assert.False(t, rs.MentionsAttribute(name), "the rule does not read %s", name)
			}
		})
	}
	var none *Resolver
	assert.False(t, none.MentionsAttribute("href"), "no rules read nothing")
}
