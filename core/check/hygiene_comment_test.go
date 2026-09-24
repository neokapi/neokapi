package check

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

func commentSpanTexts(text string) []string {
	var out []string
	for _, s := range commentDoubleSpaceSpans(text) {
		out = append(out, text[s[0]:s[1]])
	}
	return out
}

func TestCommentDoubleSpaces(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want int
	}{
		// The owner's double-space verdicts, as the comment text reads.
		{"TSX-DO122 flag", "Bind it into a ConceptView timeline slot:  slots={{ timeline: (p) => <ConceptTimeline {...p} /> }}", 1},
		{"TSX-DO123 flag", "Bind it into a ConceptView constraints slot:  slots={{ constraints: (p) => <ConstraintsPanel {...p} /> }}", 1},
		{"TSX-DO124 flag", "Read / Write / Glob / mcp tool / etc.  ev.title already prettifies mcp names.", 1},
		{"TSX-DO125 fine: a run of five", "  [Translate ready source only]\n  [Transport only]     — push, don't translate (scope \"none\")", 0},
		{"TSX-DO126 fine: a run of twenty", "  window.kapi.pseudo({ expansion: 30 })       // on, +30% padding\n  window.kapi.pseudo(null)                    // off", 0},
		{"TSX-DO127 fine: aligned with the next line", "layouts:\n  • \"tabs\"  — a tabbed view: [Command] | [Result]   (compact, good in prose)\n  • \"split\" — side by side terminal ⇄ curated result", 0},
		{"MJS-DO161 fine: a run of three", "    alice: { token, name, email },\n    bob:   { token, name, email } }", 0},

		{"two spaces between sentences", "native are absorbed.  Within each block the", 1},
		{"aligned with the line above", "SMTP_HOST  the host\nSMTP_PORT  the port", 0},
		{"inside backticks", "Run `kapi  check` to see it.", 0},
		{"inside double quotes", "The label reads \"a  b\" on screen.", 0},
		{"inside curly quotes", "The label reads “a  b” on screen.", 0},
		{"after a closed quote", "The label reads \"ab\"  and more.", 1},
		{"inside a quote opened on the line above", "The phrase \"content\nmemory\" is found in \"content  memory\" too.", 0},
		{"after a quote the paragraph above left open", "An \"unclosed quote.\n\nA new  paragraph.", 1},
		{"the indent of a line", "First line\n  indented continuation", 0},
		{"trailing spaces", "Ends with two  ", 0},
		{"beside a tab", "tab\t  then", 0},
		{"aligned through a tab", "a\tb  c\nx\ty  z", 0},
		{"one per line", "two  here\nlonger text  there", 2},
		{"must fail: two lines whose runs end at one column align", "two  here\nand  there", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := commentSpanTexts(tc.text)
			assert.Len(t, got, tc.want, "%q", got)
			for _, s := range got {
				assert.Equal(t, "  ", s)
			}
			assert.Equal(t, tc.want > 0, CommentDoubleSpaces(tc.text))
		})
	}

	t.Run("a column after an inline code counts the code as one", func(t *testing.T) {
		code := string(model.ObjectReplacement)
		assert.Empty(t, commentSpanTexts("x"+code+"  wide\nab  wide"), "the runs end at the same column")
	})
}

// A comment block reads the comment rule and every other block the rule for
// prose, which reports any run of two or more spaces between words.
func TestContentLintChoosesTheDoubleSpaceRule(t *testing.T) {
	const aligned = "SMTP_HOST  the host\nSMTP_PORT  the port"
	find := func(b *model.Block) map[string]bool {
		findings, err := runTool(NewContentLintTool())(b)
		assert.NoError(t, err)
		out := map[string]bool{}
		for _, f := range findings {
			out[f.Category] = f.Fails
		}
		return out
	}
	assert.NotContains(t, find(commentCanary("func/Parse", true, aligned)), "double-spaces", "aligned text in a comment is layout")
	prose := find(CanaryBlock(aligned))
	assert.Contains(t, prose, "double-spaces", "must fail: the same text outside a comment keeps the prose rule")
	assert.False(t, prose["double-spaces"], "a double space in prose reports")
	assert.Contains(t, find(commentCanary("func/Parse", true, "A slip  here.")), "double-spaces", "a double space between words in a comment is reported")
}
