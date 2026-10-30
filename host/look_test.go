package host

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workspace"
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

// lookStamp is the stamp the suggestions pass left for the checkout at root
// of the project suggestProject wrote under name, and whether it left one.
func lookStamp(t *testing.T, app *App, name, root string) (workspace.ContextImportStamp, bool) {
	t.Helper()
	ws, err := app.Workspace(t.Context())
	require.NoError(t, err)
	stamps, err := ws.ContextImports(t.Context(), workspace.ProjectKey(projectIDFor(name)), normalizedCheckout(root))
	require.NoError(t, err)
	for _, s := range stamps {
		if s.Path == suggestPassStampPath {
			return s, true
		}
	}
	return workspace.ContextImportStamp{}, false
}

// kapiRecords counts the operations the suggestions pass recorded for the
// project at root.
func kapiRecords(t *testing.T, app *App, root string) int {
	t.Helper()
	log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	n := 0
	for _, op := range log.Operations {
		if op.Actor == SuggestActor {
			n++
		}
	}
	return n
}

// lookMap runs the Map over the project at root.
func lookMap(t *testing.T, app *App, root string) *LookProject {
	t.Helper()
	m, err := app.LookProject(lookCommand(t.Context(), recipeOf(root)))
	require.NoError(t, err)
	return m
}

// TestLookProject_FirstLookFilesWhatKapiSees: the first look at a new project
// runs the suggestions pass, so the Map counts what the files do without a
// model, and a second look runs no pass.
func TestLookProject_FirstLookFilesWhatKapiSees(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := suggestProject(t, "firstlook", "", map[string]string{
		"docs/a.md": "Send an email.\n\nReply by email.\n\nOr send an e-mail.\n",
	})

	m := lookMap(t, app, root)
	require.Equal(t, 1, m.Suggests)
	require.Len(t, m.Suggestions, 1)
	assert.Equal(t, "Name", m.Suggestions[0].Kind)
	assert.Equal(t, "email, not e-mail, e mail", m.Suggestions[0].Text)
	assert.Equal(t, "kapi rules", m.Next)
	var text bytes.Buffer
	require.NoError(t, m.FormatText(&text))
	assert.Contains(t, text.String(), "kapi suggests 1")
	assert.Contains(t, text.String(), "What your files do\n  Name  email, not e-mail, e mail  2 of 3 uses\n")

	first, ok := lookStamp(t, app, "firstlook", root)
	require.True(t, ok, "the pass stamps the checkout")
	filed := kapiRecords(t, app, root)
	require.Positive(t, filed)

	// A person drops the suggestion, so nothing answers; the stamp says the
	// pass has run.
	_, err := app.DecideContextReview(t.Context(), ContextReviewRequest{Actor: person, Project: recipeOf(root), Drop: []string{m.Suggestions[0].ID}})
	require.NoError(t, err)
	assert.Zero(t, lookMap(t, app, root).Suggests)
	second, ok := lookStamp(t, app, "firstlook", root)
	require.True(t, ok)
	assert.Equal(t, first.At, second.At, "the second look runs no pass")
	assert.Equal(t, filed, kapiRecords(t, app, root))
}

// TestLookProject_EmptyPassRunsOnce: a pass that finds nothing records no
// operation, and its stamp keeps the next look from running it again.
func TestLookProject_EmptyPassRunsOnce(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := suggestProject(t, "emptypass", "", map[string]string{"docs/a.md": "# Guide\n\nOpen the editor to write.\n"})

	assert.Zero(t, lookMap(t, app, root).Suggests)
	assert.Zero(t, kapiRecords(t, app, root))
	first, ok := lookStamp(t, app, "emptypass", root)
	require.True(t, ok, "an empty pass stamps the checkout")

	lookMap(t, app, root)
	second, ok := lookStamp(t, app, "emptypass", root)
	require.True(t, ok)
	assert.Equal(t, first.At, second.At, "the second look runs no pass")
}

// TestLookProject_NoBlocksNoStamp: a project with no content has given the
// pass nothing to read, so the look after files are added still runs it.
func TestLookProject_NoBlocksNoStamp(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := suggestProject(t, "noblocks", "", nil)

	assert.NotNil(t, lookMap(t, app, root).Source)
	_, ok := lookStamp(t, app, "noblocks", root)
	assert.False(t, ok)
}

// TestLookProject_KeptRuleRunsNoPass: a project with a rule a person kept is
// past its first look, so the look runs no pass over it.
func TestLookProject_KeptRuleRunsNoPass(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := suggestProject(t, "keptrule", "", map[string]string{
		"docs/a.md": "Send an email.\n\nReply by email.\n\nOr send an e-mail.\n",
	})
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: agentIn("s1"), Project: recipeOf(root), Term: "email", InsteadOf: []string{"e-mail"},
		Evidence: []contextop.Evidence{{Path: "docs/a.md"}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: op.ID})
	require.NoError(t, err)

	assert.Zero(t, lookMap(t, app, root).Suggests)
	_, ok := lookStamp(t, app, "keptrule", root)
	assert.False(t, ok, "no pass ran")
	assert.Zero(t, kapiRecords(t, app, root))
}
