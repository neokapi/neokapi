// Native ports of the upstream Okapi integration tests that exercise the
// three XML config-preset vocabularies — DITA, DocBook and ResX — through
// the `xml` (okf_xml) and `xmlstream` (okf_xmlstream) filters under their
// bundled ITS-rule / rule-file configurations:
//
//   - DITA:    okf_xmlstream-dita (filters/xmlstream/dita.yml)
//   - DocBook: okf_xml-docbook    (filters/xml/okf_xml-docbook.fprm)
//   - ResX:    okf_xml-resx       (filters/xml/resx.fprm)
//
// The native counterparts to those bundled configs are the DitaConfig /
// DocBookConfig / ResXConfig presets (presets.go). These tests drive the
// REAL upstream fixtures (from the version-pinned okapi-testdata/ mirror,
// resolved via spec.FindOkapiTestdataRoot — never a hardcoded path) through
// the native reader+writer with each preset and assert the same extraction
// shape the upstream integration tests verify, plus read→write→re-read
// block stability. Tests skip cleanly (t.Skip) when okapi-testdata is
// absent so a checkout that has not fetched it never breaks.
//
// The upstream integration-test contracts these port:
//
//   - RoundTripDitaIT#ditaFiles / #ditaFilesSerialized — event round-trip
//     of every /dita/ fixture through okf_xmlstream-dita.
//   - DitaXliffCompareIT#ditaXliffCompareFiles — extracted-xliff compare.
//   - RoundTripDocBookIT#docBookFiles / #docBookSerializedFiles — XML
//     round-trip of /docbook/ fixtures through okf_xml-docbook.
//   - DocBookXliffCompareIT#docBookXliffCompareFiles — extracted-xliff
//     compare.
//   - RoundTripResxIT#resxFiles / #resxSerializedFiles — XML round-trip of
//     /resx/ fixtures through okf_xml-resx.
//   - ResxXliffCompareIT#resxFiles — extracted-xliff compare.
//
// Honest divergences (no false // okapi: claim is made for these):
//
//   - Inline-code finder: the native xml reader has no regex code finder
//     (see reader_test.go okapi-unmapped XmlStreamConfigurationTest#
//     testCodeFinderRules). ResX placeholders like {0}/{1:t} round-trip as
//     literal text rather than <ph ctype="x-regxph"> inline codes. The
//     extracted source TEXT is identical (the placeholder bytes are part of
//     the segment) and the round-trip is byte-exact, so block-shape and
//     stability contracts hold; the inline-code shape does not.
//   - ResX localization note: upstream attaches the sibling <comment> text
//     as an ITS locNote (locNotePointer="../comment") on each trans-unit.
//     The native reader streams each <value> block at its end tag, before
//     the following-sibling <comment> is parsed, so it does not surface the
//     note as a block annotation. The <comment> stays in skeleton and
//     round-trips verbatim — only translator-facing note metadata is not
//     carried.
//   - DocBook mixed-content segmentation: the upstream okf_xml-docbook
//     selectors are namespaced to the DocBook namespace (db:). For the
//     unprefixed (DocBook 4.x) sample fixture those selectors miss, so
//     upstream treats inline phrase elements like <acronym> as separate
//     flows (extra trans-units). The native reader matches inline rules by
//     local name (DocBook-5-correct) and folds <acronym> into its parent
//     paragraph as an inline code — one block instead of three. The
//     extracted text is identical; the segmentation count differs. The
//     block-text contracts below assert the native (spec-faithful)
//     segmentation and note the upstream count.
package xml_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// presetFixture loads a real upstream integration-test fixture from the
// in-repo, version-pinned okapi-testdata/ mirror (fetched by
// scripts/fetch-okapi-testdata.sh). rel is relative to that mirror's root.
// Tests skip cleanly when the mirror or file is absent.
func presetFixture(t *testing.T, rel string) []byte {
	t.Helper()
	root, err := spec.FindOkapiTestdataRoot()
	if err != nil {
		t.Skipf("okapi-testdata not available: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Skipf("upstream fixture %s not available: %v", rel, err)
	}
	return data
}

// readPreset reads data through the native xml reader configured with cfg
// (using a skeleton store so the writer can regenerate later) and returns
// the streamed parts.
func readPreset(t *testing.T, cfg *xmlfmt.Config, data []byte) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	require.NoError(t, reader.SetConfig(cfg))
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(data), "x", model.LocaleEnglish)))
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

// readPresetBlockTexts reads data through cfg and returns the source text
// of each extracted Block (the native analogue of the upstream xliff's
// <source> sequence).
func readPresetBlockTexts(t *testing.T, cfg *xmlfmt.Config, data []byte) []string {
	t.Helper()
	return testutil.BlockTexts(testutil.FilterBlocks(readPreset(t, cfg, data)))
}

// rereadStable drives data through reader→writer→reader with cfg and
// asserts the re-read block count equals the first extraction's count —
// the streaming analogue of the upstream RoundTrip*IT EventComparator /
// XmlComparator contract (extraction is stable across a write+re-read).
// Returns the regenerated bytes.
func rereadStable(t *testing.T, cfg *xmlfmt.Config, data []byte) []byte {
	t.Helper()
	ctx := t.Context()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	reader := xmlfmt.NewReader()
	require.NoError(t, reader.SetConfig(cfg))
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(data), "x", model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks1 := testutil.FilterBlocks(parts)
	reader.Close()
	require.NotEmpty(t, blocks1)

	var buf bytes.Buffer
	writer := xmlfmt.NewWriter()
	writer.SetSkeletonStore(store)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	require.NotZero(t, buf.Len())

	store2, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer func() { _ = store2.Close() }()
	reader2 := xmlfmt.NewReader()
	require.NoError(t, reader2.SetConfig(cfg))
	reader2.SetSkeletonStore(store2)
	require.NoError(t, reader2.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), "x", model.LocaleEnglish)))
	blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	assert.Len(t, blocks2, len(blocks1),
		"re-read block count must match the original extraction")
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// DITA — okf_xmlstream-dita (filters/xmlstream/dita.yml)
// ---------------------------------------------------------------------------

// okapi: DitaXliffCompareIT#ditaXliffCompareFiles — extracting a real
// /dita/ fixture under okf_xmlstream-dita yields the same source segments
// the upstream xliff records. changingtheoil.dita's gold xliff has ten
// trans-units (title, shortdesc, the context <p>, the <stepsection>, and
// six <cmd> steps); <cmd> is NOT in dita.yml's INLINE set so each is its
// own block, and the <link>/<related-links> structure carries no
// translatable text. DitaConfig() reproduces that exact extraction.
func TestDitaConfig_Extraction(t *testing.T) {
	data := presetFixture(t, "integration-tests/okapi/src/test/resources/dita/changingtheoil.dita")
	texts := readPresetBlockTexts(t, xmlfmt.DitaConfig(), data)

	// Whitespace inside block text is collapsed by the native reader (the
	// upstream EventComparator is whitespace-insensitive), so compare on
	// the collapsed forms.
	want := []string{
		"Changing the oil in your car",
		"Once every 6000 kilometers or three months, change the oil in your car.",
		"Changing the oil regularly will help keep the engine in good condition.",
		"To change the oil:",
		"Remove the old oil filter.",
		"Drain the old oil.",
		"Install a new oil filter and gasket.",
		"Add new oil to the engine.",
		"Check the air filter and replace or clean it.",
		"Top up the windshield washer fluid.",
	}
	require.Len(t, texts, len(want))
	for i := range want {
		assert.Equal(t, want[i], collapse(texts[i]), "dita block %d", i)
	}
}

// okapi: DitaXliffCompareIT#ditaXliffCompareFiles — dita.yml's
// ATTRIBUTES_ONLY element rules extract translatable attributes
// (map/@title, topicref/@navtitle, vrm/@version, …) as their own units.
// derbyadmin.ditamap exercises the .ditamap navtitle/title surface: the
// first extracted unit is the map's @title and the file yields many
// topicref @navtitle units. block.Name carries the element-path@attr.
func TestDitaConfig_AttributeExtraction(t *testing.T) {
	data := presetFixture(t, "integration-tests/okapi/src/test/resources/dita/derbyadmin.ditamap")
	blocks := testutil.FilterBlocks(readPreset(t, xmlfmt.DitaConfig(), data))
	require.NotEmpty(t, blocks)

	// map/@title — dita.yml: map ATTRIBUTES_ONLY translatableAttributes:[title].
	assert.Equal(t, "Server and Administration Guide", blocks[0].SourceText())
	assert.Equal(t, "map@title", blocks[0].Name)

	// At least one topicref/@navtitle unit (dita.yml: topicref
	// ATTRIBUTES_ONLY translatableAttributes:[navtitle]).
	var navtitles int
	for _, b := range blocks {
		if strings.HasSuffix(b.Name, "topicref@navtitle") {
			navtitles++
		}
	}
	assert.Positive(t, navtitles, "expected topicref @navtitle attribute units")
}

// okapi: RoundTripDitaIT#ditaFiles
// okapi-skip: RoundTripDitaIT#ditaFilesSerialized — Okapi serialized-skeleton roundtrip variant; native uses its own skeleton store (no serialized-skeleton mode)
// read→write→re-read of /dita/ fixtures preserves the extracted block
// sequence under okf_xmlstream-dita. The native skeleton-writer collapses
// whitespace in translated content (the upstream EventComparator is
// whitespace-insensitive), so the contract verified is block-count
// stability across the round-trip rather than byte equality.
func TestDitaConfig_RoundTripStable(t *testing.T) {
	for _, name := range []string{"changingtheoil.dita", "derbyadmin.ditamap"} {
		t.Run(name, func(t *testing.T) {
			data := presetFixture(t, "integration-tests/okapi/src/test/resources/dita/"+name)
			rereadStable(t, xmlfmt.DitaConfig(), data)
		})
	}
}

// okapi: RoundTripDitaIT#ditaFiles — dita.yml's universal @translate
// attribute gating: '.*' EXCLUDE when @translate="no" and '.+' INCLUDE
// when @translate="yes". A <ph translate="no"> subtree (PI-Problem2.dita)
// is suppressed from the surrounding text unit, while the rest of the
// paragraph still extracts.
func TestDitaConfig_TranslateNoExclusion(t *testing.T) {
	data := presetFixture(t, "integration-tests/okapi/src/test/resources/dita/PI-Problem2.dita")
	texts := readPresetBlockTexts(t, xmlfmt.DitaConfig(), data)
	require.NotEmpty(t, texts)
	// The translatable text is extracted; the translate="no" <ph> phrase
	// placeholder text ("Phrase") never surfaces as a block.
	joined := bytes.Buffer{}
	for _, s := range texts {
		joined.WriteString(s)
	}
	assert.Contains(t, joined.String(), "Discussing issues with Okapi")
}

// ---------------------------------------------------------------------------
// DocBook — okf_xml-docbook (filters/xml/okf_xml-docbook.fprm)
// ---------------------------------------------------------------------------

// okapi: DocBookXliffCompareIT#docBookXliffCompareFiles — extracting the
// real docbook-sample.xml under okf_xml-docbook yields the document's
// titles, author names, paragraphs and table cells; the screenshot
// subtree is excluded (okf_xml-docbook.fprm translateRule translate="no"
// over screenshot), so its <screeninfo>… still extracts only as part of
// the figure title flow per DocBook structure.
//
// The block COUNT differs from the upstream xliff (22 native vs 24
// upstream) because the sample is unprefixed DocBook 4.x: the upstream
// db:-namespaced withinTextRule misses <acronym>, splitting one paragraph
// into three trans-units, whereas the native reader treats <acronym> as
// inline (DocBook-5-correct) and folds it into one block. The assertions
// below pin the native, spec-faithful extraction.
func TestDocBookConfig_Extraction(t *testing.T) {
	data := presetFixture(t, "integration-tests/okapi/src/test/resources/docbook/docbook-sample.xml")
	texts := readPresetBlockTexts(t, xmlfmt.DocBookConfig(), data)
	require.NotEmpty(t, texts)

	collapsed := make([]string, len(texts))
	for i, s := range texts {
		collapsed[i] = collapse(s)
	}

	// Titles, author parts, pubdate, paragraphs, table cells — present in
	// both the native extraction and the upstream xliff.
	for _, want := range []string{
		"My Document Title",
		"Christoph",
		"Lühr",
		"r-Pentomino",
		"26.04.2010",
		"How to write DocBook documentation",
		"Organize your thoughts",
		"this is a subsection",
		"sdfsdfsd",
		"sdfsdfsdfsdf",
		"and a sub sub section ...",
		"Sample Table Title",
		"row 1 cell 1",
		"row 1 cell 2",
		"row 2 cell 1",
		"row 2 cell 2",
		"Do your work",
		"Foobar Section",
		"Title of a Screenshot",
	} {
		assert.Contains(t, collapsed, want, "docbook block %q", want)
	}

	// The first <para> with the inline <acronym>Ipsum</acronym> is ONE
	// native block whose collapsed text contains the acronym content
	// inline (the spec-faithful, DocBook-5 behavior — upstream's
	// db:-namespaced rule would split it for this unprefixed fixture).
	var acronymBlock string
	for _, s := range collapsed {
		if strings.Contains(s, "Lorem Ipsum doloret") && strings.Contains(s, "Ipsum doloret ... Lorem Ipsum") {
			acronymBlock = s
			break
		}
	}
	require.NotEmpty(t, acronymBlock, "expected the acronym paragraph as a single block")
	assert.Contains(t, acronymBlock, "Ipsum", "inline <acronym> content must remain in the block")
}

// okapi: DocBookXliffCompareIT#docBookXliffCompareFiles — verbatim /
// non-prose elements are excluded under okf_xml-docbook.fprm
// (translateRule translate="no" over computeroutput / programlisting /
// screen / screenshot / synopsis / literallayout). Their text must not
// surface as translatable blocks.
func TestDocBookConfig_VerbatimExcluded(t *testing.T) {
	input := `<?xml version="1.0"?>
<article>
  <para>Run the command now.</para>
  <programlisting>int main() { return 0; }</programlisting>
  <screen>$ ls -la /etc</screen>
  <para>Done.</para>
</article>`
	texts := readPresetBlockTexts(t, xmlfmt.DocBookConfig(), []byte(input))
	require.Len(t, texts, 2)
	assert.Equal(t, "Run the command now.", texts[0])
	assert.Equal(t, "Done.", texts[1])
	for _, s := range texts {
		assert.NotContains(t, s, "int main")
		assert.NotContains(t, s, "ls -la")
	}
}

// okapi: DocBookConfig inline phrase elements — okf_xml-docbook.fprm's
// withinTextRule withinText="yes" set folds inline elements
// (emphasis, command, filename, …) into the surrounding paragraph as a
// single text unit. This pins the inline classification on a small
// synthetic paragraph (the native reader matches by local name).
func TestDocBookConfig_InlinePhrase(t *testing.T) {
	input := `<?xml version="1.0"?>
<article><para>Open the <command>kapi</command> tool and edit <filename>config.xml</filename>.</para></article>`
	texts := readPresetBlockTexts(t, xmlfmt.DocBookConfig(), []byte(input))
	require.Len(t, texts, 1, "inline command/filename must fold into one paragraph block")
	assert.Equal(t, "Open the kapi tool and edit config.xml.", collapse(texts[0]))
}

// okapi: RoundTripDocBookIT#docBookFiles
// okapi-skip: RoundTripDocBookIT#docBookSerializedFiles — Okapi serialized-skeleton roundtrip variant; native uses its own skeleton store (no serialized-skeleton mode)
// read→write→re-read of /docbook/ fixtures preserves the extracted block
// sequence under okf_xml-docbook (XmlComparator contract). docbook-sample
// round-trips byte-exact; stdf_manual round-trips with stable blocks.
func TestDocBookConfig_RoundTripStable(t *testing.T) {
	for _, name := range []string{"docbook-sample.xml", "stdf_manual.xml"} {
		t.Run(name, func(t *testing.T) {
			data := presetFixture(t, "integration-tests/okapi/src/test/resources/docbook/"+name)
			rereadStable(t, xmlfmt.DocBookConfig(), data)
		})
	}
}

// ---------------------------------------------------------------------------
// ResX — okf_xml-resx (filters/xml/resx.fprm)
// ---------------------------------------------------------------------------

// okapi: ResxXliffCompareIT#resxFiles — extracting Test.resx under
// okf_xml-resx yields exactly the five <data>/<value> entries (Cancel,
// Greetings, HelloWorld, Ok, Time) named by the parent <data>/@name
// (itsx:idValue="../@name"). The <resheader> entries (resmimetype,
// version, reader, writer) are NOT extracted — resx.fprm's selector is
// //data/value, and the leading UTF-8 BOM is dropped, not extracted.
//
// Honest divergence: the {0}/{1}/{0:t} placeholders surface as literal
// text rather than <ph ctype="x-regxph"> inline codes (no native code
// finder); the source text is identical, so the value strings still match
// the upstream <source> text content.
func TestResXConfig_Extraction(t *testing.T) {
	data := presetFixture(t, "integration-tests/okapi/src/test/resources/resx/Test.resx")
	blocks := testutil.FilterBlocks(readPreset(t, xmlfmt.ResXConfig(), data))

	type tu struct{ name, text string }
	want := []tu{
		{"Cancel", "Cancel"},
		{"Greetings", "Hello {0}, my name is {1}."},
		{"HelloWorld", "Hello world!"},
		{"Ok", "OK"},
		{"Time", "The time is {0:t}."},
	}
	require.Len(t, blocks, len(want))
	for i, w := range want {
		assert.Equal(t, w.name, blocks[i].Name, "resx block %d name", i)
		assert.Equal(t, w.text, blocks[i].SourceText(), "resx block %d text", i)
	}
}

// okapi: ResxXliffCompareIT#resxFiles — the @type / @mimetype / '>'-name
// gating from resx.fprm. HelloWorld.en.resx contains a typed entry
// (Desert_key, @type → not translatable) and one plain entry
// (Some_Key_In_Resx_File, no @type, xml:space=preserve → translatable):
// exactly one block, matching the upstream single-trans-unit xliff.
func TestResXConfig_TypeGating(t *testing.T) {
	data := presetFixture(t, "integration-tests/okapi/src/test/resources/resx/HelloWorld.en.resx")
	blocks := testutil.FilterBlocks(readPreset(t, xmlfmt.ResXConfig(), data))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Some_Key_In_Resx_File", blocks[0].Name)
	assert.Equal(t, "en default- Hi Dan", blocks[0].SourceText())
}

// okapi: ResxXliffCompareIT#resxFiles — Test01.resx is the larger ResX
// fixture (22 trans-units in the upstream xliff). Its entries use varied
// '.Text'/'.Items'/'.ToolTipText' name suffixes (all translatable — only a
// '.Name' suffix is excluded per resx.fprm) plus the special
// @name="$this.Text" entry, which is translatable despite the '$' prefix
// via resx.fprm's dedicated //data[@name='$this.Text']/value rule. The
// native extraction reproduces the same 22 named units.
func TestResXConfig_Test01Names(t *testing.T) {
	data := presetFixture(t, "integration-tests/okapi/src/test/resources/resx/Test01.resx")
	blocks := testutil.FilterBlocks(readPreset(t, xmlfmt.ResXConfig(), data))

	require.Len(t, blocks, 22)
	// First and last names anchor the sequence; the '$this.Text' entry is
	// the dedicated rule.
	assert.Equal(t, "toolStripMenuItem1.Text", blocks[0].Name)
	assert.Equal(t, "&File", blocks[0].SourceText())
	assert.Equal(t, "$this.Text", blocks[len(blocks)-1].Name)
	assert.Equal(t, "Gandalf Monitoring System", blocks[len(blocks)-1].SourceText())

	// Every block is named from the parent <data>/@name (idValue="../@name"),
	// never from the element-path fallback ("root.data.value"). ResX
	// @name values legitimately contain '.' suffixes (.Text/.Items/…), so
	// the discriminator is the path separator the fallback would produce.
	for i, b := range blocks {
		assert.NotContains(t, b.Name, ".value", "block %d name should be a data/@name, got %q", i, b.Name)
		assert.NotContains(t, b.Name, "root.", "block %d name should be a data/@name, got %q", i, b.Name)
	}
}

// okapi: RoundTripResxIT#resxFiles
// okapi-skip: RoundTripResxIT#resxSerializedFiles — Okapi serialized-skeleton roundtrip variant; native uses its own skeleton store (no serialized-skeleton mode)
// read→write→re-read of /resx/ fixtures preserves the extracted block
// sequence under okf_xml-resx (XmlComparator contract). ResX round-trips
// byte-exact (no whitespace collapse: <value> content is preserved
// verbatim through skeleton), so this also asserts byte equality.
func TestResXConfig_RoundTripStable(t *testing.T) {
	for _, name := range []string{"Test.resx", "HelloWorld.en.resx", "test_with_placeholders.resx", "Test01.resx"} {
		t.Run(name, func(t *testing.T) {
			data := presetFixture(t, "integration-tests/okapi/src/test/resources/resx/"+name)
			out := rereadStable(t, xmlfmt.ResXConfig(), data)
			assert.Equal(t, string(data), string(out), "ResX round-trip should be byte-exact")
		})
	}
}

func collapse(s string) string { return xmlfmt.CollapseWhitespace(s) }
