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
// cross-implementation comparison. If normalizeText is nil, the default
// normalizer (whitespace collapse + trim) is used. For markup formats
// (HTML, XML), pass normalizeMarkupBlockText to also decode entities.
//
// blockTexts extracts translatable block texts from parts for comparison.
// Uses renderSourceText so that inline span data (placeholders, codes) is
// included — SourceText() strips span markers, losing content like %s
// format codes that the bridge represents as Span placeholders.
func blockTexts(parts []*model.Part, normalizeText func(string) string) []string {
	if normalizeText == nil {
		normalizeText = normalizeBlockTextDefault
	}
	var texts []string
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok || !b.Translatable {
			continue
		}
		text := normalizeText(renderSourceText(b))
		if text == "" {
			continue
		}
		texts = append(texts, text)
	}
	return texts
}

// renderSourceText renders a block's source including span data.
func renderSourceText(b *model.Block) string {
	var buf strings.Builder
	for _, seg := range b.Source {
		if len(seg.Runs) == 0 {
			continue
		}
		frag := model.RunsToFragment(seg.Runs)
		if !frag.HasSpans() {
			buf.WriteString(frag.CodedText)
			continue
		}
		spanIdx := 0
		for _, r := range frag.CodedText {
			if (r == model.MarkerOpening || r == model.MarkerClosing || r == model.MarkerPlaceholder) && spanIdx < len(frag.Spans) {
				buf.WriteString(frag.Spans[spanIdx].Data)
				spanIdx++
			} else {
				buf.WriteRune(r)
			}
		}
	}
	return buf.String()
}

// blockTextSet builds a concatenated string of all block texts for substring
// comparison. This handles segmentation differences: one implementation may
// split "A B" into two blocks ["A", "B"] while another keeps it as ["A B"].
// By joining all texts and comparing the full content, we verify that no
// translatable content is lost or added, only segmented differently.
func blockTextSet(texts []string) string {
	return strings.Join(texts, " ")
}

// normalizeBlockTextDefault collapses whitespace and trims. Used for formats
// where block text does not contain markup entities (JSON, YAML, Properties, PO, etc.).
func normalizeBlockTextDefault(s string) string {
	return collapseAndTrim(s)
}

// normalizeMarkupBlockText strips XML/HTML tags, decodes entities, then
// collapses whitespace. Used for HTML, XML, and OpenXML formats where
// rendered span data contains markup tags and entity encoding differs.
func normalizeMarkupBlockText(s string) string {
	s = stripXMLTags(s)
	s = html.UnescapeString(s)
	return collapseAndTrim(s)
}

// stripXMLTags removes XML/HTML tags from a string, keeping text content.
func stripXMLTags(s string) string {
	var buf strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
		} else if r == '>' && inTag {
			inTag = false
		} else if !inTag {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// normalizePropertiesBlockText normalizes backslash escapes for comparison.
// Native preserves raw escape sequences (\:, \=, \_) while the bridge
// may unescape or double-escape them.
func normalizePropertiesBlockText(s string) string {
	// Normalize backslash sequences: \\ → \, \: → :, \= → =, \_ → _
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\:`, `:`)
	s = strings.ReplaceAll(s, `\=`, `=`)
	s = strings.ReplaceAll(s, `\_`, `_`)
	return collapseAndTrim(s)
}

func collapseAndTrim(s string) string {
	var buf strings.Builder
	inSpace := false
	for _, r := range s {
		// Treat NBSP (\u00A0) and other Unicode spaces as regular whitespace
		// for comparison. Native preserves raw bytes; bridge normalizes.
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\u00A0' {
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
