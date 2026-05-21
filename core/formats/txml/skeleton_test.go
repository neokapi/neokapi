package txml_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/txml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := txml.NewReader()
	writer := txml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

// okapi: RoundTripTxmlIT#txmlFiles
// okapi-skip: RoundTripTxmlIT#txmlSerializedFiles — Okapi serialized-skeleton variant; native uses its own skeleton store
func TestSkeletonStore_ByteExact_SimpleTranslatable(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<skeleton>&lt;html&gt;&lt;p&gt;</skeleton>
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello World</source><target>Bonjour le monde</target></segment></translatable>
<skeleton>&lt;/p&gt;&lt;/html&gt;</skeleton>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SourceOnly(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Source only text</source></segment></translatable>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "source-only TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_TwoSegments(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><target>Bonjour</target></segment><segment segmentId="s2"><source>Goodbye</source><target>Au revoir</target></segment></translatable>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "two-segment TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleTranslatables(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>First block</source><target>Premier bloc</target></segment></translatable>
<translatable blockId="b2" datatype="html"><segment segmentId="s1"><source>Second block</source><target>Deuxieme bloc</target></segment></translatable>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multi-translatable TXML roundtrip should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><target>Bonjour</target></segment></translatable>
</txml>
`
	ctx := t.Context()

	reader := txml.NewReader()
	writer := txml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Modify the French translation.
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello" {
				b.Targets[model.LocaleID("fr")] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Salut"}}}}}
			}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><target>Salut</target></segment></translatable>
</txml>
`
	assert.Equal(t, expected, buf.String())
}

// Regression for the dropped-<target> bug. A source-only <segment> (no
// original <target> child) that acquires a translated target on
// write-back must emit a fresh <target>…</target> just before
// </segment>. Previously the reader recorded byte positions only for
// <source>/<target> regions that already existed, so a translated
// target had nowhere to be spliced and was silently dropped — exactly
// the parity gap against Okapi's TXMLSkeletonWriter, which always
// regenerates <target> when the TextUnit has one for the output locale
// (TXMLSkeletonWriter.java:167-176).
func TestSkeletonStore_InjectsTargetIntoSourceOnlySegment(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source></segment></translatable>
</txml>
`
	ctx := t.Context()

	reader := txml.NewReader()
	writer := txml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// The source had no <target>; add one for fr, as a pseudo/MT step would.
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			b.Targets[model.LocaleID("fr")] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}}}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><target>Bonjour</target></segment></translatable>
</txml>
`
	assert.Equal(t, expected, buf.String(),
		"a translated target must be injected before </segment> for a source-only segment")
}

// Companion to the injection regression: the new <target> must be
// placed AFTER any trailing <ws> element (the XSD content model is
// (ws?, source, ws? target), so the target is always last, immediately
// before </segment> — TXMLSkeletonWriter.java:135-176). A source-only
// segment with a trailing <ws> must still get the target spliced in the
// correct slot, leaving the <ws> untouched.
func TestSkeletonStore_InjectsTargetAfterTrailingWS(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><ws> </ws></segment></translatable>
</txml>
`
	ctx := t.Context()

	reader := txml.NewReader()
	writer := txml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			b.Targets[model.LocaleID("fr")] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}}}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><ws> </ws><target>Bonjour</target></segment></translatable>
</txml>
`
	assert.Equal(t, expected, buf.String(),
		"the injected target must follow a trailing <ws>, just before </segment>")
}

func TestSkeletonStore_WithTranslation_Escaping(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Hello</source><target>Bonjour</target></segment></translatable>
</txml>
`
	ctx := t.Context()

	reader := txml.NewReader()
	writer := txml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set a translation with XML special characters.
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			b.Targets[model.LocaleID("fr")] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "A & B < C"}}}}}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Contains(t, buf.String(), "<target>A &amp; B &lt; C</target>")
}

func TestSkeletonStore_ByteExact_XmlEntities(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>A &amp; B &lt; C</source></segment></translatable>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TXML with XML entities should be byte-exact")
}

// okapi: TXMLFilterTest#testOutputWithCommentedOutSegments
// On rewrite, XML-commented-out <segment>s are left untouched in the
// skeleton between live segments — they are never extracted as Blocks
// and never dropped. Okapi rewrites the live segment with a gtmt
// attribute it does not have; neokapi's native writer preserves the
// surrounding skeleton (including the comments) byte-exactly and only
// re-splices the live <source>/<target> content, so an unmodified
// roundtrip is byte-identical and the commented segments remain.
func TestOutputPreservesCommentedSegments(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><!--<segment segmentId="s1" modified="true"><source>Segment one</source><target>Segment un</target></segment>--><!--<segment segmentId="s1bis" modified="true"><source>Segment one bis</source><target>Segment un bis</target></segment>--><segment segmentId="2"><source>segment two</source><target>segment deux</target></segment><!--<segment segmentId="3"><source>segment two</source><target>segment deux</target></segment>--></translatable>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "commented-out segments must be preserved verbatim on rewrite")
	// The single live segment is the only extracted Block.
	assert.Contains(t, output, "<!--<segment segmentId=\"s1\"")
	assert.Contains(t, output, "<!--<segment segmentId=\"s1bis\"")
	assert.Contains(t, output, "<!--<segment segmentId=\"3\"")
	assert.Contains(t, output, "<segment segmentId=\"2\"><source>segment two</source><target>segment deux</target></segment>")
}

// Regression for the inline-code writeback bug fixed in renderTXMLInline:
// a <source>/<target> region that contains <ut> inline codes must be
// re-emitted as <ut x type>...</ut> on the skeleton write path, not
// flattened to its (re-escaped) inner data. Without the fix the codes
// collapse into plain text and a read → write → read cycle loses them.
func TestRoundTripPreservesInlineCodes(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<txml locale="en" version="1.0" datatype="regexp" targetlocale="fr">
<translatable blockId="b1" datatype="html"><segment segmentId="s1"><source>Text in <ut x="1" type="bold">&lt;b&gt;</ut>bold<ut x="2" type="bold">&lt;/b&gt;</ut></source><target>Texte en <ut x="1" type="bold">&lt;b&gt;</ut>gras<ut x="2" type="bold">&lt;/b&gt;</ut></target></segment></translatable>
</txml>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "inline <ut> codes must survive an unmodified roundtrip byte-for-byte")
}

// okapi: TXMLFilterTest#testDoubleExtraction
// okapi: TxmlXliffCompareIT#txmlXliffCompareFiles
// Okapi's RoundTripComparison extracts each fixture, writes it, re-extracts
// the output, and asserts the two event streams match. The native
// equivalent here reads each real Wordfast Pro fixture (Test01.docx.txml,
// Test02.html.txml, Test03.mif.txml — the same three files Okapi uses),
// writes it back through the skeleton store, re-reads, and asserts the
// extracted Blocks (segment text and inline-code run structure) are
// identical across the cycle. This exercises <ut> code preservation on
// real corpora. Skipped cleanly when okapi-testdata isn't checked out.
func TestDoubleExtractionFixtures(t *testing.T) {
	fixtures := []string{
		"okapi/filters/txml/src/test/resources/Test01.docx.txml",
		"okapi/filters/txml/src/test/resources/Test02.html.txml",
		"okapi/filters/txml/src/test/resources/Test03.mif.txml",
	}
	for _, rel := range fixtures {
		t.Run(rel, func(t *testing.T) {
			path := findOkapiFixture(t, rel)
			if path == "" {
				t.Skip("okapi-testdata not available")
			}
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			first := extractWithSkeleton(t, string(data))
			rewritten := snippetRoundtripWithSkeleton(t, string(data))
			second := extractWithSkeleton(t, rewritten)

			require.Equal(t, len(first), len(second), "block count must be stable across roundtrip")
			for i := range first {
				require.Equal(t, len(first[i].Source), len(second[i].Source),
					"block %d: source segment count must be stable", i)
				for s := range first[i].Source {
					assert.Equal(t, first[i].Source[s].Text(), second[i].Source[s].Text(),
						"block %d segment %d: source text must be stable", i, s)
					assert.Equal(t, len(first[i].Source[s].Runs), len(second[i].Source[s].Runs),
						"block %d segment %d: inline-code run structure must be stable", i, s)
				}
			}
		})
	}
}

// extractWithSkeleton reads a TXML document through the skeleton store
// and returns its translatable Blocks.
func extractWithSkeleton(t *testing.T, input string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := txml.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(ctx))
}
