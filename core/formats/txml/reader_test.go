package txml_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/txml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// All test snippets follow Wordfast Pro's real TXML schema, mirroring
// the upstream STARTFILE constant in TXMLFilterTest.
const startFile = `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" segtype="sentence" createdby="WF2.3.0" datatype="regexp" targetlocale="fr" file_extension="html" editedby="WF2.3.0">
<skeleton>&lt;html&gt;&lt;p&gt;</skeleton>`

const simpleTwoSegmentsTXML = startFile + `<translatable blockId="b1" datatype="html"><segment segmentId="s1" modified="true"><source>Segment one</source><target>Segment un</target></segment><segment segmentId="2"><source>segment two</source></segment></translatable></txml>`

const sourceOnlyTXML = startFile + `<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Source only text</source></segment></translatable></txml>`

const inlineCodesTXML = startFile + `<translatable blockId="b1" datatype="html"><segment segmentId="s1" modified="true"><source>Segment one</source><target>Segment un</target></segment><segment segmentId="2"><source>Segment <ut x='1' type='bold'>&lt;b></ut>TWO<ut x='2' type='bold'>&lt;/b></ut></source></segment></translatable></txml>`

// okapi: TXMLFilterTest#testSimpleEntry
// Extracts source and target from one <translatable> with two <segment>
// children (the second is source-only), mirroring the Java assertions
// tu.getId()=="b1", first source segment "Segment one", target "Segment un".
func TestReadSimpleTranslatable(t *testing.T) {
	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTwoSegmentsTXML, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// One <translatable> => one Block whose source carries two segment
	// spans over the single Run sequence (concatenated text).
	require.Len(t, blocks, 1)
	assert.Equal(t, "b1", blocks[0].ID)
	assert.Equal(t, "Segment onesegment two", blocks[0].SourceText())
	require.Equal(t, 2, blocks[0].SourceSegmentCount())
	assert.Equal(t, "Segment one", model.RunsText(blocks[0].SourceSegmentRuns(0)))
	assert.Equal(t, "segment two", model.RunsText(blocks[0].SourceSegmentRuns(1)))
	assert.True(t, blocks[0].HasTarget("fr"))
	// Target only present for the first segment: a single dense target
	// span over the target runs.
	frKey := model.Variant(model.LocaleID("fr"))
	trgOv := blocks[0].SegmentationFor(&frKey)
	require.NotNil(t, trgOv)
	require.Len(t, trgOv.Spans, 1)
	assert.Equal(t, "Segment un", model.RunsText(trgOv.Spans[0].Range.ExtractRuns(blocks[0].TargetRuns("fr"))))
}

// okapi: TXMLFilterTest#testSimpleEntry
// The <translatable datatype="..."> attribute is preserved on the Block
// (neokapi exposes it as a Property; Okapi carries it on the TU MIME type).
func TestReadDatatypeProperty(t *testing.T) {
	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTwoSegmentsTXML, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "html", blocks[0].Properties["datatype"])
}

// okapi: TXMLFilterTest#testSimpleEntry
// A <segment> with only <source> (no <target>) produces a Block with no
// target entry for that slot — the second segment in testSimpleEntry.
func TestReadSourceOnlySegment(t *testing.T) {
	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sourceOnlyTXML, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Source only text", blocks[0].SourceText())
	assert.False(t, blocks[0].HasTarget("fr"))
}

// okapi: TXMLFilterTest#testEntryWithCodes
// <ut x type> elements become inline PlaceholderRun codes that contribute
// nothing to plain SourceText(), matching Okapi's "Segment <1/>TWO<2/>".
func TestReadUTInlineCodes(t *testing.T) {
	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(inlineCodesTXML, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Segment oneSegment TWO", blocks[0].SourceText())

	// Inspect the second segment's runs (extracted via the source
	// segmentation overlay).
	require.Equal(t, 2, blocks[0].SourceSegmentCount())
	runs := blocks[0].SourceSegmentRuns(1)
	require.Len(t, runs, 4) // text "Segment ", ut, text "TWO", ut
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "Segment ", runs[0].Text.Text)
	require.NotNil(t, runs[1].Ph)
	assert.Equal(t, "1", runs[1].Ph.ID)
	assert.Equal(t, "bold", runs[1].Ph.Type)
	assert.Equal(t, "<b>", runs[1].Ph.Data)
	require.NotNil(t, runs[2].Text)
	assert.Equal(t, "TWO", runs[2].Text.Text)
	require.NotNil(t, runs[3].Ph)
	assert.Equal(t, "2", runs[3].Ph.ID)
	assert.Equal(t, "</b>", runs[3].Ph.Data)
}

// okapi: TXMLFilterTest#testEntryWithFirstOutOf2SegmentsCommentedOut
// An XML-commented first <segment> is excluded; only the live second
// segment ("segment two"/"segment deux") is extracted.
func TestReadCommentedFirstSegment(t *testing.T) {
	snippet := startFile + `<translatable blockId="b1" datatype="html"><!--<segment segmentId="s1" modified="true"><source>Segment one</source><target>Segment un</target></segment>--><segment segmentId="2"><source>segment two</source><target>segment deux</target></segment></translatable></txml>`

	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "segment two", blocks[0].SourceText())
	assert.True(t, blocks[0].HasTarget("fr"))
	assert.Equal(t, "segment deux", blocks[0].TargetText("fr"))
}

// okapi: TXMLFilterTest#testEntryWithAllSegmentsCommentedOut
// When every <segment> is XML-commented, the <translatable> yields no
// Block (Okapi's getTextUnit returns null).
func TestReadAllSegmentsCommented(t *testing.T) {
	snippet := startFile + `<translatable blockId="b1" datatype="html"><!--<segment segmentId="s1"><source>Segment one</source><target>Segment un</target></segment>--><!--<segment segmentId="2"><source>segment two</source><target>segment deux</target></segment>--></translatable></txml>`

	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	assert.Empty(t, blocks)
}

// okapi: TXMLFilterTest#testEntryWithThirdSegmentsNotCommentedOut
// With the first two segments commented out, only the live third
// segment ("segment three") survives extraction.
func TestReadThirdSegmentLive(t *testing.T) {
	snippet := startFile + `<translatable blockId="b1" datatype="html"><!--<segment segmentId="s1"><source>Segment one</source><target>Segment un</target></segment>--><!--<segment segmentId="2"><source>segment two</source><target>segment deux</target></segment>--><segment segmentId="3"><source>segment three</source><target>segment trois</target></segment></translatable></txml>`

	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "segment three", blocks[0].SourceText())
}

// okapi: TXMLFilterTest#testWS
// <ws> siblings carry skeleton whitespace and never enter the joined
// source text. Okapi keeps <ws> as inter-segment holders in the
// TextContainer ("  [text S]  <1/> [text S2] \t"); neokapi treats <ws>
// purely as skeleton, so it is absent from SourceText() but the segment
// boundaries and codes are preserved identically.
func TestReadWSExcludedFromSource(t *testing.T) {
	snippet := startFile + `<translatable blockId="b1" datatype="html"><segment segmentId="s1"><ws>  </ws><source>text S</source><target>text T</target><ws>  <ut x='1'>&lt;br/></ut> </ws></segment><segment segmentId="s2"><source>text S2</source><ws> 	</ws></segment></translatable></txml>`

	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "text Stext S2", blocks[0].SourceText())
}

// okapi: TXMLFilterTest#testRevisedEntry
// <revisions> children are ignored; only the live <source>/<target>
// text is extracted (Okapi: source "Segment one", target "Segment un",
// the prior-revision target "previous translation" is not absorbed).
func TestReadRevisionMetadataIgnored(t *testing.T) {
	snippet := startFile + `<translatable blockId="b1" datatype="html"><segment segmentId="s1" modified="true"><source>Segment one</source><target>Segment un</target><revisions><revision id="1" creationid="Roberto" creationdate="20130109T162701Z" type="target"><target>previous translation</target></revision></revisions></segment><segment segmentId="2"><source>segment two</source></segment></translatable></txml>`

	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Segment onesegment two", blocks[0].SourceText())
	assert.NotContains(t, blocks[0].SourceText(), "previous translation")
	// Live target is preserved; the revision target is not absorbed.
	assert.Equal(t, "Segment un", blocks[0].TargetText("fr"))
}

// okapi: TXMLFilterTest#testEmptySegments
// A <segment> with an empty <source></source> contributes an empty
// Segment (no runs); the joined block text is empty and, since no
// <target> is present, the block has no French target — matching
// Okapi's "  []  <1/> []  \t" source and null getTarget(fr).
func TestReadEmptySources(t *testing.T) {
	snippet := startFile + `<translatable blockId="b1" datatype="html"><segment segmentId="s1"><ws>  </ws><source></source><ws>  <ut x='1'>&lt;br/></ut> </ws></segment><segment segmentId="s2"><source></source><ws> 	</ws></segment></translatable></txml>`

	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Empty(t, blocks[0].SourceText())
	// Two empty <source></source> segments → two (zero-width) source
	// segment spans over an empty Run sequence.
	require.Equal(t, 2, blocks[0].SourceSegmentCount())
	// No <target> children present → no French target (Okapi: getTarget(fr)==null).
	assert.False(t, blocks[0].HasTarget("fr"))
}

// okapi: TXMLFilterTest#testEntryWithSecondOutOf2SegmentsCommentedOut
// The second of two <segment>s is XML-commented out; only the live
// first segment ("Segment one"/"Segment un") survives extraction.
func TestReadCommentedSecondSegment(t *testing.T) {
	snippet := startFile + `<translatable blockId="b1" datatype="html"><segment segmentId="s1" modified="true"><source>Segment one</source><target>Segment un</target></segment><!--<segment segmentId="2"><source>segment two</source><target>segment deux</target></segment>--></translatable></txml>`

	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "b1", blocks[0].ID)
	assert.Equal(t, "Segment one", blocks[0].SourceText())
	require.Len(t, blocks[0].Source, 1)
	assert.True(t, blocks[0].HasTarget("fr"))
	assert.Equal(t, "Segment un", blocks[0].TargetText("fr"))
}

// okapi: TXMLFilterTest#testEntryWith1SegmentCommentedOut
// A <translatable> whose single <segment> is XML-commented out yields no
// Block at all (Okapi's getTextUnit returns null).
func TestReadSingleSegmentCommented(t *testing.T) {
	snippet := startFile + `<translatable blockId="b1" datatype="html"><!--<segment segmentId="s1" modified="true"><source>Segment one</source><target>Segment un</target></segment>--></translatable></txml>`

	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	assert.Empty(t, blocks)
}

// okapi: TXMLFilterTest#testSegments
// Three segments: the first carries source+target plus <ws> holders, the
// second is source-only with a trailing <ws>, and the third has an empty
// <source></source> wrapped in <ws> braces. Okapi renders the source as
// "  [textS1]  <1/> [textS2] \t{{[]}}" and the target as
// "  [textT1]  <1/> [] \t{{[]}}". In neokapi the <ws> holders are
// skeleton-only, so the joined source text is "textS1textS2", three
// source segments exist (third empty), and only the first segment has a
// French target — the same segmentation and target presence Okapi shows.
func TestReadThreeSegmentsWithEmptyThird(t *testing.T) {
	snippet := startFile + `<translatable blockId="b1" datatype="html"><segment segmentId="s1"><ws>  </ws><source>textS1</source><target>textT1</target><ws>  <ut x='1'>&lt;br/></ut> </ws></segment><segment segmentId="s2"><source>textS2</source><ws> 	</ws></segment><segment segmentId="s3"><ws>{{</ws><source></source><ws>}}</ws></segment></translatable></txml>`

	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)

	// Three source segments; third is empty (empty <source></source>).
	require.Equal(t, 3, blocks[0].SourceSegmentCount())
	assert.Equal(t, "textS1", model.RunsText(blocks[0].SourceSegmentRuns(0)))
	assert.Equal(t, "textS2", model.RunsText(blocks[0].SourceSegmentRuns(1)))
	assert.Empty(t, model.RunsText(blocks[0].SourceSegmentRuns(2)))
	assert.Equal(t, "textS1textS2", blocks[0].SourceText())

	// Only the first segment carries a <target>: a single dense target span.
	frKey := model.Variant(model.LocaleID("fr"))
	trgOv := blocks[0].SegmentationFor(&frKey)
	require.NotNil(t, trgOv)
	require.Len(t, trgOv.Spans, 1)
	assert.Equal(t, "textT1", model.RunsText(trgOv.Spans[0].Range.ExtractRuns(blocks[0].TargetRuns("fr"))))
}

// neokapi-only: the streaming Part model has no Okapi counterpart test.
// Verifies LayerStart/LayerEnd wrap the TXML content and carry the
// source/target locale parsed from the <txml> root element.
func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTwoSegmentsTXML, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "txml", layer.Format)
	assert.Equal(t, model.LocaleID("en"), layer.Locale)
	assert.Equal(t, "fr", layer.Properties["target-locale"])
}

// neokapi-only: format detection is registry-driven in neokapi (no Okapi
// equivalent test). Verifies the TXML MIME type, extension and sniffer.
func TestReaderSignature(t *testing.T) {
	reader := txml.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-txml+xml")
	assert.Contains(t, sig.Extensions, ".txml")
	assert.NotNil(t, sig.Sniff)
	assert.True(t, sig.Sniff([]byte(`<txml locale="en">`)))
	assert.False(t, sig.Sniff([]byte(`<html>`)))
}

func TestReaderMetadata(t *testing.T) {
	reader := txml.NewReader()
	assert.Equal(t, "txml", reader.Name())
	assert.Equal(t, "Wordfast Pro TXML", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

// Roundtrip via the direct (non-skeleton) writer path.
func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTwoSegmentsTXML, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := txml.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale("fr")

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Segment one")
	assert.Contains(t, output, "Segment un")
	assert.Contains(t, output, "<txml")
	assert.Contains(t, output, "<translatable")
	assert.Contains(t, output, `locale="en"`)
	assert.Contains(t, output, `targetlocale="fr"`)
}

func TestRoundTripWithNewTarget(t *testing.T) {
	ctx := t.Context()

	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sourceOnlyTXML, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Add target translation.
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			block.SetTargetText("fr", "Texte source uniquement")
		}
	}

	var buf bytes.Buffer
	writer := txml.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale("fr")

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Source only text")
	assert.Contains(t, output, "Texte source uniquement")
}

// Regression: real Wordfast Pro TXML fixture from the upstream Okapi
// test corpus must parse to one or more translatable Blocks. Skipped
// cleanly when okapi-testdata isn't checked out.
func TestReadOkapiTest01Fixture(t *testing.T) {
	path := findOkapiFixture(t, "okapi/filters/txml/src/test/resources/Test01.docx.txml")
	if path == "" {
		t.Skip("okapi-testdata not available")
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	ctx := t.Context()
	reader := txml.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromString(string(data), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks, "Test01.docx.txml should yield translatable blocks")
	require.GreaterOrEqual(t, len(blocks), 9, "Test01.docx.txml has 9 <translatable> elements")

	// Spot-check a known piece of source text from the fixture.
	hasEndNote := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "This is the text of the end note.") {
			hasEndNote = true
			break
		}
	}
	assert.True(t, hasEndNote, "should find the end-note source segment")
}

// findOkapiFixture walks up from cwd looking for go.work, then resolves
// rel under okapi-testdata/<latest-version>/. Returns "" when the
// corpus isn't checked out so the caller can skip cleanly.
func findOkapiFixture(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	base := filepath.Join(dir, "okapi-testdata")
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	var latest string
	for _, e := range entries {
		if e.IsDir() && e.Name() > latest {
			latest = e.Name()
		}
	}
	if latest == "" {
		return ""
	}
	return filepath.Join(base, latest, rel)
}
