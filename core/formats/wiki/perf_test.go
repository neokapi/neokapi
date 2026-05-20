package wiki

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// markerDenseParagraph builds a paragraph dense with a single inline opener
// (`<`) that never forms a valid construct (a bare `<` is not a recognised
// `<sub>`/`<sup>`/`<del>`/`<nowiki>` tag). Every candidate therefore fails its
// match cheaply and the scan advances by one byte at a time.
//
// This isolates the #608 N2 quadratic: the old splitDokuWikiInlineRuns
// re-Indexed ALL nine markers from `scan` to end-of-paragraph on EVERY
// iteration. The eight markers that never appear in the text were re-searched
// every loop, each scanning the whole remaining paragraph for an absent
// needle — O(n) per iteration over O(n) iterations = O(n^2). The cached
// next-occurrence offsets record "absent" (-1) once and never re-search those
// markers, making the scan linear. Letters between openers keep bytes
// distinct so a substring search cannot short-circuit.
func markerDenseParagraph(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("< ")
		b.WriteByte(byte('a' + i%26))
		b.WriteByte(' ')
	}
	return b.String()
}

// segmentWithText returns a model.Block carrying a single source segment whose
// runs are one TextRun holding `text`, mirroring what the reader produces for
// a plain paragraph before inline tokenisation runs.
func segmentWithText(text string) *model.Block {
	b := &model.Block{}
	seg := &model.Segment{}
	seg.SetRuns([]model.Run{{Text: &model.TextRun{Text: text}}})
	b.Source = []*model.Segment{seg}
	return b
}

// concatRunText reconstructs the full text from a run slice so we can assert
// the inline tokeniser is byte-neutral (the concatenation of all run payloads
// equals the original paragraph text).
func concatRunText(runs []model.Run) string {
	var sb strings.Builder
	for _, r := range runs {
		switch {
		case r.Text != nil:
			sb.WriteString(r.Text.Text)
		case r.PcOpen != nil:
			sb.WriteString(r.PcOpen.Data)
		case r.PcClose != nil:
			sb.WriteString(r.PcClose.Data)
		case r.Ph != nil:
			sb.WriteString(r.Ph.Data)
		}
	}
	return sb.String()
}

// TestSplitDokuWikiInlineRuns_MarkerDenseByteNeutral verifies the N2 fix is
// byte-neutral on a marker-dense paragraph: whether or not any construct
// matched, the concatenation of the emitted runs reproduces the input text
// exactly. With no valid pairs, the tokeniser must leave the text unchanged.
func TestSplitDokuWikiInlineRuns_MarkerDenseByteNeutral(t *testing.T) {
	text := markerDenseParagraph(4000)
	runs, changed := splitDokuWikiInlineRuns(text)
	if changed {
		// Even if it decided to split, the bytes must round-trip exactly.
		assert.Equal(t, text, concatRunText(runs), "inline run split must be byte-neutral")
	} else {
		assert.Nil(t, runs, "no change should return nil runs")
	}
}

// TestTokenizeDokuWikiInlineCodes_MarkerDense exercises the public-facing
// entry point on a marker-dense paragraph block and asserts the text content
// survives byte-for-byte.
func TestTokenizeDokuWikiInlineCodes_MarkerDense(t *testing.T) {
	text := markerDenseParagraph(4000)
	block := segmentWithText(text)
	tokenizeDokuWikiInlineCodes(block)
	require.Len(t, block.Source, 1)
	assert.Equal(t, text, concatRunText(block.Source[0].Runs),
		"tokeniser must preserve the paragraph bytes exactly")
}

// TestSplitDokuWikiInlineRuns_DensePlusRealPairs verifies the cached-offset
// scan stays byte-neutral across a range of marker-dense inputs interleaved
// with genuine constructs. Byte-neutrality (runs concatenate back to the
// input) is the contract the perf fix must preserve; the existing reader test
// suite covers which constructs are recognised.
func TestSplitDokuWikiInlineRuns_DensePlusRealPairs(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"dense_only", markerDenseParagraph(200)},
		{"bold_then_dense", "plain **bold** then ** ** ** __ __ tail"},
		{"bold_and_italic_then_dense", "**bold** and //italic// then // // //"},
		{"dense_italic_openers", strings.Repeat("// ", 500)},
		{"leading_dense_then_bold", strings.Repeat("** ", 100) + "**real**"},
		{"links_images_macros", "[[link]] {{img.png}} ~~NOTOC~~ %% raw %% ** ** //"},
		{"unmatched_html_openers", strings.Repeat("<x ", 500)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runs, changed := splitDokuWikiInlineRuns(tc.text)
			if !changed {
				assert.Nil(t, runs, "no change should return nil runs")
				return
			}
			assert.Equal(t, tc.text, concatRunText(runs), "must be byte-neutral")
		})
	}
}

// BenchmarkSplitDokuWikiInlineRuns_MarkerDense benchmarks the inline tokeniser
// on a marker-dense paragraph — the input class that was O(n^2) before #608.
func BenchmarkSplitDokuWikiInlineRuns_MarkerDense(b *testing.B) {
	text := markerDenseParagraph(4000)
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for b.Loop() {
		_, _ = splitDokuWikiInlineRuns(text)
	}
}
