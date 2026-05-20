package html_test

import (
	"strconv"
	"strings"
	"testing"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

// buildDivSpanHeavyHTML constructs a large document made of `n` sibling
// non-container block elements (<div>, <section>, <article>) — exactly the
// elements that fall through to remainingContent + forwardScanForBlock-
// Children in the reader (#608, N1). Each block carries a short inline run so
// the classifier exercises its forward scan, and each block's text is unique
// (an incrementing counter) so the formerly O(n) bytes.Index in
// remainingContent cannot short-circuit on an earlier identical block — it
// must scan from byte 0 to the live tokenizer position, which is what made
// the path O(n^2). The structure is byte-stable so it also feeds the
// byte-exact skeleton roundtrip assertion.
func buildDivSpanHeavyHTML(n int) string {
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := range n {
		id := strconv.Itoa(i)
		// Alternate the structural wrappers so several distinct
		// non-container tags hit the fall-through classification path.
		switch i % 3 {
		case 0:
			b.WriteString("<div>Text " + id + " <span>inline " + id + "</span> tail</div>")
		case 1:
			b.WriteString("<section>Text " + id + " <b>bold " + id + "</b> tail</section>")
		default:
			b.WriteString("<article>Text " + id + " <em>emph " + id + "</em> tail</article>")
		}
	}
	b.WriteString("</body></html>")
	return b.String()
}

// TestDivSpanHeavy_ByteExactRoundtrip proves the N1 perf fix is byte-neutral:
// a large div/span/section/article-heavy document round-trips through the
// skeleton store unchanged.
func TestDivSpanHeavy_ByteExactRoundtrip(t *testing.T) {
	input := buildDivSpanHeavyHTML(2000)
	output := roundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "div/span-heavy skeleton roundtrip must be byte-exact")
}

// BenchmarkHTMLDivSpanHeavy benchmarks the reader on a div/span-heavy document
// — the input class that exercised the formerly O(n^2) remainingContent /
// forwardScanForBlockChildren path (#608, N1). Run with increasing -benchtime
// or compare across sizes to confirm linear scaling.
func BenchmarkHTMLDivSpanHeavy(b *testing.B) {
	input := buildDivSpanHeavyHTML(2000)
	ctx := b.Context()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for b.Loop() {
		reader := htmlfmt.NewReader()
		_ = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
		for range reader.Read(ctx) {
		}
		reader.Close()
	}
}
