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

// What a replayed paragraph keeps and okapi drops: Word's _GoBack bookmark
// with its end, a proofing mark, an empty run, an empty sdtEndPr, and the
// complex-script toggles of a run with no complex-script text.
func TestXMLCanonicalStripsWhatOkapiOmitsFromAReplayedParagraph(t *testing.T) {
	n := roundtrip.XMLCanonical{SortAttrs: true, StripWMLSkippableElements: true}

	native := `<w:body xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:p><w:proofErr w:type="spellStart"/><w:r><w:rPr><w:b/><w:bCs/><w:iCs/></w:rPr><w:t>Latin</w:t></w:r>` +
		`<w:r><w:rPr><w:b/><w:bCs/></w:rPr><w:t>עברית</w:t></w:r>` +
		`<w:r><w:rPr><w:i/></w:rPr></w:r><w:r><w:lastRenderedPageBreak/></w:r></w:p>` +
		`<w:p><w:bookmarkStart w:id="0" w:name="_GoBack"/><w:bookmarkStart w:id="1" w:name="kept"/><w:bookmarkEnd w:id="1"/></w:p>` +
		`<w:p><w:bookmarkEnd w:id="0"/></w:p>` +
		`<w:sdt><w:sdtPr><w:id w:val="1"/></w:sdtPr><w:sdtEndPr/><w:sdtContent><w:p/></w:sdtContent></w:sdt>` +
		`</w:body>`
	okapi := `<w:body xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Latin</w:t></w:r>` +
		`<w:r><w:rPr><w:b/><w:bCs/></w:rPr><w:t>עברית</w:t></w:r></w:p>` +
		`<w:p><w:bookmarkStart w:id="1" w:name="kept"/><w:bookmarkEnd w:id="1"/></w:p>` +
		`<w:p/>` +
		`<w:sdt><w:sdtPr><w:id w:val="1"/></w:sdtPr><w:sdtContent><w:p/></w:sdtContent></w:sdt>` +
		`</w:body>`

	gotNative, err := n.Normalize([]byte(native))
	require.NoError(t, err)
	gotOkapi, err := n.Normalize([]byte(okapi))
	require.NoError(t, err)
	assert.Equal(t, string(gotOkapi), string(gotNative))
}

// Adjacent runs with equal properties merge on both sides, and so do the
// text elements inside the merged run, which is the shape okapi's RunMerger
// writes.
func TestXMLCanonicalMergesAdjacentWMLRuns(t *testing.T) {
	n := roundtrip.XMLCanonical{SortAttrs: true, StripXMLSpacePreserve: true, MergeAdjacentWMLRuns: true}
	split := `<w:p xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>Bo</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">ld </w:t></w:r>` +
		`<w:r><w:t>plain</w:t></w:r><w:r><w:tab/></w:r><w:r><w:t>after</w:t></w:r>` +
		`<w:r><w:rPr><w:i/></w:rPr><w:t>it</w:t></w:r></w:p>`
	merged := `<w:p xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>Bold </w:t></w:r>` +
		`<w:r><w:t>plain</w:t><w:tab/><w:t>after</w:t></w:r>` +
		`<w:r><w:rPr><w:i/></w:rPr><w:t>it</w:t></w:r></w:p>`
	gotSplit, err := n.Normalize([]byte(split))
	require.NoError(t, err)
	gotMerged, err := n.Normalize([]byte(merged))
	require.NoError(t, err)
	assert.Equal(t, string(gotMerged), string(gotSplit))
	assert.Equal(t, "xml-canonical(sort-attrs,strip-xml-space-preserve,merge-wml-runs)", n.Name())
}

// A byte order mark goes with the declaration it precedes.
func TestStripXMLDeclarationDropsAByteOrderMark(t *testing.T) {
	got, err := roundtrip.StripXMLDeclaration{}.Normalize([]byte("\xef\xbb\xbf<?xml version=\"1.0\"?>\n<a/>"))
	require.NoError(t, err)
	assert.Equal(t, "<a/>", string(got))
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

// The core-properties strip cancels the one asymmetry left in
// docProps/core.xml: native replays a property the source wrote empty, and
// okapi's Jericho pending start tag leaves the last self-closing one out.
func TestXMLCanonicalStripsEmptyCoreProperties(t *testing.T) {
	n := roundtrip.XMLCanonical{SortAttrs: true, StripEmptyCoreProperties: true}

	const head = `<cp:coreProperties ` +
		`xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/">`
	native := head +
		`<dc:title>A title</dc:title><dc:subject></dc:subject>` +
		`<cp:revision>4</cp:revision><cp:category/>` +
		`</cp:coreProperties>`
	okapi := head +
		`<dc:title>A title</dc:title>` +
		`<cp:revision>4</cp:revision>` +
		`</cp:coreProperties>`

	gotNative, err := n.Normalize([]byte(native))
	require.NoError(t, err)
	gotOkapi, err := n.Normalize([]byte(okapi))
	require.NoError(t, err)
	assert.Equal(t, string(gotOkapi), string(gotNative))
	assert.Contains(t, string(gotNative), "A title")
	assert.Contains(t, string(gotNative), "revision", "a property outside the text-unit list stays")
	assert.NotContains(t, string(gotNative), "category")
	assert.Equal(t, "xml-canonical(sort-attrs,strip-empty-core-props)", n.Name())
}

// A property holding text keeps it whatever the surrounding whitespace, and
// an empty element outside `<cp:coreProperties>` is left alone.
func TestXMLCanonicalKeepsCorePropertiesWithText(t *testing.T) {
	n := roundtrip.XMLCanonical{StripEmptyCoreProperties: true}

	const part = `<cp:coreProperties ` +
		`xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/">` + "\n" +
		`  <dc:title>A title</dc:title>` + "\n" +
		`  <dc:creator>User</dc:creator>` + "\n" +
		`</cp:coreProperties>`
	got, err := n.Normalize([]byte(part))
	require.NoError(t, err)
	assert.Contains(t, string(got), "A title")
	assert.Contains(t, string(got), "User")

	const other = `<w:body xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:category/></w:body>`
	gotOther, err := n.Normalize([]byte(other))
	require.NoError(t, err)
	assert.Contains(t, string(gotOther), "category",
		"the strip reaches only the children of a core-properties root")
}
