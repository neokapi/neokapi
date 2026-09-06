package markdown_test

import (
	"testing"

	"os"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExcludedInlineHTMLRoundTrips is the #2444 reproducer. Everything between
// an excluded element's open and close tag is folded into one opaque
// placeholder, and the reader took each child's bytes from its Lines() span.
// goldmark's BaseInline.Lines() panics by design, so an emphasis, a link, a
// code span or an autolink in there crashed the read. The bytes of an inline
// child now come from the offset resolver.
func TestExcludedInlineHTMLRoundTrips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		text  string // source text of the first block
	}{
		{"emphasis in a script span", "a <script>*b*</script> c\n", "a  c"},
		{"strong emphasis", "a <script>**b**</script> c\n", "a  c"},
		{"link in a style span", "a <style>[x](y)</style> c\n", "a  c"},
		{"link with a title", "a <script>[x](y 't')</script> c\n", "a  c"},
		{"reference link", "a <script>[x][r]</script> c\n\n[r]: y\n", "a  c"},
		{"image", "a <script>![i](s)</script> c\n", "a  c"},
		{"code span", "a <script>`c`</script> b\n", "a  b"},
		{"autolink", "a <script><https://example.com></script> b\n", "a  b"},
		{"strikethrough", "a <script>~~s~~</script> b\n", "a  b"},
		{"nested emphasis", "a <script>*b `c` [d](e)*</script> f\n", "a  f"},
		{"span never closed", "a <script>*b* [c](d)\n", "a "},
		{"emphasis inside math", "a <math>*b*</math> c\n", "a  c"},
		{"nested excluded elements", "a <script>b <style>*c*</style> d</script> e\n", "a  e"},
		{"emphasis across a soft break", "a <script>*b\nc*</script> d\n", "a  d"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocks := assertSkeletonByteExact(t, tc.input)
			assert.Equal(t, tc.text, blocks[0].SourceText())
		})
	}
}

// TestExcludedInlineHTMLKeepsItsMarkupVerbatim pins what the placeholder
// carries: the whole excluded span, tags included, so pseudo-translation and
// the rebuild path both replay bytes nobody may edit.
func TestExcludedInlineHTMLKeepsItsMarkupVerbatim(t *testing.T) {
	t.Parallel()
	blocks := readBlocks(t, "a <script>*b* [c](d)</script> e\n")
	require.Len(t, blocks, 1)

	var opens []*model.PcOpenRun
	for _, r := range blocks[0].Source {
		if r.PcOpen != nil {
			opens = append(opens, r.PcOpen)
		}
	}
	require.Len(t, opens, 1)
	assert.Equal(t, "fmt:html", opens[0].Type)
	assert.Equal(t, "<script>*b* [c](d)</script>", opens[0].Data)
}

// TestExcludedInlineHTMLFixture walks the committed fixture, which is also a
// FuzzReadMarkdown seed.
func TestExcludedInlineHTMLFixture(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/excluded-html-inline.md")
	require.NoError(t, err)
	blocks := assertSkeletonByteExact(t, string(data))
	assert.NotEmpty(t, testutil.BlockTexts(blocks))
}
