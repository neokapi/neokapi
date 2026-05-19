package odf

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

// Writer implements DataFormatWriter for ODF files.
type Writer struct {
	format.BaseFormatWriter
	resolver        format.SubfilterResolver
	skeletonStore   *format.SkeletonStore
	originalContent []byte
}

var _ format.OriginalContentSetter = (*Writer)(nil)
var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.SubfilterAware = (*Writer)(nil)

// NewWriter creates a new ODF writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "odf",
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

// SetOriginalContent sets the original document bytes for reconstruction.
func (w *Writer) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// Write consumes Parts and writes the reconstructed ODF document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Collect all blocks keyed by ID
	blocks := make(map[string]*model.Block)
	childLayerValues := make(map[string]string)
	for part := range parts {
		switch part.Type {
		case model.PartBlock:
			if b, ok := part.Resource.(*model.Block); ok {
				blocks[b.ID] = b
			}
		case model.PartLayerStart:
			if layer, ok := part.Resource.(*model.Layer); ok && isSubfilteredLayer(layer) {
				val, err := w.writeChildLayer(ctx, layer, parts)
				if err != nil {
					return fmt.Errorf("odf: writing child layer %s: %w", layer.Name, err)
				}
				childLayerValues[layer.Name] = val
			}
		}
	}

	if w.originalContent == nil {
		return errors.New("odf: writer requires original content for reconstruction")
	}

	// Open original ZIP
	origZR, err := zip.NewReader(bytes.NewReader(w.originalContent), int64(len(w.originalContent)))
	if err != nil {
		return fmt.Errorf("odf: invalid original ZIP: %w", err)
	}

	// Create output ZIP
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// If we have a skeleton store, use skeleton-based reconstruction
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("odf: skeleton flush: %w", err)
		}
		if err := w.writeFromSkeleton(origZR, zw, blocks, childLayerValues); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		_, err = w.Output.Write(buf.Bytes())
		return err
	}

	// Fallback: reparse-based reconstruction
	if err := w.writeFromReparse(origZR, zw, blocks, childLayerValues); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	_, err = w.Output.Write(buf.Bytes())
	return err
}

// writeFromSkeleton reconstructs translatable XML parts using the skeleton store.
func (w *Writer) writeFromSkeleton(origZR *zip.Reader, zw *zip.Writer,
	blocks map[string]*model.Block, childLayerValues map[string]string) error {

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
			return fmt.Errorf("odf: reading skeleton: %w", err)
		}

		switch entry.Type {
		case format.SkeletonText:
			if currentPart != "" {
				currentBuf.Write(entry.Data)
			}

		case format.SkeletonRef:
			refID := string(entry.Data)

			// Check for part-boundary markers
			if strings.HasPrefix(refID, skelPartStartPrefix) {
				currentPart = strings.TrimPrefix(refID, skelPartStartPrefix)
				currentBuf.Reset()
				continue
			}
			if strings.HasPrefix(refID, skelPartEndPrefix) {
				partPath := strings.TrimPrefix(refID, skelPartEndPrefix)
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
					currentBuf.WriteString(w.getBlockText(block))
				}
			}
		}
	}

	// Write output ZIP: replace translatable parts with skeleton-reconstructed content
	for _, f := range origZR.File {
		if content, ok := partContents[f.Name]; ok && len(content) > 0 {
			fh := f.FileHeader
			fh.Method = zip.Deflate
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
		} else if f.Name == "mimetype" {
			// mimetype must be stored uncompressed (ODF spec requirement)
			origData, err := readZipFile(f)
			if err != nil {
				return err
			}
			fh := f.FileHeader
			fh.Method = zip.Store
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(origData); err != nil {
				return err
			}
		} else {
			// Copy unchanged
			if err := zw.Copy(f); err != nil {
				return err
			}
		}
	}

	return nil
}

// writeFromReparse copies the original ZIP, replacing translatable content
// in content.xml and styles.xml using XML reparse (fallback without skeleton).
func (w *Writer) writeFromReparse(origZR *zip.Reader, zw *zip.Writer,
	blocks map[string]*model.Block, childLayerValues map[string]string) error {

	for _, f := range origZR.File {
		if val, ok := childLayerValues[f.Name]; ok {
			// Write subfiltered content reconstructed through sub-format writer
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write([]byte(val)); err != nil {
				return err
			}
		} else if f.Name == "content.xml" || f.Name == "styles.xml" || f.Name == "meta.xml" {
			// Replace translatable content in XML files
			origData, err := readZipFile(f)
			if err != nil {
				return fmt.Errorf("odf: reading %s: %w", f.Name, err)
			}

			newData, err := w.replaceContent(origData, blocks)
			if err != nil {
				return fmt.Errorf("odf: replacing content in %s: %w", f.Name, err)
			}

			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(newData); err != nil {
				return err
			}
		} else if f.Name == "mimetype" {
			// mimetype must be stored uncompressed (ODF spec requirement)
			origData, err := readZipFile(f)
			if err != nil {
				return err
			}
			fh := f.FileHeader
			fh.Method = zip.Store
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(origData); err != nil {
				return err
			}
		} else {
			// Copy unchanged
			if err := zw.Copy(f); err != nil {
				return err
			}
		}
	}

	return nil
}

// replaceContent replaces translatable text in an ODF XML document.
// It walks the XML tree and replaces text in translatable elements
// with target text from the collected blocks.
func (w *Writer) replaceContent(data []byte, blocks map[string]*model.Block) ([]byte, error) {
	// Build a block index by matching source text
	blockByText := make(map[string]*model.Block)
	for _, b := range blocks {
		blockByText[b.SourceText()] = b
	}

	d := xml.NewDecoder(bytes.NewReader(data))
	var output bytes.Buffer
	enc := xml.NewEncoder(&output)

	var elementStack []xml.Name
	var textBuf strings.Builder
	var tokenBuf []xml.Token
	inTranslatable := false
	var translatableDepth int

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			elementStack = append(elementStack, t.Name)
			t = t.Copy()

			if isTranslatableElement(t.Name) && !inTranslatable {
				inTranslatable = true
				translatableDepth = len(elementStack)
				textBuf.Reset()
				tokenBuf = []xml.Token{t}
			} else if inTranslatable {
				tokenBuf = append(tokenBuf, t)
				// Skip text collection for inline elements — we collect their CharData
			} else {
				if err := enc.EncodeToken(t); err != nil {
					return nil, err
				}
			}

		case xml.CharData:
			if inTranslatable {
				textBuf.Write(t)
				tokenBuf = append(tokenBuf, t.Copy())
			} else {
				if err := enc.EncodeToken(t.Copy()); err != nil {
					return nil, err
				}
			}

		case xml.EndElement:
			if inTranslatable {
				tokenBuf = append(tokenBuf, t)

				if len(elementStack) == translatableDepth {
					// End of translatable element — check for replacement
					text := strings.TrimSpace(textBuf.String())
					if block, ok := blockByText[text]; ok {
						replacement := w.getBlockText(block)
						// Write the start element
						if err := enc.EncodeToken(tokenBuf[0]); err != nil {
							return nil, err
						}
						// Write replaced text
						if err := enc.EncodeToken(xml.CharData(replacement)); err != nil {
							return nil, err
						}
						// Write the end element
						if err := enc.EncodeToken(t); err != nil {
							return nil, err
						}
					} else {
						// No replacement — write original tokens
						for _, tok := range tokenBuf {
							if err := enc.EncodeToken(tok); err != nil {
								return nil, err
							}
						}
					}
					inTranslatable = false
					tokenBuf = nil
				}
			} else {
				if err := enc.EncodeToken(t); err != nil {
					return nil, err
				}
			}

			if len(elementStack) > 0 {
				elementStack = elementStack[:len(elementStack)-1]
			}

		case xml.ProcInst:
			if inTranslatable {
				tokenBuf = append(tokenBuf, t.Copy())
			} else {
				if err := enc.EncodeToken(t.Copy()); err != nil {
					return nil, err
				}
			}

		case xml.Comment:
			if inTranslatable {
				tokenBuf = append(tokenBuf, t.Copy())
			} else {
				if err := enc.EncodeToken(t.Copy()); err != nil {
					return nil, err
				}
			}

		case xml.Directive:
			if inTranslatable {
				tokenBuf = append(tokenBuf, t.Copy())
			} else {
				if err := enc.EncodeToken(t.Copy()); err != nil {
					return nil, err
				}
			}
		}
	}

	if err := enc.Flush(); err != nil {
		return nil, err
	}

	return output.Bytes(), nil
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
				sb.WriteString(w.getBlockText(block))
			}
		}
	}
	return sb.String()
}

// getBlockText returns the rendered text for a block. When the block
// carries inline-code runs (PcOpen/PcClose pairs captured by the reader
// from elements like <text:span>, <text:script>, <draw:frame>, etc.),
// the runs are serialised via RenderRunsWithData so the original markup
// is spliced back into the reconstructed XML — mirroring upstream Okapi
// ODFFilter's TextFragment-with-codes round-trip. Plain-text-only blocks
// use TargetText/SourceText as before.
func (w *Writer) getBlockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		runs := block.TargetRuns(w.Locale)
		if hasInlineCodeRuns(runs) {
			return model.RenderRunsWithData(runs)
		}
		return block.TargetText(w.Locale)
	}
	runs := block.SourceRuns()
	if hasInlineCodeRuns(runs) {
		return model.RenderRunsWithData(runs)
	}
	return block.SourceText()
}
