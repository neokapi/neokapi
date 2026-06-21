package markdown

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// TestMarkdownInlineTagCoverage guards against projection drift: every
// attribute-free `fmt:`/`struct:` formatting type the shared vocabulary defines
// must have a Markdown delimiter mapping, so a newly added inline type cannot
// silently fall through to plain text on the cross-format export path. Link and
// image types carry attributes and are handled separately in
// renderInlineMarkdown, so they are exempt here.
func TestMarkdownInlineTagCoverage(t *testing.T) {
	vocab := model.DefaultVocabulary()
	for _, typ := range vocab.TypesInCategory("formatting") {
		if _, ok := mdInlineTag[typ]; !ok {
			t.Errorf("formatting type %q has no Markdown projection in mdInlineTag", typ)
		}
	}
}
