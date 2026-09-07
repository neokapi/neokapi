package openxml

// okapi-filter: openxml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A DrawingML run states its formatting on <a:rPr> (ECMA-376 Part 1
// §21.1.2.3.9): five attributes the model names, a dozen more it does not
// (`lang`, `sz`, `dirty`, `smtClean`, `spc`, `kern`, `err`), and children that
// carry the colour, the typeface and the hyperlink.
//
// The reader kept the five and skipped the element, so every slide came back
// with the rest gone and with runs that differed only in size or colour merged
// into one. #2512 reported it against 1009-1.pptx, whose slide layouts lose
// `<a:rPr lang="en-US" smtClean="0"/>` on every run.
//
// The answer is the one SpreadsheetML rich text reached in #2478: a declared
// code per named formatting type, so a translator sees where the bold starts,
// and one opaque paired code carrying the source bytes, so nothing else has to
// be modelled to survive. The two families share the projection
// (run_projection.go) and differ only in what each half carries.

const dmlContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>` +
	`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>` +
	`</Types>`

const dmlRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>` +
	`</Relationships>`

const dmlPresentation = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
	`<p:sldIdLst><p:sldId id="256" r:id="rId1" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></p:sldIdLst>` +
	`</p:presentation>`

const dmlPresentationRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/>` +
	`</Relationships>`

// dmlSlide wraps one <a:p> sequence in the smallest shape a slide can hold.
func dmlSlide(paragraphs string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
		`xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="2" name="Title 1"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/>` + paragraphs +
		`</p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
}

func dmlDeck(t *testing.T, slide string) []byte {
	t.Helper()
	return buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", dmlContentTypes},
		{"_rels/.rels", dmlRootRels},
		{"ppt/presentation.xml", dmlPresentation},
		{"ppt/_rels/presentation.xml.rels", dmlPresentationRels},
		{"ppt/slides/slide1.xml", slide},
	})
}

// dmlRichParagraphs is the shape #2512 reported: runs whose <a:rPr> carries the
// language, the size, the smart-tag flag and a colour, one of them bold.
const dmlRichParagraphs = `<a:p>` +
	`<a:r><a:rPr lang="en-US" sz="1800" dirty="0" smtClean="0"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:rPr><a:t>Red </a:t></a:r>` +
	`<a:r><a:rPr lang="en-US" sz="1800" b="1" dirty="0" smtClean="0"/><a:t>bold</a:t></a:r>` +
	`<a:r><a:rPr lang="en-US" sz="2400" dirty="0" smtClean="0"/><a:t> big</a:t></a:r>` +
	`<a:endParaRPr lang="en-US" dirty="0"/></a:p>`

func dmlSlideXML(t *testing.T, out []byte) string {
	t.Helper()
	return string(zipPartBytes(t, out, "ppt/slides/slide1.xml"))
}

// TestDMLRunProperties_UntranslatedRoundTripIsByteIdentical is the fidelity
// half: the element the reader did not model goes back as it was written.
func TestDMLRunProperties_UntranslatedRoundTripIsByteIdentical(t *testing.T) {
	slide := dmlSlide(dmlRichParagraphs)
	out := skeletonRoundtripBytes(t, dmlDeck(t, slide), "deck.pptx")
	assert.Equal(t, slide, dmlSlideXML(t, out),
		"an untranslated shape must go back byte for byte")
}

// TestDMLRunProperties_RunsThatDifferDoNotMerge is the merged-run rule #2520
// stated for SpreadsheetML: two runs that differ only in size are two runs.
func TestDMLRunProperties_RunsThatDifferDoNotMerge(t *testing.T) {
	blocks := dmlBlocks(t, dmlDeck(t, dmlSlide(dmlRichParagraphs)))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Red bold big", blocks[0].SourceText())

	out := dmlWriteBack(t, dmlDeck(t, dmlSlide(dmlRichParagraphs)))
	got := dmlSlideXML(t, out)
	assert.Equal(t, 3, strings.Count(got, "<a:r>"),
		"the three source runs stay three runs through a translation")
	assert.Contains(t, got, `<a:srgbClr val="FF0000"/>`, "the red run keeps its colour")
	assert.Contains(t, got, `sz="2400"`, "the big run keeps its size")
	assert.Contains(t, got, "RED ", "the translation is written back")
}

// TestDMLRunProperties_ModelCarriesNamedAndOpaqueCodes states what the block
// looks like: a declared code per named formatting type, and one opaque code
// holding the source element.
func TestDMLRunProperties_ModelCarriesNamedAndOpaqueCodes(t *testing.T) {
	blocks := dmlBlocks(t, dmlDeck(t, dmlSlide(dmlRichParagraphs)))
	require.Len(t, blocks, 1)

	var opaque, bold int
	for _, r := range blocks[0].Source {
		if r.PcOpen == nil {
			continue
		}
		switch r.PcOpen.Type {
		case TypeDMLRunProps:
			opaque++
			assert.True(t, strings.HasPrefix(r.PcOpen.Attr(AttrDMLRPr), "<a:rPr"),
				"the opaque code carries the source element")
			assert.Empty(t, r.PcOpen.Data,
				"the source bytes travel on AttrDMLRPr, not on Data, so no other format replays them")
		case TypeBold:
			bold++
		}
	}
	assert.Equal(t, 3, opaque, "one opaque code per run: the three runs carry different <a:rPr>")
	assert.Equal(t, 1, bold, "only the middle run is bold")
}

// TestDMLRunProperties_TurningBoldOffRemovesTheAttribute covers the edit the
// reconciliation exists for: the codes, not the source bytes, decide the named
// formatting.
func TestDMLRunProperties_TurningBoldOffRemovesTheAttribute(t *testing.T) {
	out := dmlWriteBackRuns(t, dmlDeck(t, dmlSlide(dmlRichParagraphs)), func(src []model.Run) []model.Run {
		var kept []model.Run
		var dropped []string
		for _, r := range src {
			if r.PcOpen != nil && r.PcOpen.Type == TypeBold {
				dropped = append(dropped, r.PcOpen.ID)
				continue
			}
			if r.PcClose != nil && len(dropped) > 0 && r.PcClose.ID == dropped[len(dropped)-1] {
				dropped = dropped[:len(dropped)-1]
				continue
			}
			kept = append(kept, r)
		}
		return kept
	})
	got := dmlSlideXML(t, out)
	assert.NotContains(t, got, `b="1"`, "the bold attribute goes when its code does")
	assert.Contains(t, got, `<a:rPr lang="en-US" sz="1800" dirty="0" smtClean="0"/><a:t>bold</a:t>`,
		"every other byte of the element stays where it was")
}

// TestDMLRunProperties_TurningBoldOnAddsTheAttribute is the other direction: a
// run whose source carried no bold gets the canonical attribute.
func TestDMLRunProperties_TurningBoldOnAddsTheAttribute(t *testing.T) {
	out := dmlWriteBackRuns(t, dmlDeck(t, dmlSlide(dmlRichParagraphs)), func(src []model.Run) []model.Run {
		var boldID string
		for _, r := range src {
			if r.PcOpen != nil && r.PcOpen.Type == TypeBold {
				boldID = r.PcOpen.ID
			}
		}
		require.NotEmpty(t, boldID)
		var out []model.Run
		for _, r := range src {
			if r.PcOpen != nil && r.PcOpen.Type == TypeDMLRunProps && strings.Contains(r.PcOpen.Attr(AttrDMLRPr), `sz="2400"`) {
				out = append(out, r, model.Run{PcOpen: &model.PcOpenRun{ID: boldID + "x", Type: TypeBold, SubType: SubTypeBold}})
				continue
			}
			out = append(out, r)
		}
		return out
	})
	got := dmlSlideXML(t, out)
	assert.Contains(t, got, `<a:rPr lang="en-US" sz="2400" dirty="0" smtClean="0" b="1"/>`,
		"a wanted attribute the source lacked is written in the canonical spelling, after the ones it had")
}

// TestDMLRunProperties_UnderlineKeepsItsSourceValue covers a named attribute
// whose source value says more than the type: `u="dbl"` is not `u="sng"`.
func TestDMLRunProperties_UnderlineKeepsItsSourceValue(t *testing.T) {
	slide := dmlSlide(`<a:p><a:r><a:rPr lang="en-US" u="dbl" dirty="0"/><a:t>Underlined</a:t></a:r></a:p>`)
	got := dmlSlideXML(t, dmlWriteBack(t, dmlDeck(t, slide)))
	assert.Contains(t, got, `u="dbl"`,
		"the code says underlined; the element keeps the form it was read in")
}

// TestDMLRunProperties_FormattingBoundaryClosesTheRun covers the case where a
// stretch of formatting opens with no close before it: the run has to end
// where the code opens, or the formatting reaches the wrong text.
func TestDMLRunProperties_FormattingBoundaryClosesTheRun(t *testing.T) {
	slide := dmlSlide(`<a:p><a:r><a:t>plain</a:t></a:r><a:r><a:rPr b="1"/><a:t>bold</a:t></a:r></a:p>`)
	got := dmlSlideXML(t, dmlWriteBack(t, dmlDeck(t, slide)))
	assert.Contains(t, got, `<a:r><a:t>PLAIN</a:t></a:r>`, "the unformatted stretch stays unformatted")
	assert.Contains(t, got, `<a:rPr b="1"/><a:t>BOLD</a:t>`, "the bold stretch carries the bold")
}

// TestDMLParagraph_FieldSurvivesBesideText covers the second half of #2512: a
// paragraph that carries text was rebuilt from its runs alone, so every other
// child of <a:p> was deleted.
func TestDMLParagraph_FieldSurvivesBesideText(t *testing.T) {
	const fld = `<a:fld id="{6CB26F11-8631-43B9-8796-736DC3103ED4}" type="slidenum">` +
		`<a:rPr lang="en-US" smtClean="0"/><a:t>2</a:t></a:fld>`
	slide := dmlSlide(`<a:p><a:r><a:rPr lang="en-US"/><a:t>Slide </a:t></a:r>` + fld + `</a:p>`)

	assert.Equal(t, slide, dmlSlideXML(t, skeletonRoundtripBytes(t, dmlDeck(t, slide), "deck.pptx")),
		"an untranslated paragraph holding a field must go back byte for byte")

	blocks := dmlBlocks(t, dmlDeck(t, slide))
	require.Len(t, blocks, 1)
	var fields int
	for _, r := range blocks[0].Source {
		if r.Ph != nil && r.Ph.Type == TypeOpaqueParaChild {
			fields++
			assert.Equal(t, SubTypeDMLField, r.Ph.SubType)
			assert.Equal(t, fld, r.Ph.Data)
		}
	}
	assert.Equal(t, 1, fields, "the field reaches the block as a placeholder rather than being dropped")

	assert.Contains(t, dmlSlideXML(t, dmlWriteBack(t, dmlDeck(t, slide))), fld,
		"a translated paragraph keeps the field")
}

// TestDMLParagraph_BreakKeepsItsRunProperties covers <a:br>, which carries the
// <a:rPr> that decides the height of the line it starts.
func TestDMLParagraph_BreakKeepsItsRunProperties(t *testing.T) {
	const br = `<a:br><a:rPr lang="en-US" sz="1200" dirty="0"/></a:br>`
	slide := dmlSlide(`<a:p><a:r><a:rPr lang="en-US"/><a:t>one</a:t></a:r>` + br +
		`<a:r><a:rPr lang="en-US"/><a:t>two</a:t></a:r></a:p>`)

	assert.Equal(t, slide, dmlSlideXML(t, skeletonRoundtripBytes(t, dmlDeck(t, slide), "deck.pptx")),
		"an untranslated paragraph holding a break must go back byte for byte")
	assert.Contains(t, dmlSlideXML(t, dmlWriteBack(t, dmlDeck(t, slide))), br,
		"a translated paragraph keeps the break's own properties")
}

// TestDMLRunProperties_UpstreamFixture is #2512 on the file it was reported
// against: every slide layout in 1009-1.pptx loses <a:rPr lang="en-US"
// smtClean="0"/> on every run.
func TestDMLRunProperties_UpstreamFixture(t *testing.T) {
	path := filepath.Join(testdataDir(t), "1009-1.pptx")
	original, err := os.ReadFile(path)
	require.NoError(t, err)

	out := skeletonRoundtripBytes(t, original, filepath.Base(path))
	for _, part := range []string{
		"ppt/slides/slide1.xml",
		"ppt/slideLayouts/slideLayout1.xml",
		"ppt/slideLayouts/slideLayout2.xml",
		"ppt/notesSlides/notesSlide1.xml",
	} {
		assert.Equal(t, string(zipPartBytes(t, original, part)), string(zipPartBytes(t, out, part)),
			"%s must go back byte for byte", part)
	}
}

// dmlBlocks reads a deck and returns the blocks its slide produced.
func dmlBlocks(t *testing.T, pkg []byte) []*model.Block {
	t.Helper()
	reader := NewReader()
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          "deck.pptx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(pkg),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	require.NoError(t, reader.Close())

	var out []*model.Block
	for _, b := range testutil.FilterBlocks(parts) {
		if b.Translatable && b.Properties["partPath"] == "ppt/slides/slide1.xml" {
			out = append(out, b)
		}
	}
	return out
}

// dmlWriteBack translates every block by upper-casing its text, leaving the
// codes where they are, which is what an editor produces.
func dmlWriteBack(t *testing.T, pkg []byte) []byte {
	t.Helper()
	return dmlWriteBackRuns(t, pkg, func(src []model.Run) []model.Run {
		out := make([]model.Run, len(src))
		for i, r := range src {
			if r.Text != nil {
				out[i] = model.TextR(strings.ToUpper(r.Text.Text))
				continue
			}
			out[i] = r
		}
		return out
	})
}

func dmlWriteBackRuns(t *testing.T, pkg []byte, translate func([]model.Run) []model.Run) []byte {
	t.Helper()
	return skeletonWriteBackRuns(t, pkg, model.LocaleID("qps"), translate)
}
