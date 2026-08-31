package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Writer implements DataFormatWriter for EPUB e-book files.
type Writer struct {
	format.BaseFormatWriter
	resolver      format.SubfilterResolver
	skeletonStore *format.SkeletonStore
	// originalContent holds the source archive bytes when handed over via
	// SetOriginalContent. When sourcePath is set instead the source is
	// re-opened from disk, avoiding a full second copy in memory (#608, S2).
	originalContent []byte
	sourcePath      string
	// srcEntries caches the source archive's entry bytes, populated on first
	// use by sourceEntry. A subfiltered entry is reconstructed by the
	// sub-format writer, which needs the original bytes to splice translated
	// text into rather than regenerating the markup from the content model.
	srcEntries map[string][]byte
}

var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.OriginalContentSetter = (*Writer)(nil)
var _ format.SourcePathSetter = (*Writer)(nil)
var _ format.SubfilterAware = (*Writer)(nil)

// NewWriter creates a new EPUB writer.
func NewWriter() *Writer {
	return &Writer{
		FormatName:       "epub",
		RequiresSkeleton: true,
	}
}

// SetSubfilterResolver sets the resolver for creating sub-format writers.
func (w *Writer) SetSubfilterResolver(resolver format.SubfilterResolver) {
	w.resolver = resolver
}

// SetSkeletonStore sets the skeleton store for streaming reconstruction.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// SetOriginalContent provides the original EPUB bytes for roundtrip fidelity.
func (w *Writer) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// SetSourcePath records the path to the original EPUB so reconstruction
// can re-open it from disk instead of holding a full in-memory copy.
// When set it takes precedence over SetOriginalContent (#608, S2).
func (w *Writer) SetSourcePath(path string) {
	w.sourcePath = path
}

// hasSource reports whether a source archive is available (either as held
// bytes or a re-openable path).
func (w *Writer) hasSource() bool {
	return w.sourcePath != "" || w.originalContent != nil
}

// openSource returns a *zip.Reader over the source archive. When a source
// path is set the archive is re-opened from disk (the returned closer
// must be closed by the caller); otherwise the held bytes are used and
// the returned closer is a no-op. Avoids a second full in-memory copy.
func (w *Writer) openSource() (*zip.Reader, func() error, error) {
	if w.sourcePath != "" {
		zrc, err := zip.OpenReader(w.sourcePath)
		if err != nil {
			return nil, nil, fmt.Errorf("epub writer: open source %q: %w", w.sourcePath, err)
		}
		return &zrc.Reader, zrc.Close, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(w.originalContent), int64(len(w.originalContent)))
	if err != nil {
		return nil, nil, fmt.Errorf("epub writer: reading original: %w", err)
	}
	return zr, func() error { return nil }, nil
}

// Write consumes Parts from a channel and writes a reconstructed EPUB.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Collect all blocks keyed by ID
	blocks := make(map[string]*model.Block)
	childLayerValues := make(map[string]string)
	var allParts []*model.Part
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				if w.skeletonStore != nil {
					return w.writeFromSkeleton(blocks, childLayerValues)
				}
				return w.writeEPUB(allParts, childLayerValues)
			}
			if part.Type == model.PartBlock {
				if b, ok := part.Resource.(*model.Block); ok {
					blocks[b.ID] = b
				}
			}
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok && isSubfilteredLayer(layer) {
					val, err := w.writeChildLayer(ctx, layer, parts)
					if err != nil {
						return fmt.Errorf("epub: writing child layer %s: %w", layer.Name, err)
					}
					childLayerValues[layer.Name] = val
					continue
				}
			}
			allParts = append(allParts, part)
		}
	}
}

// isSubfilteredLayer returns true if the layer was created by the subfilter mechanism.
func isSubfilteredLayer(layer *model.Layer) bool {
	if layer.Properties == nil {
		return false
	}
	_, ok := layer.Properties["subfilter.source"]
	return ok
}

// writeChildLayer collects parts until the matching PartLayerEnd and returns the
// reconstructed bytes for the entry that layer covers.
//
// Reconstruction splices translated text into the entry's *original* bytes
// (replaceXHTMLText) rather than re-rendering the Part stream through the
// sub-format writer. The sub-writer only has the content model, so it returns
// normalized markup — a synthesized doctype, the XML declaration rewritten as a
// comment, `<title>` duplicated into the body, indentation gone — and an
// untouched EPUB stops round-tripping byte-for-byte. The splice touches only the
// character data of the blocks that changed, which is the same guarantee the
// non-subfiltered path gives.
//
// The sub-writer stays as the fallback for an entry with no original bytes,
// where generating from the content model is the only option and is correct.
func (w *Writer) writeChildLayer(ctx context.Context, layer *model.Layer, parts <-chan *model.Part) (string, error) {
	var childParts []*model.Part
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return "", fmt.Errorf("unexpected end of parts stream in child layer %s", layer.ID)
			}
			if part.Type == model.PartLayerEnd {
				if endLayer, ok := part.Resource.(*model.Layer); ok && endLayer.ID == layer.ID {
					goto collected
				}
			}
			childParts = append(childParts, part)
		}
	}

collected:
	// Preferred path: splice into the entry's original bytes.
	if orig, ok := w.sourceEntry(layer.Properties["entry"]); ok {
		childBlocks := make([]*model.Block, 0, len(childParts))
		for _, p := range childParts {
			if p.Type != model.PartBlock {
				continue
			}
			if b, ok := p.Resource.(*model.Block); ok {
				childBlocks = append(childBlocks, b)
			}
		}
		return string(replaceXHTMLText(orig, childBlocks, w.Locale)), nil
	}

	if w.resolver == nil {
		return w.fallbackChildText(childParts), nil
	}

	subWriter, err := w.resolver.ResolveWriter(layer.Format)
	if err != nil {
		return w.fallbackChildText(childParts), nil
	}

	var buf bytes.Buffer
	if err := subWriter.SetOutputWriter(&buf); err != nil {
		return "", err
	}
	subWriter.SetLocale(w.Locale)

	childCh := make(chan *model.Part, len(childParts))
	for _, p := range childParts {
		childCh <- p
	}
	close(childCh)

	if err := subWriter.Write(ctx, childCh); err != nil {
		return "", err
	}
	if err := subWriter.Close(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// sourceEntry returns the named entry's bytes from the source archive, reading
// the archive once and caching every entry. Returns false when there is no
// source, or the archive has no such entry — a generated entry with no original
// is written by the sub-writer from the content model, which is correct for it.
func (w *Writer) sourceEntry(name string) ([]byte, bool) {
	if name == "" || !w.hasSource() {
		return nil, false
	}
	if w.srcEntries == nil {
		w.srcEntries = make(map[string][]byte)
		zr, closeSrc, err := w.openSource()
		if err != nil {
			return nil, false
		}
		defer func() { _ = closeSrc() }()
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			// Bounded by the shared zip limits: the writer may re-open the
			// archive from disk and cannot rely on the reader's validation.
			data, err := safeio.DefaultZipLimits.ReadEntry(f)
			if err != nil {
				continue
			}
			w.srcEntries[f.Name] = data
		}
	}
	data, ok := w.srcEntries[name]
	return data, ok
}

// fallbackChildText concatenates block source/target texts when no sub-writer is available.
func (w *Writer) fallbackChildText(parts []*model.Part) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if block, ok := p.Resource.(*model.Block); ok {
				text := block.SourceText()
				if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
					text = block.TargetText(w.Locale)
				}
				sb.WriteString(text)
			}
		}
	}
	return sb.String()
}

// writeFromSkeleton reconstructs translatable XHTML parts using the skeleton store.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block, childLayerValues map[string]string) error {
	if !w.hasSource() {
		return errors.New("epub writer: original content required for reconstruction")
	}

	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("epub writer: skeleton flush: %w", err)
	}

	// Read all skeleton entries, splitting by part-boundary markers
	partContents := make(map[string][]byte)
	var currentPart string
	var currentBuf bytes.Buffer

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("epub writer: reading skeleton: %w", err)
		}

		switch entry.Type {
		case format.SkeletonText:
			if currentPart != "" {
				currentBuf.Write(entry.Data)
			}

		case format.SkeletonRef:
			refID := string(entry.Data)

			// Check for part-boundary markers
			if after, ok := strings.CutPrefix(refID, skelPartStartPrefix); ok {
				currentPart = after
				currentBuf.Reset()
				continue
			}
			if after, ok := strings.CutPrefix(refID, skelPartEndPrefix); ok {
				partPath := after
				if currentBuf.Len() > 0 {
					partContents[partPath] = append([]byte{}, currentBuf.Bytes()...)
				}
				currentPart = ""
				currentBuf.Reset()
				continue
			}

			// Regular block ref or layer ref — render translated text
			if currentPart != "" {
				if strings.HasPrefix(refID, "layer:") {
					layerPath := refID[6:]
					if val, ok := childLayerValues[layerPath]; ok {
						currentBuf.WriteString(val)
					}
				} else if block, ok := blocks[refID]; ok {
					currentBuf.WriteString(w.renderBlockText(block))
				}
			}
		}
	}

	// Open original ZIP for copying structure
	zr, closeSrc, err := w.openSource()
	if err != nil {
		return err
	}
	defer func() { _ = closeSrc() }()

	zw := zip.NewWriter(w.Output)
	defer zw.Close()

	for _, file := range zr.File {
		if content, ok := partContents[file.Name]; ok && len(content) > 0 {
			// Replace with skeleton-reconstructed content
			fh := file.FileHeader
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(content); err != nil {
				return err
			}
		} else {
			// Copy unchanged — use raw copy to preserve CRC/data descriptors
			if err := zw.Copy(file); err != nil {
				return err
			}
		}
	}

	return nil
}

// renderBlockText returns the translated (or source) text for a block.
func (w *Writer) renderBlockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		if runs := block.TargetRuns(w.Locale); len(runs) > 0 {
			return xmlEscape(model.RunsText(runs))
		}
	}
	if len(block.Source) > 0 {
		return xmlEscape(model.RunsText(block.Source))
	}
	return ""
}

func (w *Writer) writeEPUB(parts []*model.Part, childLayerValues map[string]string) error {
	if !w.hasSource() {
		return errors.New("epub writer: original content required for roundtrip")
	}

	// Build map of entry -> translated blocks
	entryBlocks := make(map[string][]*model.Block)
	for _, part := range parts {
		if part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok {
			continue
		}
		entry := block.Properties["entry"]
		if entry == "" {
			continue
		}
		entryBlocks[entry] = append(entryBlocks[entry], block)
	}

	zr, closeSrc, err := w.openSource()
	if err != nil {
		return err
	}
	defer func() { _ = closeSrc() }()

	zw := zip.NewWriter(w.Output)
	defer zw.Close()

	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			_, err := zw.Create(file.Name)
			if err != nil {
				return err
			}
			continue
		}

		// Preserve compression settings via header copy
		header := file.FileHeader
		writer, err := zw.CreateHeader(&header)
		if err != nil {
			return err
		}

		if val, ok := childLayerValues[file.Name]; ok {
			// Write subfiltered content reconstructed through sub-format writer
			if _, err := io.WriteString(writer, val); err != nil {
				return err
			}
		} else if blocks, ok := entryBlocks[file.Name]; ok && len(blocks) > 0 {
			// Read original content, bounded by the shared safeio zip limits
			// (per-entry uncompressed size + inflate-ratio zip-bomb guard on
			// the actual stream) — the writer may re-open the archive from
			// disk, so it cannot rely on the reader's earlier validation.
			origContent, err := safeio.DefaultZipLimits.ReadEntry(file)
			if err != nil {
				return err
			}

			// Replace text in XHTML
			translated := replaceXHTMLText(origContent, blocks, w.Locale)
			if _, err := writer.Write(translated); err != nil {
				return err
			}
		} else {
			// Copy original content
			rc, err := file.Open()
			if err != nil {
				return err
			}
			if _, err := io.Copy(writer, rc); err != nil {
				rc.Close()
				return err
			}
			rc.Close()
		}
	}

	return nil
}

// replaceXHTMLText rewrites an XHTML entry in place: every span of character
// data that belongs to a translatable block is replaced with the text the
// pipeline produced (the target for locale when the block has one, otherwise the
// block's own source), and every other byte of the entry is the original byte.
//
// Two things are load-bearing here.
//
// The lookup key is the text the reader recorded for the block (propXHTMLText) —
// what actually stands in this entry — never the block's current source. Keyed on
// the current source the comparison is tautological: after a source edit the
// needle IS the new text, so it matches nothing in the original entry, the
// replacement no-ops, and the write reports success with the edit missing from
// the file (#1482). Blocks carrying no recorded witness (built by something other
// than this reader) fall back to their source, the best available statement of
// what they replace.
//
// And the entry is spliced, not re-encoded. Round-tripping the token stream
// through encoding/xml re-declared the default namespace on every element
// (`<html xmlns="…" xmlns="…">`, `<p xmlns="…">`) because the decoder resolves
// each name into Name.Space and the encoder re-emits it — invalid XML, produced
// even when nothing had been edited. Splicing preserves the entry byte for byte
// outside the replaced spans, which is the same "keep the original, overwrite
// only what changed" discipline the other package writers follow.
func replaceXHTMLText(content []byte, blocks []*model.Block, locale model.LocaleID) []byte {
	// Build a map from the text as read to the text to write.
	replacements := make(map[string]string)
	for _, block := range blocks {
		asRead, ok := format.VerbatimText(block, propXHTMLText)
		if !ok {
			asRead = block.SourceText()
		}
		targetText := block.SourceText()
		if !locale.IsEmpty() && block.HasTarget(locale) {
			targetText = block.TargetText(locale)
		}
		replacements[strings.TrimSpace(asRead)] = targetText
	}
	if len(replacements) == 0 {
		return content
	}

	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	// blockElements are the containers whose text the reader surfaced as blocks;
	// only their character data is eligible for replacement.
	blockElements := map[string]bool{
		"p": true, "h1": true, "h2": true, "h3": true,
		"h4": true, "h5": true, "h6": true, "li": true,
		"dt": true, "dd": true, "th": true, "td": true,
		"figcaption": true, "caption": true, "summary": true,
		"blockquote": true, "title": true,
	}

	var (
		edits    []byteSplice
		textBuf  strings.Builder
		spans    []byteSpan
		inBlock  bool
		depth    int
		lastRead int64
	)

	// flushBlock decides what happens to the character-data spans collected for
	// one block element. The whole replacement goes into the FIRST span and the
	// rest are dropped, so inline markup between them (an <em>, a <span>) keeps
	// its own bytes and its position.
	flushBlock := func() {
		defer func() {
			textBuf.Reset()
			spans = nil
		}()
		if len(spans) == 0 {
			return
		}
		replacement, ok := replacements[strings.TrimSpace(textBuf.String())]
		if !ok {
			return
		}
		edits = append(edits, byteSplice{byteSpan: spans[0], text: xmlEscape(replacement)})
		for _, s := range spans[1:] {
			edits = append(edits, byteSplice{byteSpan: s})
		}
	}

	for {
		start := lastRead
		tok, err := decoder.Token()
		lastRead = decoder.InputOffset()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if blockElements[t.Name.Local] {
				if inBlock {
					flushBlock()
				}
				inBlock = true
				depth++
			} else if inBlock {
				depth++
			}
		case xml.EndElement:
			if blockElements[t.Name.Local] {
				flushBlock()
				depth--
				if depth <= 0 {
					inBlock = false
					depth = 0
				}
			} else if inBlock {
				depth--
			}
		case xml.CharData:
			if inBlock {
				textBuf.Write(t)
				spans = append(spans, byteSpan{start: start, end: lastRead})
			}
		}
	}
	flushBlock()

	return applySplices(content, edits)
}

// byteSpan is a half-open byte range of the original entry.
type byteSpan struct{ start, end int64 }

// byteSplice replaces a span with text; an empty text deletes the span.
type byteSplice struct {
	byteSpan
	text string
}

// applySplices rebuilds content with each splice applied. The splices are
// produced in document order by a single forward scan, so no sorting is needed;
// an out-of-order or out-of-range one is skipped rather than corrupting the
// output.
func applySplices(content []byte, splices []byteSplice) []byte {
	if len(splices) == 0 {
		return content
	}
	var out bytes.Buffer
	out.Grow(len(content))
	cursor := int64(0)
	for _, s := range splices {
		if s.start < cursor || s.end > int64(len(content)) || s.start > s.end {
			continue
		}
		out.Write(content[cursor:s.start])
		out.WriteString(s.text)
		cursor = s.end
	}
	out.Write(content[cursor:])
	return out.Bytes()
}
