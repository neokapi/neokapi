package odf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upstreamFixture loads a real upstream Okapi openoffice test fixture
// from the in-repo, version-pinned okapi-testdata/ tree (fetched by
// scripts/fetch-okapi-testdata.sh). Tests skip cleanly when the tree
// is absent so they never break a checkout that hasn't fetched it.
//
// Using the genuine .odt/.ods/.odp packages (rather than hand-built
// minimal ZIPs) is required by #515 — the upstream corpus exercises
// real authoring shapes (page-number fields, bookmark references,
// document metadata) that synthetic fixtures systematically miss, and
// it lets these ports assert byte-for-byte against the same inputs the
// upstream Java tests use.
func upstreamFixture(t *testing.T, name string) []byte {
	t.Helper()
	root, err := spec.FindOkapiTestdataRoot()
	if err != nil {
		t.Skipf("okapi-testdata not available: %v", err)
	}
	path := filepath.Join(root, "okapi/filters/openoffice/src/test/resources", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("upstream fixture %s not available: %v", name, err)
	}
	return data
}

// readUpstreamParts reads a real upstream fixture through the native
// ODF reader and returns the streamed parts.
func readUpstreamParts(t *testing.T, name string) []*model.Part {
	t.Helper()
	data := upstreamFixture(t, name)
	ctx := t.Context()
	reader := odf.NewReader()
	doc := testutil.RawDocFromReader(bytes.NewReader(data), name, model.LocaleEnglish)
	require.NoError(t, reader.Open(ctx, doc))
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

// okapi: ODFFilterTest#testDefaultInfo — reader exposes a non-nil config,
// name and detection signature (Okapi: getParameters/getName/getConfigurations
// non-null and non-empty). The native counterpart to Okapi's filter
// metadata is the reader's Config + Name/DisplayName + Signature.
func TestReaderDefaultInfo(t *testing.T) {
	reader := odf.NewReader()

	// getParameters() != null  → native Config is present.
	cfg := reader.Config()
	require.NotNil(t, cfg)
	assert.Equal(t, "odf", cfg.FormatName())

	// getName() != null  → native reader has a stable name + display name.
	assert.NotEmpty(t, reader.Name())
	assert.NotEmpty(t, reader.DisplayName())

	// getConfigurations() non-null and non-empty  → native exposes a
	// detection signature with MIME types and extensions.
	sig := reader.Signature()
	require.NotEmpty(t, sig.MIMETypes)
	require.NotEmpty(t, sig.Extensions)
}

// okapi: OpenOfficeFilterTest#testStartDocument — the first emitted part is
// the document's root layer carrying the format identity, locale, encoding
// and document-type. Okapi's FilterTestDriver.testStartDocument asserts the
// first event is a START_DOCUMENT with non-null encoding/id/locale/
// linebreak/filterWriter/mimetype; the native streaming equivalent is the
// leading PartLayerStart whose root Layer carries those structural fields.
func TestStartDocumentRootLayer(t *testing.T) {
	parts := readUpstreamParts(t, "TestDocument01.odt")
	require.NotEmpty(t, parts)

	require.Equal(t, model.PartLayerStart, parts[0].Type)
	root, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part must carry the root layer")

	assert.NotEmpty(t, root.ID, "id is null")             // sd.getId()
	assert.NotEmpty(t, root.Format, "format is null")     // sd.getFilterId()/mimetype identity
	assert.False(t, root.Locale.IsEmpty(), "locale null") // sd.getLocale()
	assert.NotEmpty(t, root.Encoding, "encoding is null") // sd.getEncoding()
	assert.Equal(t, "odt", root.Properties["docType"])

	// The last part closes the same root layer (symmetric document frame).
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: ODFFilterTest#testDoubleExtraction — read → write → re-read of a
// real upstream ODF package preserves the extracted block sequence. Okapi's
// ODFFilterTest.testDoubleExtraction round-trips bare content.xml/meta.xml/
// styles.xml fragments through RoundTripComparison; the native ZIP-aware
// reader exercises the same extraction+reconstruction contract over the
// full package (it cannot consume bare inner XML), so we drive the genuine
// .odt/.ods packages and assert the re-read block count is stable.
func TestDoubleExtractionUpstream(t *testing.T) {
	ctx := t.Context()
	for _, name := range []string{"TestDocument01.odt", "TestDocument02.odt", "TestSpreadsheet01.ods"} {
		t.Run(name, func(t *testing.T) {
			data := upstreamFixture(t, name)

			reader := odf.NewReader()
			require.NoError(t, reader.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(data), name, model.LocaleEnglish)))
			parts := testutil.CollectParts(t, reader.Read(ctx))
			blocks1 := testutil.FilterBlocks(parts)
			reader.Close()
			require.NotEmpty(t, blocks1)

			var buf bytes.Buffer
			writer := odf.NewWriter()
			require.NoError(t, writer.SetOutputWriter(&buf))
			writer.SetOriginalContent(data)
			writer.SetLocale(model.LocaleEnglish)
			require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
			require.NoError(t, writer.Close())
			require.NotZero(t, buf.Len())

			reader2 := odf.NewReader()
			require.NoError(t, reader2.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), name, model.LocaleEnglish)))
			blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
			reader2.Close()

			assert.Equal(t, len(blocks1), len(blocks2),
				"re-read block count must match the original extraction")
		})
	}
}

// okapi: OpenOfficeFilterTest#testBookmarkReferencesHandling — with the
// default parameters, a <text:bookmark-ref> with cached inner text is
// preserved verbatim as an opaque inline code (the auto-generated
// reference value is "defined elsewhere" and must not be extracted),
// and the cached '&' is XML-encoded as "&amp;".
//
// Okapi's data provider exercises three rows: default (extractReferences
// off, encodeCharacterEntityReferenceGlyphs on), extractReferences on,
// and encode off. The native reader has no extractReferences /
// encodeCharacterEntityReferenceGlyphs knobs — it always protects
// bookmark-ref subtrees and always XML-encodes their cached text — so
// this maps only the DEFAULT row (the one matching native behavior). The
// fixture's first paragraph yields exactly the same code data the Java
// default row asserts via getCode(1).getData().
func TestBookmarkReferencesHandlingDefault(t *testing.T) {
	parts := readUpstreamParts(t, "bookmark-reference.odt")
	blocks := testutil.FilterBlocks(parts)

	// 3 text units, matching Okapi's textUnits.size() == 3.
	require.Len(t, blocks, 3)

	// First TU holds the two <text:bookmark-ref> codes; the second one
	// (Okapi getCode(1)) carries the cached "1 Heading & title" text with
	// the ampersand XML-encoded, preserved verbatim as an opaque code.
	runs := blocks[0].SourceRuns()
	require.Len(t, runs, 2, "two protected bookmark-ref codes expected")
	require.NotNil(t, runs[1].Ph, "second bookmark-ref must be an opaque placeholder code")
	assert.Equal(t,
		`<text:bookmark-ref text:reference-format="text" text:ref-name="__RefHeading___Toc1166_1491481279">1 Heading &amp; title</text:bookmark-ref>`,
		runs[1].Ph.Data)
}

// okapi: OpenOfficeFilterTest#testMetadataExtraction — document metadata
// (dc:description, dc:subject, dc:title, meta:keyword, meta:user-defined)
// is extracted alongside body and master-page text. Okapi's default
// (extractMetadata=true) row expects exactly ten text units; the native
// reader always extracts these metadata elements (it has no
// extractMetadata=false toggle), so this maps the default row and asserts
// the same set of ten strings is produced.
func TestMetadataExtractionDefault(t *testing.T) {
	parts := readUpstreamParts(t, "TestDocumentWithMetadata.odt")
	texts := testutil.BlockTexts(testutil.FilterBlocks(parts))

	// Same count as Okapi's extractMetadata=true expectation.
	require.Len(t, texts, 10)

	// Body + master-page text.
	assert.Contains(t, texts, "Text on the first page.")
	assert.Contains(t, texts, "Text on the second page.")
	assert.Contains(t, texts, "Author: Test")

	// Document metadata — extracted because metadata extraction is on by
	// default (Okapi's true row; native always extracts these).
	assert.Contains(t, texts, "Test document meta comments")    // dc:description
	assert.Contains(t, texts, "met keywod1")                    // meta:keyword
	assert.Contains(t, texts, "keyword2")                       // meta:keyword
	assert.Contains(t, texts, "Test document meta description") // dc:subject
	assert.Contains(t, texts, "Test document meta title")       // dc:title
	assert.Contains(t, texts, "Test custom property's value")   // meta:user-defined
}

// okapi: OpenOfficeFilterTest#testNumberTag — presentation auto-fields
// (text:page-number, presentation:header/footer/date-time) round-trip as
// opaque inline codes, and a page-number's cached "<number>" sentinel is
// XML-encoded as "&lt;number&gt;". Okapi's data provider toggles
// encodeCharacterEntityReferenceGlyphs; the native reader always encodes,
// so this maps the default (encode=true) row. The .odp fixture yields the
// same nineteen text units, with the page-number code carrying the
// encoded sentinel exactly as the Java default row asserts.
func TestNumberTagDefault(t *testing.T) {
	parts := readUpstreamParts(t, "TestDocumentWithNumberTag.odp")
	blocks := testutil.FilterBlocks(parts)

	// Same count as Okapi's encode=true expectation.
	require.Len(t, blocks, 19)

	// First TU is plain body text.
	assert.Equal(t, "There will be a lot of them", blocks[0].SourceText())

	// Find a bare page-number TU and assert its opaque code data carries
	// the XML-encoded "<number>" sentinel.
	var pageNumberData string
	for _, b := range blocks {
		runs := b.SourceRuns()
		if len(runs) == 1 && runs[0].Ph != nil && runs[0].Ph.Type == "x-page-number" {
			pageNumberData = runs[0].Ph.Data
			break
		}
	}
	require.NotEmpty(t, pageNumberData, "expected a standalone page-number field code")
	assert.Equal(t, "<text:page-number>&lt;number&gt;</text:page-number>", pageNumberData)
}

// okapi-skip: ODFFilterTest#testITSMarkup — ITS metadata (its:translate,
// its:locNote/locNoteType, its:term/termConfidence/annotatorsRef,
// its:localeFilterList/localeFilterType) is parsed by upstream Okapi into
// per-Code GenericAnnotations (TRANSLATE / LOCNOTE / TERM / LOCFILTER).
// The native model has no equivalent ITS annotation layer on inline codes:
// text:span elements carrying ITS attributes are preserved as generic
// inline codes (their markup round-trips verbatim) but the ITS semantics
// are not surfaced as queryable annotations. The fixture is also bare
// content.xml (not a ZIP package), which the native ZIP-aware reader
// cannot open. Not applicable to the native reader/writer until an ITS
// annotation model exists.
