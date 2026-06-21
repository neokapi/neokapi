package asciidoc

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// adocInlineExempt lists formatting types AsciiDoc has no native inline markup
// for. Their content survives (the wrapper is dropped); they are intentionally
// absent from adocInlineTag. Keeping the exemptions explicit means any OTHER
// newly added formatting type still trips TestAsciidocInlineTagCoverage.
var adocInlineExempt = map[string]bool{
	"fmt:bidi":        true,
	"fmt:handwriting": true,
}

// TestAsciidocInlineTagCoverage guards against projection drift: every
// attribute-free `fmt:` formatting type the shared vocabulary defines must
// either have an AsciiDoc delimiter mapping in adocInlineTag or be an explicit
// exemption, so a newly added inline type cannot silently fall through to plain
// text on the cross-format export path. Link and image types carry attributes
// and are handled separately in renderInlineAsciidoc, so they are exempt here.
func TestAsciidocInlineTagCoverage(t *testing.T) {
	vocab := model.DefaultVocabulary()
	for _, typ := range vocab.TypesInCategory("formatting") {
		if _, ok := adocInlineTag[typ]; ok {
			continue
		}
		if adocInlineExempt[typ] {
			continue
		}
		t.Errorf("formatting type %q has no AsciiDoc projection in adocInlineTag and is not exempt", typ)
	}
}
