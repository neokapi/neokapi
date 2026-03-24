//go:build integration

package compat

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// nativeRoundTripResult holds both the byte output and extracted parts.
type nativeRoundTripResult struct {
	output []byte
	parts  []*model.Part
}

// nativeRoundTrip reads input through a native format reader then writes it
// back through the corresponding writer using a skeleton store for byte-exact
// reconstruction. The writer is configured with target locale "fr" to match
// the bridge/tikal roundtrip (en → fr).
func nativeRoundTrip(t *testing.T, newReader func() format.DataFormatReader, newWriter func() format.DataFormatWriter, input []byte, uri string) nativeRoundTripResult {
	t.Helper()
	ctx := context.Background()

	reader := newReader()
	writer := newWriter()

	// Wire skeleton store for byte-exact roundtrip.
	store, err := format.NewSkeletonStore()
	require.NoError(t, err, "creating skeleton store")
	defer store.Close()

	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		emitter.SetSkeletonStore(store)
	}
	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
		consumer.SetSkeletonStore(store)
	}
	if setter, ok := writer.(format.OriginalContentSetter); ok {
		setter.SetOriginalContent(input)
	}

	// Set target locale to match bridge/tikal (en → fr identity roundtrip).
	writer.SetLocale("fr")

	// Read.
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(input)),
	}
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error, "reading part")
		parts = append(parts, pr.Part)
	}
	require.NoError(t, reader.Close())

	// Write.
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)

	require.NoError(t, writer.Write(ctx, ch))
	require.NoError(t, writer.Close())

	return nativeRoundTripResult{output: buf.Bytes(), parts: parts}
}

// extractParts reads HTML bytes through a native reader (no skeleton store)
// and returns the extracted parts. Used to re-read bridge/tikal output for
// event-level comparison.
func extractParts(t *testing.T, newReader func() format.DataFormatReader, content []byte, uri string) []*model.Part {
	t.Helper()
	ctx := context.Background()

	reader := newReader()
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error, "extracting parts")
		parts = append(parts, pr.Part)
	}
	require.NoError(t, reader.Close())
	return parts
}

// blockTexts extracts translatable block texts from parts, normalized for
// cross-implementation comparison. Whitespace is collapsed to single spaces
// and trimmed; empty blocks are skipped. HTML entities are decoded.
func blockTexts(parts []*model.Part) []string {
	var texts []string
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok || !b.Translatable {
			continue
		}
		text := normalizeBlockText(b.SourceText())
		if text == "" {
			continue
		}
		texts = append(texts, text)
	}
	return texts
}

// blockTextSet builds a concatenated string of all block texts for substring
// comparison. This handles segmentation differences: one implementation may
// split "A B" into two blocks ["A", "B"] while another keeps it as ["A B"].
// By joining all texts and comparing the full content, we verify that no
// translatable content is lost or added, only segmented differently.
func blockTextSet(texts []string) string {
	return strings.Join(texts, " ")
}

// normalizeBlockText normalizes a block's text for cross-implementation
// comparison: decode HTML entities, collapse whitespace, trim.
func normalizeBlockText(s string) string {
	// Decode HTML entities (&lt; → <, &copy; → ©, etc.)
	s = html.UnescapeString(s)

	// Collapse whitespace and trim.
	var buf strings.Builder
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
			if !inSpace && buf.Len() > 0 {
				buf.WriteByte(' ')
				inSpace = true
			}
		} else {
			buf.WriteRune(r)
			inSpace = false
		}
	}
	return strings.TrimSpace(buf.String())
}
