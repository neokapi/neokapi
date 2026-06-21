package html

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// TestHTMLInlineTagCoverage guards against projection drift: every
// attribute-free `fmt:` formatting type the shared vocabulary defines must have
// an HTML open/close mapping in htmlInlineTag, so a newly added inline type
// cannot silently fall through to plain text on the cross-format export path.
// Link and image types carry attributes and are handled separately in
// renderInlineHTML, so they are exempt here.
func TestHTMLInlineTagCoverage(t *testing.T) {
	vocab := model.DefaultVocabulary()
	for _, typ := range vocab.TypesInCategory("formatting") {
		if _, ok := htmlInlineTag[typ]; !ok {
			t.Errorf("formatting type %q has no HTML projection in htmlInlineTag", typ)
		}
	}
}
