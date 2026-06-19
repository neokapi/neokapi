package asciidoc_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/asciidoc"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

// readParts reads input through the AsciiDoc reader (no skeleton store) and
// returns the streamed parts.
func readParts(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	r := asciidoc.NewReader()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer r.Close()
	return testutil.CollectParts(t, r.Read(ctx))
}

// readBlocks returns only the translatable blocks for input.
func readBlocks(t *testing.T, input string) []*model.Block {
	t.Helper()
	return testutil.FilterBlocks(readParts(t, input))
}

// skelRoundtrip reads input with a wired skeleton store and writes it back
// through the same store, returning the reconstructed bytes. With locale empty
// the output is the untouched source projection.
func skelRoundtrip(t *testing.T, input string, locale model.LocaleID) string {
	t.Helper()
	ctx := t.Context()

	reader := asciidoc.NewReader()
	writer := asciidoc.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	if !locale.IsEmpty() {
		writer.SetLocale(locale)
	}
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// origRoundtrip reads input (no skeleton) and writes the parts back through the
// SetOriginalContent path, returning the reconstructed bytes.
func origRoundtrip(t *testing.T, input string, locale model.LocaleID) string {
	t.Helper()
	ctx := t.Context()

	parts := readParts(t, input)

	writer := asciidoc.NewWriter()
	writer.SetOriginalContent([]byte(input))
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	if !locale.IsEmpty() {
		writer.SetLocale(locale)
	}
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// writeOriginalParts writes pre-read (possibly translated) parts back through
// the SetOriginalContent path against the given source, returning the bytes.
func writeOriginalParts(t *testing.T, input string, parts []*model.Part, locale model.LocaleID) string {
	t.Helper()
	ctx := t.Context()
	writer := asciidoc.NewWriter()
	writer.SetOriginalContent([]byte(input))
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	if !locale.IsEmpty() {
		writer.SetLocale(locale)
	}
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// blockByText returns the first block whose source text matches, or fails.
func blockByText(t *testing.T, blocks []*model.Block, text string) *model.Block {
	t.Helper()
	for _, b := range blocks {
		if b.SourceText() == text {
			return b
		}
	}
	t.Fatalf("no block with source text %q (have %v)", text, testutil.BlockTexts(blocks))
	return nil
}
