package mdx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An attribute value can be copy the reader sees — a tab's label, an image's
// alt text — and it used to ride the skeleton, so no locale could change it.
//
// The scoping is the table's: HTML and ARIA attributes count anywhere, and the
// component-prop conventions only on PascalCase components, because `label` on
// an <option> is a DOM prop rather than a sentence.

func attrTexts(t *testing.T, src string) []string {
	t.Helper()
	blocks, _ := readAll(t, src)
	var out []string
	for _, b := range blocks {
		out = append(out, b.SourceText())
	}
	return out
}

func TestTranslatableAttributeValuesAreOffered(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // "" means the value must NOT be offered
	}{
		{name: "component label", src: "<TabItem label=\"A tab label\">\n\nBody.\n\n</TabItem>\n", want: "A tab label"},
		{name: "img alt", src: "<img alt=\"A described image\" src=\"/x.png\" />\n", want: "A described image"},
		{name: "aria-label", src: "<button aria-label=\"Close the dialog\" />\n", want: "Close the dialog"},
		{name: "single quotes", src: "<TabItem label='A tab label' />\n", want: "A tab label"},

		// Not copy: an identifier, a path, a program.
		{name: "value prop", src: "<TabItem value=\"a\" />\n"},
		{name: "src path", src: "<img src=\"/static/x.png\" />\n"},
		{name: "expression value", src: "<TabItem label={someVar} />\n"},
		// `label` on a lowercase element is a DOM prop, not a sentence.
		{name: "label on a plain element", src: "<option label=\"a\" />\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			texts := attrTexts(t, tc.src)
			joined := strings.Join(texts, "\x00")
			if tc.want == "" {
				assert.NotContains(t, joined, "a\x00")
				for _, got := range texts {
					assert.NotEqual(t, "/static/x.png", got)
					assert.NotEqual(t, "someVar", got)
				}
				return
			}
			assert.Contains(t, texts, tc.want)
		})
	}
}

// The value is a slice of the tag, so the tag's own bytes must still
// reconstruct around it.
func TestAttributeValueKeepsTheRoundTrip(t *testing.T) {
	src := "<TabItem value=\"a\" label=\"A tab label\">\n\nBody prose.\n\n</TabItem>\n"
	blocks, diags := readAll(t, src)

	require.NotEmpty(t, blocks)
	for _, d := range diags {
		assert.NotEqual(t, "structure.markdown-span-opaque", d.Category,
			"surfacing an attribute must not cost the region its rebuild")
	}
}
