package vignette_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/vignette"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vignetteSkeletonRoundtrip reads input through the vignette reader
// (no translation, no edits) and writes it back through the writer
// using a SkeletonStore. Returns the writer's output for caller
// comparison against the input.
func vignetteSkeletonRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := vignette.NewReader()
	writer := vignette.NewWriter()

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

func TestSkeletonStore_ByteExact_EmptyProject(t *testing.T) {
	output := vignetteSkeletonRoundtrip(t, minimalEmptyDoc)
	assert.Equal(t, minimalEmptyDoc, output, "empty project roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_PlainBilingualPair(t *testing.T) {
	// With useCDATA=true (default) and the source-side payload "hello"
	// being plain text, the writer re-emits `<![CDATA[hello]]>` in place
	// of the original `hello`. The rest of the document (envelope, other
	// instance, attribute names, locale tags) is byte-exact via skeleton.
	output := vignetteSkeletonRoundtrip(t, plainBilingualPair)
	assert.Contains(t, output, "<![CDATA[hello]]>")
	assert.Contains(t, output, "bonjour", "non-extracted instance payload preserved verbatim")
	assert.Contains(t, output, `xmlns="http://www.vignette.com/xmlschemas/importexport"`)
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := vignette.NewReader()
	writer := vignette.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(plainBilingualPair, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.SourceText() == "hello" {
			b.SetTargetText(locale, "salut")
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "<![CDATA[salut]]>")
	assert.NotContains(t, output, "<![CDATA[hello]]>", "source text should be replaced by translation")
	// The non-source-locale instance payload round-trips verbatim: with
	// ExtractNonTranslatableContent ON (default) it rides a skeleton ref as a
	// non-translatable content block whose literal source bytes are written
	// back unchanged (no MT target on a non-translatable block).
	assert.Contains(t, output, "bonjour")
}

func TestSkeletonStore_RealisticHTMLPayloadWritesValidXML(t *testing.T) {
	output := vignetteSkeletonRoundtrip(t, simpleBilingualPair)
	// "ENtext" is the decoded source-side payload; write-side re-wraps
	// it in <p> and CDATA-escapes the result for embedding in valueCLOB.
	assert.Contains(t, output, "<![CDATA[<p>ENtext</p>]]>")
	// The envelope is preserved.
	assert.Contains(t, output, "<importContentInstance>")
	assert.Contains(t, output, "</packageBody>")
}

func TestSkeletonStore_NoSkeleton_FallbackWritesPayloadsOnly(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(plainBilingualPair, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := vignette.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	output := buf.String()
	// Fallback mode: just block payloads, one per line. With
	// ExtractNonTranslatableContent ON (default) the non-source-locale
	// instance ("bonjour") is surfaced as a non-translatable content block,
	// so the fallback writer emits both the translatable source payload
	// ("hello") and the verbatim non-source-locale payload ("bonjour").
	assert.Equal(t, "hello\nbonjour", strings.TrimSpace(output))
}

// okapi-skip: VignetteFilterTest#testSimpleEntryOutput — the native
// writer uses the framework's generic SkeletonStore round-trip: it
// rewrites each extracted Block back into ITS OWN source-side region
// (the en_US instance) and passes every non-referenced region (including
// the es_ES target instance) through verbatim. Okapi's VignetteFilter
// writer, by contrast, is target-locale aware (setOptions(es_ES)): it
// writes the translation into the es_ES target instance as CDATA and the
// source into the en_US instance escaped. The native part/skeleton model
// has no analog of "copy the translation into a sibling target-locale
// instance", so the byte-exact output differs by design rather than by
// bug. The reader-side behavior IS verified by
// TestReadSimpleBilingualPair (==VignetteFilterTest#testSimpleEntry); the
// native writer's actual round-trip contract is asserted below.
//
// TestWriteSimpleEntryOutput is a neokapi-only test pinning the native
// writer's real contract for the simple bilingual document:
//   - the source-side (en_US) okf_html CLOB is re-wrapped in <p> and
//     CDATA-encoded ("<![CDATA[<p>ENtext</p>]]>"),
//   - the non-referenced es_ES instance is preserved byte-for-byte
//     (its original entity-escaped "&lt;p&gt;ES&lt;/p&gt;"),
//   - the XML declaration, namespaced packageBody envelope and the
//     interleaved <stuff/> element all round-trip verbatim.
func TestWriteSimpleEntryOutput(t *testing.T) {
	output := vignetteSkeletonRoundtrip(t, simpleBilingualPair)

	// Source-side (en_US) instance: okf_html payload re-wrapped + CDATA.
	assert.Contains(t, output, "<![CDATA[<p>ENtext</p>]]>")
	// Target-side (es_ES) instance: untouched, original escaped payload.
	assert.Contains(t, output, "<valueCLOB>&lt;p&gt;ES&lt;/p&gt;</valueCLOB>")
	// Envelope + interleaved non-instance element preserved verbatim.
	assert.True(t, strings.HasPrefix(output, `<?xml version="1.0" encoding="UTF-8"?>`))
	assert.Contains(t, output, `<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">`)
	assert.Contains(t, output, "<stuff/>", "non-instance element passes through untouched")
	assert.True(t, strings.HasSuffix(output, "</importProject></packageBody>"))
}

// okapi-skip: VignetteFilterTest#testComplexEntryOutput — same writer-model
// divergence as testSimpleEntryOutput: the native skeleton writer rewrites
// each Block into its own source-side (en_US) region and passes the es_ES
// target instances through verbatim, whereas Okapi writes the translation
// into the target-locale instances. The byte-exact output therefore
// differs by design. Reader ordering/extraction IS verified by
// TestReadComplexTwoPairs (==VignetteFilterTest#testComplexEntry); the
// native writer's actual contract is asserted below.
//
// TestWriteComplexEntryOutput is a neokapi-only test pinning the native
// writer's real contract for the complex document: both en_US source-side
// CLOBs are re-emitted as CDATA in place; both es_ES instances pass
// through with their original plain payloads ("ES-id1", "ES-id2").
func TestWriteComplexEntryOutput(t *testing.T) {
	output := vignetteSkeletonRoundtrip(t, complexTwoPair)

	// Source-side (en_US) instances re-emitted as CDATA.
	assert.Contains(t, output, "<![CDATA[EN-id1]]>")
	assert.Contains(t, output, "<![CDATA[EN-id2]]>")
	// Target-side (es_ES) instances preserved verbatim.
	assert.Contains(t, output, "<valueCLOB>ES-id1</valueCLOB>")
	assert.Contains(t, output, "<valueCLOB>ES-id2</valueCLOB>")
	// Envelope preserved.
	assert.Contains(t, output, `<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">`)
	assert.True(t, strings.HasSuffix(output, "</importProject></packageBody>"))
}

// okapi: VignetteFilterTest#testDoubleExtraction
// Upstream runs RoundTripComparison on Test01.xml: extract → write →
// re-extract → assert the two event streams are identical. The native
// equivalent reads Test01.xml through the skeleton-backed reader/writer,
// writes it back, then re-reads the output and asserts the same number of
// Blocks with identical source text (and identical key Block properties).
// This pins the native reader/writer round-trip fidelity on the real
// upstream fixture. Skips cleanly when the okapi-testdata corpus is
// absent.
func TestDoubleExtraction(t *testing.T) {
	root, err := spec.FindOkapiTestdataRoot()
	if err != nil {
		t.Skipf("okapi-testdata not available: %v", err)
	}
	path := filepath.Join(root, "okapi", "filters", "vignette", "src", "test", "resources", "Test01.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("Test01.xml not available: %v", err)
	}
	input := string(data)

	ctx := context.Background()

	// First extraction (with skeleton) + write.
	reader := vignette.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	blocks1 := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks1, "Test01.xml must extract at least one Block")

	writer := vignette.NewWriter()
	writer.SetSkeletonStore(store)
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	// Second extraction from the written output.
	reader2 := vignette.NewReader()
	store2, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store2.Close()
	reader2.SetSkeletonStore(store2)
	require.NoError(t, reader2.Open(ctx, testutil.RawDocFromString(buf.String(), model.LocaleEnglish)))
	blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	// The two extractions must be equivalent.
	require.Len(t, blocks2, len(blocks1), "re-extraction must yield the same number of Blocks")
	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText(),
			"Block %d source text must survive the round-trip", i)
		assert.Equal(t, blocks1[i].Properties["attribute"], blocks2[i].Properties["attribute"],
			"Block %d attribute name must survive the round-trip", i)
		assert.Equal(t, blocks1[i].Properties["localeId"], blocks2[i].Properties["localeId"],
			"Block %d locale id must survive the round-trip", i)
	}
}
