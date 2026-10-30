package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
)

// The reader keeps a bare URL as text, as Okapi does, and a link's or an
// image's destination in the code that closes it. A word rule reads none of
// them: the destinations never reach the text of the runs, and the bare URL
// is read as an address (check.URLSpans). The word written in prose beside
// them is still found, and a code span the reader holds as runs is code to a
// rule scoped to prose.
func TestURLsAndCodeSpansAsAWordRuleReadsThem(t *testing.T) {
	t.Parallel()
	rules := []profile.TermRuleSet{{Rules: []profile.TermRule{
		{Term: "utilize", Replacement: "use"},
		{Term: "leverage", Replacement: "rely on", Scope: profile.ScopeProse},
	}}}
	tests := []struct {
		name  string
		input string
		text  string   // the block's text, as the reader keeps it
		hits  []string // the rules that fire, in order
	}{
		{
			name:  "a bare URL",
			input: "See https://example.com/utilize/setup for the steps.\n",
			text:  "See https://example.com/utilize/setup for the steps.",
		},
		{
			name:  "a bare URL beside a use in prose",
			input: "Utilize https://example.com/leverage/utilize today.\n",
			text:  "Utilize https://example.com/leverage/utilize today.",
			hits:  []string{"utilize"},
		},
		{
			name:  "a link's destination",
			input: "Read [the guide](https://example.com/utilize/guide) first.\n",
			text:  "Read the guide first.",
		},
		{
			name:  "an image's destination",
			input: "See ![the chart](https://example.com/utilize.png) here.\n",
		},
		{
			name:  "a link whose text is a URL",
			input: "Read [https://example.com/utilize](https://example.com/utilize).\n",
			text:  "Read https://example.com/utilize.",
		},
		{
			name:  "a prose rule beside a code span",
			input: "Run `leverage --now`, then leverage it.\n",
			text:  "Run leverage --now, then leverage it.",
			hits:  []string{"leverage"},
		},
		{
			name:  "a rule with no scope inside a code span",
			input: "Run `utilize --now` to start.\n",
			text:  "Run utilize --now to start.",
			hits:  []string{"utilize"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			blocks := readBlocks(t, tt.input)
			require.Len(t, blocks, 1)
			runs := blocks[0].SourceRuns()
			text := model.RunsText(runs)
			if tt.text != "" {
				assert.Equal(t, tt.text, text)
			}
			var got []string
			for _, h := range profile.MatchTermRulesInRuns(rules, text, runs) {
				got = append(got, h.Term)
			}
			assert.Equal(t, tt.hits, got)
		})
	}
}
