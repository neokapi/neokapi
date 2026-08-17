package check_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

// A hygiene rule judges prose. The contents of an inline code span are not
// prose: `x  y` documents two spaces, `` `the the` `` quotes a repeated token,
// and a pseudo-translated `▒ ▒ … ▒ ▒` shows a wrapper. The markdown reader
// models a code span as a pcOpen/pcClose pair around plain text runs, so every
// shape rule used to read those bytes as the author's sentence and report a
// defect the author cannot correct without breaking the example.
//
// The exclusion is at the point where prose is selected — the hygiene
// flattening — so it holds for every shape rule at once rather than each rule
// defending itself. Fenced code blocks never needed it: a reader surfaces them
// as non-translatable RoleCode content, and content-lint skips a block that is
// not translatable.

func codeSpanOpen(id string) model.Run {
	return model.Run{PcOpen: &model.PcOpenRun{ID: id, Type: model.TypeInlineCode, SubType: "md:code", Data: "`"}}
}

func codeSpanClose(id string) model.Run {
	return model.Run{PcClose: &model.PcCloseRun{ID: id, Type: model.TypeInlineCode, SubType: "md:code", Data: "`"}}
}

func TestContentLint_ACodeSpanIsNotProse(t *testing.T) {
	tests := []struct {
		name    string
		runs    []model.Run
		want    []string
		wantNot []string
	}{
		{
			// The issue's own reproduction: one line of markdown holding both.
			name: "double space and doubled word inside code spans",
			runs: []model.Run{
				hygTx("Shows "), codeSpanOpen("1"), hygTx("x  y"), codeSpanClose("1"),
				hygTx(" and "), codeSpanOpen("2"), hygTx("the the"), codeSpanClose("2"),
				hygTx(" in prose."),
			},
			wantNot: []string{"double-spaces", "doubled-word", "empty"},
		},
		{
			// web/docs/contribute/architecture/002-content-model.md: a table cell
			// whose whole content is the code span documenting a hard line break.
			name:    "a cell that is nothing but a code span of two spaces",
			runs:    []model.Run{codeSpanOpen("1"), hygTx("  \n"), codeSpanClose("1")},
			wantNot: []string{"double-spaces", "leading-whitespace", "trailing-whitespace", "empty", "control-char"},
		},
		{
			// web/docs/react/configuration.md: prose quoting pseudo-translated
			// output. The shade glyphs repeat because that is what the wrapper does.
			name: "a pseudo-translation wrapper quoted as a code span",
			runs: []model.Run{
				hygTx("renders as "), codeSpanOpen("1"), hygTx("▒ ▒ Utility ▒ ▒"), codeSpanClose("1"),
			},
			wantNot: []string{"doubled-word", "double-spaces"},
		},
		{
			name:    "prose outside the span is still judged",
			runs:    []model.Run{codeSpanOpen("1"), hygTx("kapi up"), codeSpanClose("1"), hygTx(" runs  the the loop")},
			want:    []string{"double-spaces", "doubled-word"},
		},
		{
			name:    "a code span does not glue the words either side of it together",
			runs:    []model.Run{hygTx("the "), codeSpanOpen("1"), hygTx("--memory"), codeSpanClose("1"), hygTx(" the flag")},
			wantNot: []string{"doubled-word"},
		},
		{
			name:    "a bold pair still holds prose",
			runs:    []model.Run{hygPcOpen("1"), hygTx("the the"), hygPcClose("1")},
			want:    []string{"doubled-word"},
		},
		{
			name:    "a block that is only a code span is content, not empty",
			runs:    []model.Run{codeSpanOpen("1"), hygTx("--termstore"), codeSpanClose("1")},
			wantNot: []string{"empty", "leading-whitespace", "trailing-whitespace"},
		},
		{
			name:    "whitespace outside the span is still the content's edge",
			runs:    []model.Run{hygTx(" "), codeSpanOpen("1"), hygTx("x"), codeSpanClose("1")},
			want:    []string{"leading-whitespace"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cats := hygieneCategories(t, tt.runs)
			for _, w := range tt.want {
				assert.Contains(t, cats, w)
			}
			for _, w := range tt.wantNot {
				assert.NotContains(t, cats, w)
			}
		})
	}
}
