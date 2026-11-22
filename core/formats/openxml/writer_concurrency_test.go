package openxml

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// concurrencyDocument is one bold run whose rPr authors the bare-on toggle
// pair `<w:b/><w:bCs/>` over Latin text. Whether the writer keeps the `<w:bCs/>`
// depends only on the docDefaults of the package's own styles.xml.
const concurrencyDocument = `<w:document ` + wmlNS + `><w:body>` +
	`<w:p><w:r><w:rPr><w:b/><w:bCs/></w:rPr><w:t>Bold text</w:t></w:r></w:p>` +
	`</w:body></w:document>`

// concurrencyStyles is a styles part whose docDefaults rPr carries extra.
func concurrencyStyles(extra string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<w:styles ` + wmlNS + `>` +
		`<w:docDefaults><w:rPrDefault><w:rPr>` + extra + `</w:rPr></w:rPrDefault></w:docDefaults>` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` +
		`</w:styles>`
}

// Writes of different packages running at once each read the docDefaults
// toggles of their own styles.xml. A package whose docDefaults clears bCs keeps
// a run's bare-on <w:bCs/> (upstream RunParser.canBeSkipped refuses the strip);
// one whose docDefaults leaves bCs alone has it stripped. Run under -race, the
// test also catches any per-Write state shared between Writers.
func TestWriter_ConcurrentWritesKeepTheirOwnDocDefaults(t *testing.T) {
	cleared := docxWithParts(t, map[string]string{
		"word/document.xml": concurrencyDocument,
		"word/styles.xml":   concurrencyStyles(`<w:bCs w:val="0"/><w:iCs w:val="0"/>`),
	})
	plain := docxWithParts(t, map[string]string{
		"word/document.xml": concurrencyDocument,
		"word/styles.xml":   concurrencyStyles(`<w:lang w:val="en-US"/>`),
	})

	// The answer each package gives alone.
	wantCleared := string(zipPartBytes(t, translateRoundtripBytes(t, cleared, "cleared.docx"), "word/document.xml"))
	wantPlain := string(zipPartBytes(t, translateRoundtripBytes(t, plain, "plain.docx"), "word/document.xml"))
	assert.Contains(t, wantCleared, `<w:bCs/>`, "docDefaults bCs=0 keeps the run's bare-on bCs")
	assert.NotContains(t, wantPlain, `<w:bCs/>`, "without a docDefaults bCs the mirror is stripped")

	t.Run("concurrent", func(t *testing.T) {
		for i := range 16 {
			pkg, uri, want := cleared, "cleared.docx", wantCleared
			if i%2 == 1 {
				pkg, uri, want = plain, "plain.docx", wantPlain
			}
			t.Run(fmt.Sprintf("%s-%d", uri, i), func(t *testing.T) {
				t.Parallel()
				got := string(zipPartBytes(t, translateRoundtripBytes(t, pkg, uri), "word/document.xml"))
				assert.Equal(t, want, got)
			})
		}
	})
}
