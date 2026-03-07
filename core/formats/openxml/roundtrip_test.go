package openxml

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
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

	// Read with skeleton store
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
	err = reader.Open(context.Background(), doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(context.Background()))
	reader.Close()

	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	require.NotEmpty(t, texts)

	// Write with skeleton store
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(context.Background(), ch)
	require.NoError(t, err)
	writer.Close()

	// Re-read and verify
	parts2 := readFromBytes(t, buf.Bytes())
	blocks2 := translatableBlocks(parts2)
	texts2 := blockTexts(blocks2)

	assert.Equal(t, texts, texts2, "skeleton roundtrip should preserve block texts")
}

// TestRoundtripWithTranslation tests translation roundtrip.
func TestRoundtripWithTranslation(t *testing.T) {
	original, err := os.ReadFile("testdata/simple.docx")
	require.NoError(t, err)

	// Read
	parts1 := readFromBytes(t, original)
	blocks1 := translatableBlocks(parts1)
	require.NotEmpty(t, blocks1)

	// Set translations
	frFR := model.LocaleID("fr-FR")
	for _, b := range blocks1 {
		b.SetTargetText(frFR, "Traduit: "+b.SourceText())
	}

	// Write with locale
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetLocale(frFR)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts1AsPartSlice(parts1))
	err = writer.Write(context.Background(), ch)
	require.NoError(t, err)
	writer.Close()

	assert.True(t, buf.Len() > 0)
}

// helpers

func readFromBytes(t *testing.T, data []byte) []*model.Part {
	t.Helper()
	reader := NewReader()
	doc := &model.RawDocument{
		URI:          "test.docx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(data),
	}
	err := reader.Open(context.Background(), doc)
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(context.Background()))
}

func writeFromParts(t *testing.T, parts []*model.Part, original []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(context.Background(), ch)
	require.NoError(t, err)
	writer.Close()
	return buf.Bytes()
}

func parts1AsPartSlice(parts []*model.Part) []*model.Part {
	return parts
}

type bytesReadCloser struct {
	*bytes.Reader
}

func (b *bytesReadCloser) Close() error { return nil }

func readCloserFromBytes(data []byte) *bytesReadCloser {
	return &bytesReadCloser{Reader: bytes.NewReader(data)}
}
