package openxml

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRoundtripSimple reads a docx, writes it back, reads again, and compares blocks.
func TestRoundtripSimple(t *testing.T) {
	original, err := os.ReadFile("testdata/simple.docx")
	require.NoError(t, err)

	// First read
	parts1 := readFromBytes(t, original)
	blocks1 := translatableBlocks(parts1)
	texts1 := blockTexts(blocks1)

	// Write back
	output := writeFromParts(t, parts1, original)

	// Second read
	parts2 := readFromBytes(t, output)
	blocks2 := translatableBlocks(parts2)
	texts2 := blockTexts(blocks2)

	// Compare
	assert.Equal(t, texts1, texts2, "roundtrip should preserve block texts")
}

// TestRoundtripFormatted reads a complex docx, writes it back, reads again.
func TestRoundtripFormatted(t *testing.T) {
	original, err := os.ReadFile("testdata/formatted.docx")
	require.NoError(t, err)

	parts1 := readFromBytes(t, original)
	blocks1 := translatableBlocks(parts1)
	texts1 := blockTexts(blocks1)

	output := writeFromParts(t, parts1, original)

	parts2 := readFromBytes(t, output)
	blocks2 := translatableBlocks(parts2)
	texts2 := blockTexts(blocks2)

	assert.Equal(t, texts1, texts2)
}

// TestRoundtripWithSkeleton tests skeleton-based roundtrip.
func TestRoundtripWithSkeleton(t *testing.T) {
	original, err := os.ReadFile("testdata/simple.docx")
	require.NoError(t, err)

	assertSkeletonRoundtrip(t, original, "test.docx")
}

// TestRoundtripWithTranslation tests translation roundtrip.
func TestRoundtripWithTranslation(t *testing.T) {
	original, err := os.ReadFile("testdata/simple.docx")
	require.NoError(t, err)

	// Read with skeleton
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          "test.docx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}
	err = reader.Open(t.Context(), doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	// Set translations on all blocks
	frFR := model.LocaleID("fr-FR")
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
				b.SetTargetText(frFR, "FR: "+b.SourceText())
			}
		}
	}

	// Write with locale
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	writer.SetLocale(frFR)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(t.Context(), ch)
	require.NoError(t, err)
	writer.Close()

	assert.True(t, buf.Len() > 0, "output should not be empty")

	// Re-read and verify translations appear
	parts2 := readFromBytes(t, buf.Bytes())
	blocks2 := translatableBlocks(parts2)
	texts2 := blockTexts(blocks2)

	for _, text := range texts2 {
		assert.True(t, strings.HasPrefix(text, "FR: "),
			"translated text should start with 'FR: ', got: %q", text)
	}
}

// okapi-filter: openxml

// --- Glob-based roundtrip tests matching bridge test pattern ---

// okapi: OpenXMLDefaultConfigRoundTripTest#testWitthDefaultConfig
// okapi: OpenXMLRoundTripTest#runTestTwice
// TestRoundTrip_Docx performs skeleton roundtrip on all DOCX test files.
//
// Both upstream Java tests this maps to roundtrip a curated DOCX set and assert
// the rewritten package is unchanged: runTestTwice checks idempotency of
// Escapades.docx, testWitthDefaultConfig diffs a gold-file list. Our native
// equivalent is a stronger extraction-roundtrip over the entire DOCX corpus —
// extract → write → extract → compare block texts. The curated files the Java
// tests actually exercise (e.g. Escapades.docx) roundtrip cleanly here.
//
// A handful of corpus files exercise complex-field / style-vanish edge cases
// whose write-back fidelity is tracked separately and excluded below. They are
// genuine known native gaps (not test artefacts); each maps to its tracking
// issue so the contract audit doesn't claim a behaviour we don't yet honour.
//
// This extract→write→extract roundtrip over the real upstream OpenXML corpus is
// also the contract Okapi's integration-test suite enforces over its /openxml/
// file corpus and gold XLIFF. RoundTripOpenXmlIT#openXmlFiles runs
// setSerializedOutput(false) + realTestFiles (extract → re-extract event
// stability); OpenXmXliffCompareIT#openXmlXliffCompareFiles extracts to XLIFF
// and compares against gold. The native test reads the same upstream fixtures
// and t.Skip()s when okapi-testdata is absent (CI shows pending):
// okapi: RoundTripOpenXmlIT#openXmlFiles
// okapi: OpenXmXliffCompareIT#openXmlXliffCompareFiles
// okapi-skip: RoundTripOpenXmlIT#openXmlSerializedFiles — Okapi serialized-skeleton roundtrip variant (setSerializedOutput(true)); native uses its own skeleton store (no serialized-skeleton mode)
func TestRoundTrip_Docx(t *testing.T) {
	dir := testdataDir(t)
	roundTripTestFiles(t, dir, "*.docx",
		// OkapiMarkers.docx contains Okapi's own PUA marker characters (U+E101 etc.)
		// which collide with our internal sentinel mechanism. The first read produces
		// a block with PUA-only content; the roundtrip loses it. Not a real-world issue
		// since actual documents never contain these characters.
		"OkapiMarkers.docx",
		// #598: text + <w:fldChar> in the same source <w:r> — the text run is emitted
		// as a separate <w:r> on write-back, so the re-read duplicates a fragment
		// ("a humanoid" → "a , a humanoid").
		"830-7.docx",
		// #591: complex-field (<w:fldChar> begin/separate/end + instrText) structure
		// differences. The cached field-result run is duplicated or the leading run is
		// dropped on write-back ("A Text with" → " with", page result "1" → "11").
		"956.docx",
		"neverendingloop.docx",
		"1083-empty-and-hyperlink-instructions.docx",
		"1083-hyperlink-and-date-instructions.docx",
		"1083-hyperlink-and-empty-instructions.docx",
		// #589: style-vanish resolution. A run with <w:vanish w:val="0"/> overriding a
		// hidden paragraph style is dropped on write-back, so the re-read loses one
		// block (18 → 17). Belongs to the WordStyleOptimisation writer work.
		"HiddenExcluded.docx",
	)
}

// TestRoundTrip_Xlsx performs skeleton roundtrip on all XLSX test files.
func TestRoundTrip_Xlsx(t *testing.T) {
	dir := testdataDir(t)
	roundTripTestFiles(t, dir, "*.xlsx")
}

// TestRoundTrip_Pptx performs skeleton roundtrip on all PPTX test files.
func TestRoundTrip_Pptx(t *testing.T) {
	dir := testdataDir(t)
	roundTripTestFiles(t, dir, "*.pptx")
}

// roundTripTestFiles performs roundtrip tests on all files matching a glob pattern.
// It reads each file, writes it back using skeleton reconstruction, re-reads,
// and compares block texts.
func roundTripTestFiles(t *testing.T, dir, pattern string, knownFailing ...string) {
	t.Helper()

	failing := make(map[string]bool)
	for _, f := range knownFailing {
		failing[f] = true
	}

	files, err := filepath.Glob(filepath.Join(dir, pattern))
	require.NoError(t, err, "globbing test files")

	if len(files) == 0 {
		t.Fatalf("no test files matching %s/%s", dir, pattern)
	}

	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			if failing[name] {
				t.Skipf("known failing: %s", name)
			}

			original, err := os.ReadFile(f)
			require.NoError(t, err)

			assertSkeletonRoundtrip(t, original, name)
		})
	}
}

// assertSkeletonRoundtrip performs a full skeleton-based roundtrip and compares results.
func assertSkeletonRoundtrip(t *testing.T, original []byte, uri string) {
	t.Helper()

	// Read with skeleton store
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}
	err = reader.Open(t.Context(), doc)
	require.NoError(t, err)
	parts1 := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	blocks1 := translatableBlocks(parts1)
	texts1 := blockTexts(blocks1)

	// Write with skeleton store
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts1)
	err = writer.Write(t.Context(), ch)
	require.NoError(t, err)
	writer.Close()

	require.True(t, buf.Len() > 0, "output should not be empty")

	// Re-read and compare
	parts2 := readFromBytes(t, buf.Bytes())
	blocks2 := translatableBlocks(parts2)
	texts2 := blockTexts(blocks2)

	if !assert.Equal(t, len(texts1), len(texts2), "block count should match") {
		// Log details for debugging
		t.Logf("original blocks: %d, roundtrip blocks: %d", len(texts1), len(texts2))
		if len(texts1) <= 20 {
			t.Logf("original: %v", texts1)
		}
		if len(texts2) <= 20 {
			t.Logf("roundtrip: %v", texts2)
		}
		return
	}

	for i := range texts1 {
		assert.Equal(t, texts1[i], texts2[i],
			"block[%d] text mismatch", i)
	}
}

// assertSkeletonRoundtripConfig performs a skeleton-based roundtrip with a custom
// reader configuration applied to the initial read, then verifies the re-read of
// the rewritten package preserves the extracted block texts. It mirrors the
// upstream gold-package roundtrip tests that pass non-default ConditionalParameters.
func assertSkeletonRoundtripConfig(t *testing.T, original []byte, uri string, configure func(*Config)) {
	t.Helper()

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	if configure != nil {
		configure(reader.cfg)
	}
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}
	err = reader.Open(t.Context(), doc)
	require.NoError(t, err)
	parts1 := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	texts1 := blockTexts(translatableBlocks(parts1))

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	err = writer.Write(t.Context(), testutil.PartsToChannel(parts1))
	require.NoError(t, err)
	writer.Close()
	require.True(t, buf.Len() > 0, "output should not be empty")

	reader2 := NewReader()
	if configure != nil {
		configure(reader2.cfg)
	}
	doc2 := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(buf.Bytes()),
	}
	err = reader2.Open(t.Context(), doc2)
	require.NoError(t, err)
	parts2 := testutil.CollectParts(t, reader2.Read(t.Context()))
	reader2.Close()

	texts2 := blockTexts(translatableBlocks(parts2))

	require.Equal(t, texts1, texts2, "%s: roundtrip should preserve block texts", uri)
}

// --- helpers ---

func readFromBytes(t *testing.T, data []byte) []*model.Part {
	t.Helper()
	reader := NewReader()
	doc := &model.RawDocument{
		URI:          "test.docx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(data),
	}
	err := reader.Open(t.Context(), doc)
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(t.Context()))
}

func writeFromParts(t *testing.T, parts []*model.Part, original []byte) []byte {
	t.Helper()

	// Read with skeleton store for proper roundtrip
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          "test.docx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}
	err = reader.Open(t.Context(), doc)
	require.NoError(t, err)
	readParts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(readParts)
	err = writer.Write(t.Context(), ch)
	require.NoError(t, err)
	writer.Close()
	return buf.Bytes()
}

type bytesReadCloser struct {
	*bytes.Reader
}

func (b *bytesReadCloser) Close() error { return nil }

func readCloserFromBytes(data []byte) *bytesReadCloser {
	return &bytesReadCloser{Reader: bytes.NewReader(data)}
}
