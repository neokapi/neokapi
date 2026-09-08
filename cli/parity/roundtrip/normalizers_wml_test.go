//go:build parity

package roundtrip_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/cli/parity/roundtrip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The strip has to cancel the asymmetry between a part native replays as its
// author wrote it and the same part after okapi's RunSkippableElements: the
// elements go, and so does every properties container they leave empty.
func TestXMLCanonicalStripsWMLSkippableElements(t *testing.T) {
	n := roundtrip.XMLCanonical{SortAttrs: true, StripWMLSkippableElements: true}

	native := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:pPr><w:rPr><w:lang w:val="en-US"/></w:rPr></w:pPr>` +
		`<w:r><w:rPr><w:b/><w:noProof/><w:lang w:val="en-US"/></w:rPr><w:t>Bold</w:t></w:r>` +
		`<w:r><w:rPr><w:lang w:val="en-US"/></w:rPr><w:t>Plain</w:t></w:r>` +
		`</w:p>` +
		`<w:tbl><w:tblPr><w:bidiVisual/><w:tblW w:w="0" w:type="auto"/></w:tblPr></w:tbl>` +
		`</w:body></w:document>`
	okapi := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p>` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>Bold</w:t></w:r>` +
		`<w:r><w:t>Plain</w:t></w:r>` +
		`</w:p>` +
		`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/></w:tblPr></w:tbl>` +
		`</w:body></w:document>`

	gotNative, err := n.Normalize([]byte(native))
	require.NoError(t, err)
	gotOkapi, err := n.Normalize([]byte(okapi))
	require.NoError(t, err)
	assert.Equal(t, string(gotOkapi), string(gotNative))
	assert.NotContains(t, string(gotNative), "lang")
	assert.NotContains(t, string(gotNative), "noProof")
	assert.NotContains(t, string(gotNative), "pPr")
}

// A container the source wrote empty is dropped on both sides too: okapi omits
// it, and native keeps it, so the comparison must not see it either way.
func TestXMLCanonicalStripsEmptyWMLPropertyContainers(t *testing.T) {
	n := roundtrip.XMLCanonical{StripWMLSkippableElements: true}

	in := "<w:p xmlns:w=\"http://schemas.openxmlformats.org/wordprocessingml/2006/main\">\n" +
		"  <w:pPr>\n    <w:rPr>\n      <w:lang w:val=\"en-US\"/>\n    </w:rPr>\n  </w:pPr>" +
		`<w:r><w:rPr/><w:t>x</w:t></w:r></w:p>`
	want := `<w:p xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:r><w:t>x</w:t></w:r></w:p>`

	got, err := n.Normalize([]byte(in))
	require.NoError(t, err)
	wantCanon, err := n.Normalize([]byte(want))
	require.NoError(t, err)
	assert.Equal(t, string(wantCanon), string(got))
}

// Without the option the elements stay, so the option is what cancels the
// asymmetry rather than the canonical re-emission.
func TestXMLCanonicalKeepsWMLSkippableElementsByDefault(t *testing.T) {
	n := roundtrip.XMLCanonical{}
	in := `<w:r xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:rPr><w:lang w:val="en-US"/></w:rPr><w:t>x</w:t></w:r>`
	got, err := n.Normalize([]byte(in))
	require.NoError(t, err)
	assert.Contains(t, string(got), "lang")
	assert.Equal(t, "xml-canonical", n.Name())
	assert.Equal(t, "xml-canonical(strip-wml-skippable)",
		roundtrip.XMLCanonical{StripWMLSkippableElements: true}.Name())
}

// docxOf builds a package holding the two parts the effective-rPr pass reads.
func docxOf(t *testing.T, document, styles string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range []struct{ name, data string }{
		{"word/document.xml", document},
		{"word/styles.xml", styles},
	} {
		w, err := zw.Create(e.name)
		require.NoError(t, err)
		_, err = w.Write([]byte(e.data))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// The cascade must not fill a container okapi dropped. A paragraph mark
// holding only a `<w:lang>` on the native side has no counterpart on the
// okapi side; with the strip ahead of the cascade both sides resolve to the
// same paragraph, and a `<w:lang>` in docDefaults reaches no run.
func TestOpenXMLEffectiveRPrStripsSkippableElementsBeforeTheCascade(t *testing.T) {
	const styles = `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:docDefaults><w:rPrDefault><w:rPr><w:sz w:val="22"/><w:lang w:val="en-US"/></w:rPr></w:rPrDefault></w:docDefaults>` +
		`</w:styles>`
	const okapiStyles = `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:docDefaults><w:rPrDefault><w:rPr><w:sz w:val="22"/></w:rPr></w:rPrDefault></w:docDefaults>` +
		`</w:styles>`
	const native = `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:pPr><w:rPr><w:lang w:val="en-US"/></w:rPr></w:pPr><w:r><w:t>x</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	const okapi = `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:t>x</w:t></w:r></w:p>` +
		`</w:body></w:document>`

	chain := roundtrip.Chain{Steps: []roundtrip.Normalizer{
		roundtrip.OpenXMLEffectiveRPr{StripSkippableElements: true},
		roundtrip.ZipEntryNormalizer{Inner: roundtrip.XMLCanonical{SortAttrs: true, StripWMLSkippableElements: true}},
	}}
	gotNative, err := chain.Normalize(docxOf(t, native, styles))
	require.NoError(t, err)
	gotOkapi, err := chain.Normalize(docxOf(t, okapi, okapiStyles))
	require.NoError(t, err)
	assert.Equal(t, string(gotOkapi), string(gotNative))
	assert.NotContains(t, string(gotNative), "lang")
	assert.Contains(t, string(gotNative), `val="22"`, "the rest of docDefaults still cascades onto the run")

	plain := roundtrip.Chain{Steps: []roundtrip.Normalizer{
		roundtrip.OpenXMLEffectiveRPr{},
		roundtrip.ZipEntryNormalizer{Inner: roundtrip.XMLCanonical{SortAttrs: true, StripWMLSkippableElements: true}},
	}}
	filledNative, err := plain.Normalize(docxOf(t, native, styles))
	require.NoError(t, err)
	filledOkapi, err := plain.Normalize(docxOf(t, okapi, okapiStyles))
	require.NoError(t, err)
	assert.NotEqual(t, string(filledOkapi), string(filledNative),
		"without the option the cascade fills the emptied paragraph mark on one side only")
	assert.Equal(t, "openxml-effective-rpr(strip-skippable)", roundtrip.OpenXMLEffectiveRPr{StripSkippableElements: true}.Name())
}
