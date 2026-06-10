package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sigRuns renders a run sequence compactly so tests can assert on inline-code
// structure as well as text: text verbatim, <id>/</id> for a paired code,
// {id} for a placeholder, [id] for a subblock reference.
func sigRuns(runs []model.Run) string {
	var b strings.Builder
	for _, r := range runs {
		switch {
		case r.Text != nil:
			b.WriteString(r.Text.Text)
		case r.PcOpen != nil:
			fmt.Fprintf(&b, "<%s>", r.PcOpen.ID)
		case r.PcClose != nil:
			fmt.Fprintf(&b, "</%s>", r.PcClose.ID)
		case r.Ph != nil:
			fmt.Fprintf(&b, "{%s}", r.Ph.ID)
		case r.Sub != nil:
			fmt.Fprintf(&b, "[%s]", r.Sub.ID)
		}
	}
	return b.String()
}

// boldUglyRuns is "Hello <b>ugly</b> world" — the running example: a bold span
// (paired code id=1) wrapping the word "ugly".
func boldUglyRuns() []model.Run {
	return []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "b"}},
		{Text: &model.TextRun{Text: "ugly"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "b"}},
		{Text: &model.TextRun{Text: " world"}},
	}
}

func TestSedApplyRunsPreservesCodes(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		runs    []model.Run
		want    string // sigRuns of the result
		changed bool
	}{
		{
			// Replacing text wholly inside the bold span keeps the span around it.
			name:    "edit inside a bold span keeps the span",
			script:  "s/ugly/pretty/",
			runs:    boldUglyRuns(),
			want:    "Hello <1>pretty</1> world",
			changed: true,
		},
		{
			// The regex sees "Hello ugly world" (codes are invisible), so a phrase
			// spanning the bold boundaries matches. The whole bold span is consumed;
			// since bold is deletable the emptied span collapses — no empty <b></b>.
			name:    "match spanning the whole bold phrase drops the emptied span",
			script:  "s/Hello ugly world/Hi/",
			runs:    boldUglyRuns(),
			want:    "Hi",
			changed: true,
		},
		{
			// A match that crosses one boundary: the open code is interior and is
			// carried over after the replacement; the close code stays put. Codes
			// remain balanced and text outside the match is untouched.
			name:    "match crossing the opening code",
			script:  "s/lo ug/X/",
			runs:    boldUglyRuns(),
			want:    "HelX<1>ly</1> world",
			changed: true,
		},
		{
			// Global edit of a word that appears both outside and inside the span.
			name:    "global edit across and inside the span",
			script:  "s/o/0/g",
			runs:    boldUglyRuns(),
			want:    "Hell0 <1>ugly</1> w0rld",
			changed: true,
		},
		{
			// A placeholder not adjacent to the match is preserved in place.
			name:   "placeholder preserved in place",
			script: "s/world/EARTH/",
			runs: []model.Run{
				{Text: &model.TextRun{Text: "Hello "}},
				{Ph: &model.PlaceholderRun{ID: "1", Type: "icon"}},
				{Text: &model.TextRun{Text: " world"}},
			},
			want:    "Hello {1} EARTH",
			changed: true,
		},
		{
			// Backreferences resolve against the flattened text; the surrounding
			// code is preserved.
			name:    "backreference inside a span",
			script:  `s/(ug)(ly)/\2\1/`,
			runs:    boldUglyRuns(),
			want:    "Hello <1>lyug</1> world",
			changed: true,
		},
		{
			// A non-deletable code (here a line break) survives even when the edit
			// removes all the text around it — unlike a deletable bold span.
			name:   "non-deletable code survives an emptying edit",
			script: "s/Hello world/Hi/",
			runs: []model.Run{
				{Text: &model.TextRun{Text: "Hello "}},
				{Ph: &model.PlaceholderRun{ID: "1", Type: "struct:break",
					Constraints: &model.RunConstraints{Deletable: false}}},
				{Text: &model.TextRun{Text: "world"}},
			},
			want:    "{1}Hi",
			changed: true,
		},
		{
			// A deletable standalone code sitting in deleted text goes with it.
			name:   "deletable code dropped by an emptying edit",
			script: "s/Hello world/Hi/",
			runs: []model.Run{
				{Text: &model.TextRun{Text: "Hello "}},
				{Ph: &model.PlaceholderRun{ID: "1", Type: "fmt:icon",
					Constraints: &model.RunConstraints{Deletable: true}}},
				{Text: &model.TextRun{Text: "world"}},
			},
			want:    "Hi",
			changed: true,
		},
		{
			// No match leaves the runs (and codes) exactly as they were.
			name:    "no match leaves runs untouched",
			script:  "s/zzz/qqq/",
			runs:    boldUglyRuns(),
			want:    "Hello <1>ugly</1> world",
			changed: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := parseSedProgram([]string{tc.script})
			require.NoError(t, err)
			got, changed := prog.applyRuns(tc.runs)
			assert.Equal(t, tc.changed, changed)
			assert.Equal(t, tc.want, sigRuns(got))
		})
	}
}

// TestSedApplyRunsNetNoOp guards against needless re-chunking: a program whose
// net effect leaves the flattened text identical must return the original runs
// unchanged rather than a fragmented rebuild.
func TestSedApplyRunsNetNoOp(t *testing.T) {
	prog, err := parseSedProgram([]string{"s/ugly/bad/", "s/bad/ugly/"})
	require.NoError(t, err)
	runs := boldUglyRuns()
	got, changed := prog.applyRuns(runs)
	assert.False(t, changed)
	assert.Equal(t, "Hello <1>ugly</1> world", sigRuns(got))
}

// TestSedApplyRunsMultiCmd chains substitutions over runs; codes survive each
// stage.
func TestSedApplyRunsMultiCmd(t *testing.T) {
	prog, err := parseSedProgram([]string{"s/Hello/Hi/", "s/world/earth/"})
	require.NoError(t, err)
	got, changed := prog.applyRuns(boldUglyRuns())
	assert.True(t, changed)
	assert.Equal(t, "Hi <1>ugly</1> earth", sigRuns(got))
}

// TestSedToolPreservesCodesSource exercises the wired Transform producer via
// the dispatch applier: editing source keeps inline codes instead of
// flattening the block.
func TestSedToolPreservesCodesSource(t *testing.T) {
	prog, err := parseSedProgram([]string{"s/ugly/pretty/"})
	require.NoError(t, err)
	tl := newSedTool(prog, "", true)

	b := &model.Block{ID: "b1", Translatable: true, Source: boldUglyRuns()}
	_, err = tl.Apply(&model.Part{Type: model.PartBlock, Resource: b})
	require.NoError(t, err)
	assert.Equal(t, "Hello <1>pretty</1> world", sigRuns(b.Source))
}

// TestSedToolPreservesCodesTarget does the same for a target translation.
func TestSedToolPreservesCodesTarget(t *testing.T) {
	prog, err := parseSedProgram([]string{"s/laid/moche/"})
	require.NoError(t, err)
	tl := newSedTool(prog, "fr", false)

	b := &model.Block{ID: "b1", Translatable: true, Source: boldUglyRuns()}
	b.SetTargetRuns("fr", []model.Run{
		{Text: &model.TextRun{Text: "Bonjour "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "b"}},
		{Text: &model.TextRun{Text: "laid"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "b"}},
		{Text: &model.TextRun{Text: " monde"}},
	})

	_, err = tl.Apply(&model.Part{Type: model.PartBlock, Resource: b})
	require.NoError(t, err)
	assert.Equal(t, "Bonjour <1>moche</1> monde", sigRuns(b.TargetRuns("fr")))
}

// TestSedToolStructuredFallback documents that a block whose source carries a
// plural/select run cannot be spliced position-wise, so the tool falls back to
// whole-text replacement (the pre-existing, lossy behaviour) rather than
// corrupting the structure silently.
func TestSedToolStructuredFallback(t *testing.T) {
	prog, err := parseSedProgram([]string{"s/world/EARTH/"})
	require.NoError(t, err)
	tl := newSedTool(prog, "", true)

	b := &model.Block{ID: "b1", Translatable: true, Source: []model.Run{
		{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
			model.PluralOther: {{Text: &model.TextRun{Text: "world"}}},
		}}},
	}}
	_, err = tl.Apply(&model.Part{Type: model.PartBlock, Resource: b})
	require.NoError(t, err)
	assert.Equal(t, "EARTH", b.SourceText())
}

// TestEditDocumentSedHTMLPreservesInlineFormatting is the end-to-end proof over
// a real HTML reader/writer: editing a word inside a <b> span keeps the span.
func TestEditDocumentSedHTMLPreservesInlineFormatting(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	const html = `<!DOCTYPE html><html><body><p>Hello <b>ugly</b> world</p></body></html>`
	require.NoError(t, os.WriteFile(path, []byte(html), 0o644))

	prog, err := parseSedProgram([]string{"s/ugly/pretty/"})
	require.NoError(t, err)
	tl := newSedTool(prog, "", true)

	var buf bytes.Buffer
	require.NoError(t, app.editDocument(context.Background(), path, tl, "", false, "", &buf))
	out := buf.String()
	assert.Contains(t, out, "<b>pretty</b>", "bold span must survive the edit")
	assert.NotContains(t, out, "ugly")
}

// TestEditDocumentSedHTMLMatchesAcrossInlineCode proves the regex matches across
// an inline code end-to-end: "Hello ugly world" matches even though "ugly" is
// bold, because the flattening hides the code from the pattern.
func TestEditDocumentSedHTMLMatchesAcrossInlineCode(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	const html = `<!DOCTYPE html><html><body><p>Hello <b>ugly</b> world</p></body></html>`
	require.NoError(t, os.WriteFile(path, []byte(html), 0o644))

	prog, err := parseSedProgram([]string{"s/Hello ugly world/Hi there/"})
	require.NoError(t, err)
	tl := newSedTool(prog, "", true)

	var buf bytes.Buffer
	require.NoError(t, app.editDocument(context.Background(), path, tl, "", false, "", &buf))
	out := buf.String()
	assert.Contains(t, out, "Hi there", "a phrase spanning the bold boundary must match and replace")
	assert.NotContains(t, out, "ugly")
	// The whole bold span was consumed; bold is deletable, so the emptied span
	// collapses rather than leaving an empty <b></b>.
	assert.NotContains(t, out, "<b>", "emptied deletable span must not survive as an empty tag")
}
