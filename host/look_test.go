package host

import (
	"bytes"
	"slices"
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

// lookReaderOf builds a Reader of one block whose text is text, with one
// reporting finding labelled label over each rune range in spans.
func lookReaderOf(t *testing.T, width int, text string, spans [][2]int, labels []string) string {
	t.Helper()
	runs := []model.Run{model.TextR(text)}
	b := model.NewBlock("p1", text)
	b.SetSourceRuns(runs)
	var diags []check.Diagnostic
	for _, sp := range spans {
		anchor := model.RangeAnchor(runs, sp[0], sp[1])
		diags = append(diags, check.Diagnostic{Check: "hygiene", Message: "m", Location: check.Location{Block: "p1", Anchor: &anchor}})
	}
	r := &LookReader{Path: "x.md", Declared: true, width: width}
	r.place([]*model.Block{b}, diags)
	for i := range r.Blocks[0].Findings {
		r.Blocks[0].Findings[i].Label = labels[i]
	}
	var buf bytes.Buffer
	require.NoError(t, r.FormatText(&buf))
	return buf.String()
}

// TestLookReader_FindingAtAMidWordBreak: a word wider than the line breaks
// mid-word, and a finding that starts exactly at the break is labelled once,
// under the line it starts on, while a finding earlier on the first line keeps
// its carets and label there.
func TestLookReader_FindingAtAMidWordBreak(t *testing.T) {
	// The text column is 30 runes wide (width 34 less the "p1  " gutter), so
	// the 40-rune word breaks after rune 30.
	text := strings.Repeat("a", 5) + strings.Repeat("b", 5) + strings.Repeat("a", 20) + strings.Repeat("c", 5) + strings.Repeat("a", 5)
	out := lookReaderOf(t, 34, text, [][2]int{{5, 10}, {30, 35}}, []string{"first", "second"})
	lines := strings.Split(out, "\n")

	assert.Equal(t, 1, strings.Count(out, "second"), "the label at the break is drawn once:\n%s", out)
	at := slices.Index(lines, "p1  "+text[:30])
	require.Positive(t, at, out)
	assert.Equal(t, "    "+strings.Repeat(" ", 5)+"^^^^^", lines[at+1])
	assert.Equal(t, "    "+strings.Repeat(" ", 5)+"first", lines[at+2])
	assert.Equal(t, "    "+text[30:], lines[at+3])
	assert.Equal(t, "    ^^^^^", lines[at+4])
	assert.Equal(t, "    second", lines[at+5])
}

// TestLookReader_LongLabelWrapsUnderItsCaret: a label longer than the room
// right of its caret wraps onto the rows below at the caret's column, rather
// than moving left of it.
func TestLookReader_LongLabelWrapsUnderItsCaret(t *testing.T) {
	text := "Short words and then a finding near the end"
	start := strings.Index(text, "end")
	out := lookReaderOf(t, 50, text, [][2]int{{start, start + 3}}, []string{"Word: a label much longer than the room that is left"})
	lines := strings.Split(out, "\n")
	at := slices.Index(lines, "p1  "+text)
	require.Positive(t, at, out)
	col := 4 + start
	assert.Equal(t, strings.Repeat(" ", col)+"^^^", lines[at+1])
	var rows []string
	for _, l := range lines[at+2:] {
		if strings.TrimSpace(l) == "" {
			break
		}
		assert.True(t, strings.HasPrefix(l, strings.Repeat(" ", col)) && l[col] != ' ', "each row of the label starts at the caret: %q\n%s", l, out)
		rows = append(rows, strings.TrimSpace(l))
	}
	assert.Greater(t, len(rows), 1, "the label wraps")
	assert.Equal(t, "Word: a label much longer than the room that is left", strings.Join(rows, " "))
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
