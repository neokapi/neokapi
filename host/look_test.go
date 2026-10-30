package host

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// TestLookReader_CaretsFollowTheWrappedLine: a block wider than the terminal
// wraps at a space, and a finding on the second line is drawn under that line
// at its column there.
func TestLookReader_CaretsFollowTheWrappedLine(t *testing.T) {
	text := "The first part of this block is long enough to wrap, and then the word powerful sits on the next line."
	runs := []model.Run{model.TextR(text)}
	start := strings.Index(text, "powerful")
	anchor := model.RangeAnchor(runs, start, start+len("powerful"))

	r := &LookReader{Path: "docs/guide.md", Declared: true, width: 50}
	b := model.NewBlock("p1", text)
	b.SetSourceRuns(runs)
	r.place([]*model.Block{b}, []check.Diagnostic{{
		Rule: "voice.style", Check: "voice", Fails: true, Message: "Say what it does",
		Location: check.Location{Block: "p1", Anchor: &anchor},
	}})
	require.NotNil(t, r.Blocks[0].Findings[0].Span)

	var buf bytes.Buffer
	require.NoError(t, r.FormatText(&buf))
	lines := strings.Split(buf.String(), "\n")

	at := -1
	for i, l := range lines {
		if strings.Contains(l, "powerful") {
			at = i
		}
	}
	require.Greater(t, at, 2, buf.String())
	assert.NotContains(t, lines[at-1], "powerful", "the word sits on a wrapped line")
	col := strings.Index(lines[at], "powerful")
	assert.Equal(t, strings.Repeat(" ", col)+"^^^^^^^^", lines[at+1], buf.String())
	assert.Equal(t, strings.Repeat(" ", col)+"Tone: Say what it does", lines[at+2])
	for _, l := range lines[:at+3] {
		assert.LessOrEqual(t, len([]rune(l)), 50, "no line of the block is wider than the terminal: %q", l)
	}
}

// TestLookSpan_PlaceholderBeforeTheFinding: a placeholder ahead of the finding
// is drawn in its display form, and the span counts its columns.
func TestLookSpan_PlaceholderBeforeTheFinding(t *testing.T) {
	runs := []model.Run{
		model.TextR("Hello "),
		model.PhR(model.PlaceholderRun{ID: "1", Equiv: "name"}),
		model.TextR(", please login."),
	}
	anchor := model.SpanAnchor(model.RunPos{Run: 2, Offset: 9}, model.RunPos{Run: 2, Offset: 14})
	display := lookDisplayText(runs)
	span := lookSpan(runs, &anchor)
	require.NotNil(t, span)
	assert.Equal(t, "login", string([]rune(display)[span.Start:span.End]), display)
}

func TestWrapRunes(t *testing.T) {
	for _, tt := range []struct {
		text  string
		width int
		want  []string
	}{
		{text: "short", width: 10, want: []string{"short"}},
		{text: "one two three four", width: 9, want: []string{"one two", "three", "four"}},
		{text: "abcdefghij", width: 4, want: []string{"abcd", "efgh", "ij"}},
		{text: "", width: 4, want: []string{""}},
	} {
		text := []rune(tt.text)
		var got []string
		for _, ln := range wrapRunes(text, tt.width) {
			got = append(got, string(text[ln.start:ln.end]))
		}
		assert.Equal(t, tt.want, got, tt.text)
	}
}
