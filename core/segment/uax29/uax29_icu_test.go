//go:build cgo

package uax29

import (
	"context"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func text(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

// segText renders the runs covered by a span as plain text, for assertions.
func segText(runs []model.Run, sp model.Span) string {
	return model.RunsText(sp.Range.ExtractRuns(runs))
}

func segTexts(runs []model.Run, spans []model.Span) []string {
	out := make([]string, len(spans))
	for i, sp := range spans {
		out[i] = segText(runs, sp)
	}
	return out
}

func TestUAX29_Registered(t *testing.T) {
	assert.True(t, segment.HasEngine("uax29"), "the cgo build registers the uax29 engine")
	eng, err := segment.NewEngine("uax29", segment.Config{})
	require.NoError(t, err)
	assert.Equal(t, segment.LayerSentence, eng.Layer())
}

func TestUAX29_Segment(t *testing.T) {
	tests := []struct {
		name string
		lang string // cfg.Language; empty => use loc
		loc  model.LocaleID
		in   string
		want []string
	}{
		{
			name: "english three sentences",
			loc:  "en",
			in:   "Hello world. How are you? Fine.",
			// ICU keeps the sentence-final whitespace with the preceding sentence.
			want: []string{"Hello world. ", "How are you? ", "Fine."},
		},
		{
			name: "abbreviation Dr breaks under default UAX-29 rules",
			loc:  "en",
			// Documented behavior: ICU's DEFAULT sentence break iterator
			// implements the plain UAX-29 rules, which have NO abbreviation
			// suppression. A period followed by whitespace is always a
			// sentence boundary, so "Dr." terminates a sentence even when an
			// abbreviation is clearly intended. This is the key difference from
			// the SRX engine, whose rules file lists abbreviations (Dr., Mr.,
			// etc.) as non-breaking exceptions and would keep "Dr. Smith left."
			// as one segment. (ICU can match SRX-like behavior only via a
			// dictionary/filtered break iterator, which the plain ubrk_open
			// sentence iterator does not use.)
			in:   "Dr. Smith left.",
			want: []string{"Dr. ", "Smith left."},
		},
		{
			name: "abbreviation mid-text also breaks (UAX-29, no suppression)",
			loc:  "en",
			in:   "I saw Dr. Smith. He left.",
			want: []string{"I saw Dr. ", "Smith. ", "He left."},
		},
		{
			name: "japanese ideographic full stop",
			loc:  "ja",
			in:   "これはペンです。それは本です。",
			want: []string{"これはペンです。", "それは本です。"},
		},
		{
			name: "non-ascii accented",
			loc:  "fr",
			in:   "Café fermé. Déjà vu.",
			want: []string{"Café fermé. ", "Déjà vu."},
		},
		{
			name: "non-bmp emoji preserves rune alignment",
			loc:  "en",
			// The emoji is a non-BMP code point (surrogate pair in UTF-16);
			// boundaries after it must still map to correct rune offsets so the
			// span text comes out intact.
			in:   "Look 👨‍👩‍👧 here. Done now.",
			want: []string{"Look 👨‍👩‍👧 here. ", "Done now."},
		},
		{
			name: "single sentence no terminator",
			loc:  "en",
			in:   "Just one chunk",
			want: []string{"Just one chunk"},
		},
		{
			name: "language override wins over loc",
			lang: "ja",
			loc:  "en",
			in:   "一つ目。二つ目。",
			want: []string{"一つ目。", "二つ目。"},
		},
		{
			name: "bcp47 hyphen locale",
			loc:  "en-US",
			in:   "First. Second.",
			want: []string{"First. ", "Second."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, err := segment.NewEngine("uax29", segment.Config{Language: tt.lang})
			require.NoError(t, err)

			runs := []model.Run{text(tt.in)}
			spans, err := eng.Segment(context.Background(), runs, tt.loc)
			require.NoError(t, err)
			assert.Equal(t, tt.want, segTexts(runs, spans))
		})
	}
}

func TestUAX29_EmptyInput(t *testing.T) {
	eng, err := segment.NewEngine("uax29", segment.Config{})
	require.NoError(t, err)

	t.Run("nil runs", func(t *testing.T) {
		spans, err := eng.Segment(context.Background(), nil, "en")
		require.NoError(t, err)
		assert.Nil(t, spans)
	})
	t.Run("empty text run", func(t *testing.T) {
		spans, err := eng.Segment(context.Background(), []model.Run{text("")}, "en")
		require.NoError(t, err)
		assert.Nil(t, spans)
	})
}

// TestUAX29_Concurrent exercises the contract that a single engine instance is
// safe under concurrent Segment calls (each opens its own ICU break iterator).
// Run under -race to detect any shared-state misuse.
func TestUAX29_Concurrent(t *testing.T) {
	eng, err := segment.NewEngine("uax29", segment.Config{})
	require.NoError(t, err)

	runs := []model.Run{text("Hello world. How are you? Fine.")}
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			spans, err := eng.Segment(context.Background(), runs, "en")
			//nolint:testifylint // assert, not require: FailNow must not run off the test goroutine
			assert.NoError(t, err)
			assert.Len(t, spans, 3)
		})
	}
	wg.Wait()
}

func TestUAX29_ContextCancelled(t *testing.T) {
	eng, err := segment.NewEngine("uax29", segment.Config{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = eng.Segment(ctx, []model.Run{text("Hello. World.")}, "en")
	assert.ErrorIs(t, err, context.Canceled)
}
