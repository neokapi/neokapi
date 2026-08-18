package check_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// A hygiene rule reports whitespace an author left behind. A block's last line
// break is frequently not that: a YAML block scalar keeps exactly one by clip
// chomping, which is what `|` and `>` mean, and `|-` / `>-` is how an author
// asks for none. Graded as trailing whitespace, the finding is about the
// scalar's style rather than anything written, and the author cannot correct it
// without changing what the document means.
//
// The distinction is a predicate of its own rather than a change to
// [check.TrailingWhitespace], because a rule comparing two sides wants the whole
// edge: a terminator both sides carry cancels, and a target that dropped one
// differs from its source.

func TestStrayTrailingWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "a terminating line break is the content's line ending",
			text: "A sentence in a literal block.\n",
			want: "",
		},
		{
			name: "a terminating break after a wrapped folded scalar",
			text: "A sentence in a folded block wrapped over two lines.\n",
			want: "",
		},
		{
			name: "a break inside the content is not the terminator",
			text: "First line\nsecond line\n",
			want: "",
		},
		{
			name: "a second break is a blank line the author added",
			text: "A sentence.\n\n",
			want: "\n",
		},
		{
			name: "spaces before the terminator are still stray",
			text: "A sentence.  \n",
			want: "  ",
		},
		{
			name: "a tab before the terminator is still stray",
			text: "A sentence.\t\n",
			want: "\t",
		},
		{
			name: "a space after the terminator is still stray",
			text: "A sentence.\n ",
			want: "\n ",
		},
		{
			name: "trailing spaces with no break at all",
			text: "A sentence.  ",
			want: "  ",
		},
		{
			name: "a CRLF terminator is one line ending",
			text: "A sentence.\r\n",
			want: "",
		},
		{
			name: "content with no trailing whitespace",
			text: "A plain scalar sentence.",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, check.StrayTrailingWhitespace(tt.text))
		})
	}
}

// TestContentLint_ATerminatingBreakIsNotADefect is the rule reading that
// predicate: the reproduction in the issue, four scalars of the same sentence,
// as the runs each style yields.
func TestContentLint_ATerminatingBreakIsNotADefect(t *testing.T) {
	tests := []struct {
		name    string
		runs    []model.Run
		want    []string
		wantNot []string
	}{
		{
			name:    "a literal block scalar under clip chomping",
			runs:    []model.Run{hygTx("A sentence in a literal block.\n")},
			wantNot: []string{"trailing-whitespace", "empty"},
		},
		{
			name:    "a folded block scalar under clip chomping",
			runs:    []model.Run{hygTx("A sentence in a folded block wrapped over two lines.\n")},
			wantNot: []string{"trailing-whitespace"},
		},
		{
			name:    "a strip-chomped scalar carries no break",
			runs:    []model.Run{hygTx("A sentence with strip chomping.")},
			wantNot: []string{"trailing-whitespace"},
		},
		{
			name:    "a plain scalar",
			runs:    []model.Run{hygTx("A plain scalar sentence.")},
			wantNot: []string{"trailing-whitespace"},
		},
		{
			name: "keep chomping preserves a blank line, which is whitespace the author kept",
			runs: []model.Run{hygTx("A sentence with keep chomping.\n\n")},
			want: []string{"trailing-whitespace"},
		},
		{
			name: "spaces before the terminator are the defect the rule is about",
			runs: []model.Run{hygTx("A sentence with a stray space.  \n")},
			want: []string{"trailing-whitespace"},
		},
		{
			name: "trailing spaces with no terminator still report",
			runs: []model.Run{hygTx("A sentence with a stray space.  ")},
			want: []string{"trailing-whitespace"},
		},
		{
			name:    "the terminator does not hide the content's leading edge",
			runs:    []model.Run{hygTx("  Indented content.\n")},
			want:    []string{"leading-whitespace"},
			wantNot: []string{"trailing-whitespace"},
		},
		{
			name: "an inline code at the trailing edge is content, and the break after it is the terminator",
			runs: []model.Run{
				hygTx("Run "), codeSpanOpen("1"), hygTx("kapi up"), codeSpanClose("1"), hygTx("\n"),
			},
			wantNot: []string{"trailing-whitespace"},
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
