package model

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

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

// flagged renders runs with do-not-translate text in brackets, so a test sees
// both the codes and which text carries the flag.
func flagged(runs []Run) string {
	var b strings.Builder
	for _, r := range runs {
		if r.Text != nil && r.Text.NoTranslate {
			fmt.Fprintf(&b, "[%s]", r.Text.Text)
			continue
		}
		b.WriteString(sig([]Run{r}))
	}
	return b.String()
}

// kapiCheckSpan is "Run `kapi check` in CI." as the Markdown reader builds it: the
// code span's content is marked do-not-translate.
func kapiCheckSpan() []Run {
	return []Run{
		{Text: &TextRun{Text: "Run "}},
		{PcOpen: &PcOpenRun{ID: "1", Type: "fmt:code", Data: "`"}},
		{Text: &TextRun{Text: "kapi check", NoTranslate: true}},
		{PcClose: &PcCloseRun{ID: "1", Type: "fmt:code", Data: "`"}},
		{Text: &TextRun{Text: " in CI."}},
	}
}

func TestApplyTextEdits_KeepsNoTranslate(t *testing.T) {
	tests := []struct {
		name  string
		runs  []Run
		edits []TextEdit
		want  string
	}{
		{"an edit outside the flagged text leaves it flagged", kapiCheckSpan(),
			[]TextEdit{{Start: 0, End: 3, Replacement: "Execute"}}, "Execute <1>[kapi check]</1> in CI."},
		{"text replacing only flagged text is flagged", kapiCheckSpan(),
			[]TextEdit{{Start: 9, End: 14, Replacement: "verify"}}, "Run <1>[kapi verify]</1> in CI."},
		{"text replacing flagged and plain text is plain", kapiCheckSpan(),
			[]TextEdit{{Start: 9, End: 17, Replacement: "x"}}, "Run <1>[kapi ]</1>x CI."},
		{"an insertion inside flagged text is flagged", kapiCheckSpan(),
			[]TextEdit{{Start: 8, End: 8, Replacement: "-cli"}}, "Run <1>[kapi-cli check]</1> in CI."},
		{"an insertion at the edge of flagged text is plain", kapiCheckSpan(),
			[]TextEdit{{Start: 14, End: 14, Replacement: " --ship"}}, "Run <1>[kapi check] --ship</1> in CI."},
		{"a flag boundary inside one run splits it",
			[]Run{{Text: &TextRun{Text: "use "}}, {Text: &TextRun{Text: "npm", NoTranslate: true}}, {Text: &TextRun{Text: " now"}}},
			[]TextEdit{{Start: 0, End: 3, Replacement: "Use"}}, "Use [npm] now"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, flagged(ApplyTextEdits(tc.runs, tc.edits)))
		})
	}
}

// Offsets count code points, so an edit after non-ASCII text lands where a
// reader counts it.
func TestApplyTextEdits_CountsCodePoints(t *testing.T) {
	runs := []Run{
		{Text: &TextRun{Text: "Blåbær "}},
		{PcOpen: &PcOpenRun{ID: "1", Type: "fmt:bold"}},
		{Text: &TextRun{Text: "syltetøy"}},
		{PcClose: &PcCloseRun{ID: "1", Type: "fmt:bold"}},
	}
	got := ApplyTextEdits(runs, []TextEdit{{Start: 7, End: 15, Replacement: "jam"}})
	assert.Equal(t, "Blåbær <1>jam</1>", sig(got))
}

// A plural is a zero-width code the edit keeps, so text around it can be
// edited by position in the sequence's own text.
func TestApplyTextEdits_KeepsAPluralInPlace(t *testing.T) {
	plural := Run{Plural: &PluralRun{Pivot: "n", Forms: map[PluralForm][]Run{PluralOther: {{Text: &TextRun{Text: "items"}}}}}}
	runs := []Run{{Text: &TextRun{Text: "You have "}}, plural, {Text: &TextRun{Text: " now"}}}
	assert.Equal(t, "You have  now", SequenceText(runs))
	got := ApplyTextEdits(runs, []TextEdit{{Start: 4, End: 8, Replacement: "own"}})
	assert.Len(t, got, 3)
	assert.Equal(t, "You own ", got[0].Text.Text)
	assert.Same(t, plural.Plural, got[1].Plural)
	assert.Equal(t, " now", got[2].Text.Text)
}

// ApplyTextEdits places each code with a lookup into the edits rather than a
// walk over them, so a long sequence with a code and an edit per word is
// rewritten in time linear in its length.
func TestApplyTextEdits_ManyCodesAndEditsScale(t *testing.T) {
	const words = 60_000
	runs := make([]Run, 0, 2*words)
	var edits []TextEdit
	at := 0
	for i := range words {
		runs = append(runs, TextR("word "), PhR(PlaceholderRun{ID: strconv.Itoa(i), Type: "code:variable"}))
		edits = append(edits, TextEdit{Start: at, End: at + 4, Replacement: "WORD"})
		at += 5
	}

	start := time.Now()
	out := ApplyTextEdits(runs, edits)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Equal(t, strings.Repeat("WORD ", words), RunsText(out))
	assert.Len(t, out, 2*words, "every code is kept after its word")
}
