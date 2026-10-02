package format

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

// braceSyntax is a toy format whose markup is a {…} expression, escaped with a
// backslash, and whose text inside a fmt:code pair is literal.
var braceSyntax = AddedSyntax{
	Find: func(s string, i int) (int, int, bool) {
		if s[i] != '{' {
			return 0, 0, false
		}
		if j := strings.IndexByte(s[i:], '}'); j >= 0 {
			return i, i + j + 1, true
		}
		return i, len(s), true
	},
	Escape: func(s string, i int) (int, string) { return 1, `\{` },
	InCode: func(r model.Run) (bool, bool) {
		switch {
		case r.PcOpen != nil:
			return r.PcOpen.Type == "fmt:code", false
		case r.PcClose != nil:
			return false, r.PcClose.Type == "fmt:code"
		}
		return false, false
	},
}

func textRun(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

func TestRenderEditedRuns(t *testing.T) {
	code := []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:code", Data: "`"}},
		textRun("{literal}"),
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:code", Data: "`"}},
	}
	tests := []struct {
		name string
		read []model.Run
		runs []model.Run
		want string
	}{
		{
			name: "markup the edit adds is escaped",
			read: []model.Run{textRun("Hello")},
			runs: []model.Run{textRun("Hello {evil}")},
			want: `Hello \{evil}`,
		},
		{
			name: "markup the block held, moved, is kept",
			read: []model.Run{textRun("Hi {name}, bye")},
			runs: []model.Run{textRun("{name}: hello")},
			want: "{name}: hello",
		},
		{
			name: "each piece the block held accounts for one",
			read: []model.Run{textRun("Hi {name}")},
			runs: []model.Run{textRun("{name} and {name}")},
			want: `{name} and \{name}`,
		},
		{
			name: "markup spelled differently is new",
			read: []model.Run{textRun("Hi {name}")},
			runs: []model.Run{textRun("Hi {name.x}")},
			want: `Hi \{name.x}`,
		},
		{
			name: "markup inside a code pair is literal",
			read: []model.Run{textRun("x")},
			runs: append([]model.Run{textRun("y ")}, code...),
			want: "y `{literal}`",
		},
		{
			name: "markup in an inline code's data is the code's",
			read: []model.Run{{Ph: &model.PlaceholderRun{ID: "1", Data: "{held}"}}},
			runs: []model.Run{{Ph: &model.PlaceholderRun{ID: "1", Data: "{held}"}}, textRun(" {held}")},
			want: `{held} \{held}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, RenderEditedRuns(tc.runs, tc.read, braceSyntax))
		})
	}
}
