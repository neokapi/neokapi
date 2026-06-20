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
}

var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.OriginalContentSetter = (*Writer)(nil)
var _ format.SourcePathSetter = (*Writer)(nil)
var _ format.SubfilterAware = (*Writer)(nil)

// NewWriter creates a new EPUB writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName:       "epub",
			RequiresSkeleton: true,
		},
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

// writeChildLayer collects parts until the matching PartLayerEnd and writes them
// through the appropriate sub-format writer.
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
			// Read original content
			rc, err := file.Open()
			if err != nil {
				return err
			}
			origContent, err := io.ReadAll(rc)
			rc.Close()
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

// replaceXHTMLText replaces translatable text in XHTML content with translations.
func replaceXHTMLText(content []byte, blocks []*model.Block, locale model.LocaleID) []byte {
	// Build a map from source text to translated text
	replacements := make(map[string]string)
	for _, block := range blocks {
		sourceText := block.SourceText()
		targetText := sourceText
		if !locale.IsEmpty() && block.HasTarget(locale) {
			targetText = block.TargetText(locale)
		}
		replacements[sourceText] = targetText
	}

	// Parse and rebuild XHTML
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	var result bytes.Buffer
	encoder := xml.NewEncoder(&result)

	blockElements := map[string]bool{
		"p": true, "h1": true, "h2": true, "h3": true,
		"h4": true, "h5": true, "h6": true, "li": true,
		"dt": true, "dd": true, "th": true, "td": true,
		"figcaption": true, "caption": true, "summary": true,
		"blockquote": true, "title": true,
	}

	var textBuf strings.Builder
	inBlock := false
	depth := 0
	var pendingTokens []xml.Token

	flushBlock := func() {
		if textBuf.Len() > 0 {
			text := strings.TrimSpace(textBuf.String())
			if replacement, ok := replacements[text]; ok {
				// Replace all pending char data tokens with the replacement
				var newTokens []xml.Token
				replaced := false
				for _, tok := range pendingTokens {
					if _, isCharData := tok.(xml.CharData); isCharData && !replaced {
						newTokens = append(newTokens, xml.CharData(replacement))
						replaced = true
					} else if _, isCharData := tok.(xml.CharData); !isCharData {
						newTokens = append(newTokens, tok)
					}
				}
				for _, tok := range newTokens {
					_ = encoder.EncodeToken(tok)
				}
			} else {
				for _, tok := range pendingTokens {
					_ = encoder.EncodeToken(tok)
				}
			}
			textBuf.Reset()
			pendingTokens = nil
		} else {
			for _, tok := range pendingTokens {
				_ = encoder.EncodeToken(tok)
			}
			pendingTokens = nil
		}
	}

	for {
		tok, err := decoder.Token()
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
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else if inBlock {
				depth++
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else {
				_ = encoder.EncodeToken(xml.CopyToken(t))
			}
		case xml.EndElement:
			if blockElements[t.Name.Local] {
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
				flushBlock()
				depth--
				if depth <= 0 {
					inBlock = false
					depth = 0
				}
			} else if inBlock {
				depth--
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else {
				_ = encoder.EncodeToken(xml.CopyToken(t))
			}
		case xml.CharData:
			if inBlock {
				textBuf.Write(t)
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else {
				_ = encoder.EncodeToken(xml.CopyToken(t))
			}
		case xml.ProcInst:
			_ = encoder.EncodeToken(xml.CopyToken(t))
		case xml.Comment:
			_ = encoder.EncodeToken(xml.CopyToken(t))
		case xml.Directive:
			_ = encoder.EncodeToken(xml.CopyToken(t))
		}
	}

	flushBlock()
	encoder.Flush()

	return result.Bytes()
}
