package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// sig renders a run sequence compactly: text verbatim, <id>/</id> for a paired
// code, {id} for a placeholder, so tests can assert on code structure.
func sig(runs []Run) string {
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

// boldSpan builds "Hello <b>ugly</b> world" with a deletable bold pair (id=1).
func boldSpan() []Run {
	del := &RunConstraints{Deletable: true, Cloneable: true, Reorderable: true}
	return []Run{
		{Text: &TextRun{Text: "Hello "}},
		{PcOpen: &PcOpenRun{ID: "1", Type: "fmt:bold", Constraints: del}},
		{Text: &TextRun{Text: "ugly"}},
		{PcClose: &PcCloseRun{ID: "1", Type: "fmt:bold"}},
		{Text: &TextRun{Text: " world"}},
	}
}

func TestApplyTextEdits(t *testing.T) {
	tests := []struct {
		name  string
		runs  []Run
		edits []TextEdit
		want  string
	}{
		{
			// Edit wholly inside the span keeps the span around the new text.
			name:  "edit inside span keeps span",
			runs:  boldSpan(),
			edits: []TextEdit{{Start: 6, End: 10, Replacement: "pretty"}},
			want:  "Hello <1>pretty</1> world",
		},
		{
			// Replacing the span's entire content keeps the span around it.
			name:  "replace whole span content keeps span",
			runs:  boldSpan(),
			edits: []TextEdit{{Start: 6, End: 10, Replacement: "x"}},
			want:  "Hello <1>x</1> world",
		},
		{
			// Edit crossing the opening boundary: span survives, repositioned,
			// excluding the replacement text; stays balanced.
			name:  "edit crossing opening boundary keeps balance",
			runs:  boldSpan(),
			edits: []TextEdit{{Start: 3, End: 8, Replacement: "X"}},
			want:  "HelX<1>ly</1> world",
		},
		{
			// The whole bold span is consumed; deletable, so it collapses — no
			// empty <b></b>.
			name:  "emptied deletable span collapses",
			runs:  boldSpan(),
			edits: []TextEdit{{Start: 0, End: 16, Replacement: "Hi"}},
			want:  "Hi",
		},
		{
			// Same edit, but the span is non-deletable: it is kept (empty) so it
			// is never silently dropped.
			name: "emptied non-deletable span kept",
			runs: func() []Run {
				r := boldSpan()
				r[1].PcOpen.Constraints = &RunConstraints{Deletable: false}
				return r
			}(),
			edits: []TextEdit{{Start: 0, End: 16, Replacement: "Hi"}},
			want:  "Hi<1></1>",
		},
		{
			// A non-deletable placeholder survives an edit that deletes the text
			// around it.
			name: "non-deletable placeholder survives",
			runs: []Run{
				{Text: &TextRun{Text: "Hello "}},
				{Ph: &PlaceholderRun{ID: "1", Type: "struct:break",
					Constraints: &RunConstraints{Deletable: false}}},
				{Text: &TextRun{Text: "world"}},
			},
			edits: []TextEdit{{Start: 0, End: 11, Replacement: "Hi"}},
			want:  "{1}Hi",
		},
		{
			// A subblock reference is never deletable, even with no constraints.
			name: "sub reference survives",
			runs: []Run{
				{Text: &TextRun{Text: "see "}},
				{Sub: &SubRun{ID: "1", Ref: "b2"}},
				{Text: &TextRun{Text: " here"}},
			},
			edits: []TextEdit{{Start: 0, End: 9, Replacement: "X"}},
			want:  "[1]X",
		},
		{
			// Constraints absent on the run: deletability comes from the
			// vocabulary. fmt:bold is deletable, so an emptied span collapses.
			name: "vocabulary resolves deletability when unset",
			runs: []Run{
				{Text: &TextRun{Text: "a"}},
				{PcOpen: &PcOpenRun{ID: "1", Type: "fmt:bold"}},
				{Text: &TextRun{Text: "b"}},
				{PcClose: &PcCloseRun{ID: "1", Type: "fmt:bold"}},
				{Text: &TextRun{Text: "c"}},
			},
			edits: []TextEdit{{Start: 0, End: 3, Replacement: "Z"}},
			want:  "Z",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyTextEdits(tc.runs, tc.edits)
			assert.Equal(t, tc.want, sig(got))
		})
	}
}

func TestApplyTextEditsNoEdits(t *testing.T) {
	runs := boldSpan()
	got := ApplyTextEdits(runs, nil)
	assert.Equal(t, "Hello <1>ugly</1> world", sig(got))
}

func TestApplyTextEditsGlobal(t *testing.T) {
	// Two edits on either side of and inside the span; span survives.
	got := ApplyTextEdits(boldSpan(), []TextEdit{
		{Start: 4, End: 5, Replacement: "0"},   // the 'o' in Hello
		{Start: 12, End: 13, Replacement: "0"}, // the 'o' in world
	})
	assert.Equal(t, "Hell0 <1>ugly</1> w0rld", sig(got))
}

func TestHasStructuredRuns(t *testing.T) {
	assert.False(t, HasStructuredRuns(boldSpan()))
	assert.False(t, HasStructuredRuns([]Run{{Ph: &PlaceholderRun{ID: "1"}}}))
	assert.True(t, HasStructuredRuns([]Run{
		{Plural: &PluralRun{Pivot: "n", Forms: map[PluralForm][]Run{
			PluralOther: {{Text: &TextRun{Text: "x"}}},
		}}},
	}))
	assert.True(t, HasStructuredRuns([]Run{{Select: &SelectRun{Pivot: "g"}}}))
}

func TestRunDeletable(t *testing.T) {
	// Explicit constraints win.
	assert.True(t, runDeletable(Run{Ph: &PlaceholderRun{Type: "x", Constraints: &RunConstraints{Deletable: true}}}))
	assert.False(t, runDeletable(Run{Ph: &PlaceholderRun{Type: "x", Constraints: &RunConstraints{Deletable: false}}}))
	// Falls back to the vocabulary by type.
	assert.True(t, runDeletable(Run{PcOpen: &PcOpenRun{Type: "fmt:bold"}}))
	assert.False(t, runDeletable(Run{Ph: &PlaceholderRun{Type: "struct:break"}}))
	// Sub references and text are never deletable.
	assert.False(t, runDeletable(Run{Sub: &SubRun{ID: "1", Ref: "b"}}))
	assert.False(t, runDeletable(Run{Text: &TextRun{Text: "t"}}))
}
