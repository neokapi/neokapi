package archive

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for ZIP archive files.
type Writer struct {
	format.BaseFormatWriter
	resolver      format.SubfilterResolver
	originalZip   string // path to the original ZIP file (temp file from reader)
	ownsTmpFile   bool   // true if we created a temp file via SetOriginalContent
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer, SourcePathSetter, OriginalContentSetter, and SubfilterAware.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.SourcePathSetter = (*Writer)(nil)
var _ format.OriginalContentSetter = (*Writer)(nil)
var _ format.SubfilterAware = (*Writer)(nil)

// NewWriter creates a new archive writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "archive",
		},
	}
}

// SetSubfilterResolver sets the resolver for creating sub-format writers.
func (w *Writer) SetSubfilterResolver(resolver format.SubfilterResolver) {
	w.resolver = resolver
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// SetOriginalZip provides the path to the original ZIP file for roundtrip fidelity.
// The writer reads binary entries from this file during write.
func (w *Writer) SetOriginalZip(path string) {
	w.originalZip = path
}

// SetSourcePath implements format.SourcePathSetter.
// It sets the path to the original ZIP file, avoiding loading it into memory.
func (w *Writer) SetSourcePath(path string) {
	w.originalZip = path
}

// SetOriginalContent implements format.OriginalContentSetter.
// It writes the content to a temp file and uses that for roundtrip fidelity.
func (w *Writer) SetOriginalContent(content []byte) {
	f, err := os.CreateTemp("", "neokapi-archive-writer-*")
	if err != nil {
		return
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return
	}
	f.Close()
	w.originalZip = f.Name()
	w.ownsTmpFile = true
}

// Write consumes Parts from a channel and writes a reconstructed ZIP archive.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeleton(ctx, parts)
	}

	// Collect all parts to build the output, handling child layers for subfiltered content
	var allParts []*model.Part
	childLayerValues := make(map[string]string)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.writeArchive(allParts, childLayerValues)
			}
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok && isSubfilteredLayer(layer) {
					val, err := w.writeChildLayer(ctx, layer, parts)
					if err != nil {
						return fmt.Errorf("archive: writing child layer %s: %w", layer.Name, err)
					}
					childLayerValues[layer.Name] = val
					continue
				}
			}
			allParts = append(allParts, part)
		}
	}
}

// writeWithSkeleton collects all blocks, then reconstructs output from skeleton entries.
func (w *Writer) writeWithSkeleton(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)
	childLayerValues := make(map[string]string)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			switch part.Type {
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
				}
			case model.PartLayerStart:
				if layer, ok := part.Resource.(*model.Layer); ok && isSubfilteredLayer(layer) {
					val, err := w.writeChildLayer(ctx, layer, parts)
					if err != nil {
						return fmt.Errorf("archive: writing child layer %s: %w", layer.Name, err)
					}
					childLayerValues[layer.Name] = val
				}
			}
		}
	}
done:
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("archive writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID, childLayerValues)
}

// writeFromSkeleton reads skeleton entries and reconstructs the ZIP.
// Skeleton entries encode entry markers (<<ENTRY:name>> for text, <<BINARY:name>> for binary)
// interleaved with block refs. The writer uses the original ZIP for binary entries.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block, childLayerValues map[string]string) error {
	// Open original ZIP for binary entry copying
	var origZip *zip.ReadCloser
	if w.originalZip != "" {
		var err error
		origZip, err = zip.OpenReader(w.originalZip)
		if err != nil {
			return fmt.Errorf("archive writer: open original zip: %w", err)
		}
		defer origZip.Close()
	}

	// Build map of original entries for binary copy
	origEntries := make(map[string]*zip.File)
	if origZip != nil {
		for _, f := range origZip.File {
			origEntries[f.Name] = f
		}
	}

	zw := zip.NewWriter(w.Output)
	defer zw.Close()

	// Parse skeleton into a structured representation:
	// Read all skeleton entries, split by entry markers
	type entryData struct {
		name          string
		isBinary      bool
		isSubfiltered bool
		// For text entries: interleaved text/ref data
		segments []format.SkeletonEntry
	}

	var entries []entryData
	var current *entryData

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("archive writer: read skeleton: %w", err)
		}

		if entry.Type == format.SkeletonText {
			text := string(entry.Data)
			// Check for entry markers
			lines := strings.Split(text, "\n")
			var pendingText strings.Builder
			for i, line := range lines {
				if strings.HasPrefix(line, "<<ENTRY:") && strings.HasSuffix(line, ">>") {
					// Flush pending text to current entry
					if current != nil && pendingText.Len() > 0 {
						current.segments = append(current.segments, format.SkeletonEntry{
							Type: format.SkeletonText,
							Data: []byte(pendingText.String()),
						})
						pendingText.Reset()
					}
					// Save current entry if any
					if current != nil {
						entries = append(entries, *current)
					}
					name := line[len("<<ENTRY:") : len(line)-len(">>")]
					current = &entryData{name: name}
				} else if strings.HasPrefix(line, "<<BINARY:") && strings.HasSuffix(line, ">>") {
					// Flush pending text to current entry
					if current != nil && pendingText.Len() > 0 {
						current.segments = append(current.segments, format.SkeletonEntry{
							Type: format.SkeletonText,
							Data: []byte(pendingText.String()),
						})
						pendingText.Reset()
					}
					if current != nil {
						entries = append(entries, *current)
					}
					name := line[len("<<BINARY:") : len(line)-len(">>")]
					current = &entryData{name: name, isBinary: true}
				} else if strings.HasPrefix(line, "<<SUBFILTER:") && strings.HasSuffix(line, ">>") {
					// Flush pending text to current entry
					if current != nil && pendingText.Len() > 0 {
						current.segments = append(current.segments, format.SkeletonEntry{
							Type: format.SkeletonText,
							Data: []byte(pendingText.String()),
						})
						pendingText.Reset()
					}
					if current != nil {
						entries = append(entries, *current)
					}
					name := line[len("<<SUBFILTER:") : len(line)-len(">>")]
					current = &entryData{name: name, isSubfiltered: true}
				} else {
					if pendingText.Len() > 0 || i > 0 {
						pendingText.WriteString("\n")
					}
					pendingText.WriteString(line)
				}
			}
			if current != nil && pendingText.Len() > 0 {
				current.segments = append(current.segments, format.SkeletonEntry{
					Type: format.SkeletonText,
					Data: []byte(pendingText.String()),
				})
			}
		} else if entry.Type == format.SkeletonRef {
			if current != nil {
				current.segments = append(current.segments, entry)
			}
		}
	}
	if current != nil {
		entries = append(entries, *current)
	}

	// Write ZIP entries
	for _, e := range entries {
		if e.isBinary {
			// Copy from original ZIP
			if origFile, ok := origEntries[e.name]; ok {
				header := origFile.FileHeader
				writer, err := zw.CreateHeader(&header)
				if err != nil {
					return err
				}
				rc, err := origFile.Open()
				if err != nil {
					return err
				}
				if _, err := io.Copy(writer, rc); err != nil {
					rc.Close()
					return err
				}
				rc.Close()
			}
		} else if e.isSubfiltered {
			// Write subfiltered entry using child layer values
			writer, err := zw.Create(e.name)
			if err != nil {
				return err
			}
			if val, ok := childLayerValues[e.name]; ok {
				if _, err := io.WriteString(writer, val); err != nil {
					return err
				}
			}
		} else {
			// Reconstruct text entry from skeleton segments
			writer, err := zw.Create(e.name)
			if err != nil {
				return err
			}
			bw := bufio.NewWriter(writer)
			for _, seg := range e.segments {
				switch seg.Type {
				case format.SkeletonText:
					if _, err := bw.Write(seg.Data); err != nil {
						return err
					}
				case format.SkeletonRef:
					if block, ok := blocks[string(seg.Data)]; ok {
						text := w.blockText(block)
						if _, err := bw.WriteString(text); err != nil {
							return err
						}
					}
				}
			}
			if err := bw.Flush(); err != nil {
				return err
			}
		}
	}

	return nil
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
				sb.WriteString(w.blockText(block))
			}
		}
	}
	return sb.String()
}

func (w *Writer) writeArchive(parts []*model.Part, childLayerValues map[string]string) error {
	// Build a map of entry name -> translated lines
	entryBlocks := make(map[string][]string)
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

		text := w.blockText(block)
		entryBlocks[entry] = append(entryBlocks[entry], text)
	}

	// If we have original ZIP, do a roundtrip preserving all entries
	if w.originalZip != "" {
		return w.writeRoundtrip(entryBlocks, childLayerValues)
	}

	// Otherwise write only the entries we have blocks for
	return w.writeFromParts(parts, entryBlocks, childLayerValues)
}

func (w *Writer) writeRoundtrip(entryBlocks map[string][]string, childLayerValues map[string]string) error {
	origZip, err := zip.OpenReader(w.originalZip)
	if err != nil {
		return fmt.Errorf("archive writer: reading original: %w", err)
	}
	defer origZip.Close()

	zw := zip.NewWriter(w.Output)
	defer zw.Close()

	for _, file := range origZip.File {
		if file.FileInfo().IsDir() {
			_, err := zw.Create(file.Name)
			if err != nil {
				return err
			}
			continue
		}

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
		} else if lines, ok := entryBlocks[file.Name]; ok {
			// Write translated content
			content := strings.Join(lines, "\n") + "\n"
			if _, err := io.WriteString(writer, content); err != nil {
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

func (w *Writer) writeFromParts(parts []*model.Part, entryBlocks map[string][]string, childLayerValues map[string]string) error {
	zw := zip.NewWriter(w.Output)
	defer zw.Close()

	// Also include Data parts (binary entries) as empty placeholders
	written := make(map[string]bool)

	// Write subfiltered entries
	for entry, val := range childLayerValues {
		writer, err := zw.Create(entry)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(writer, val); err != nil {
			return err
		}
		written[entry] = true
	}

	// Write text entries
	for entry, lines := range entryBlocks {
		writer, err := zw.Create(entry)
		if err != nil {
			return err
		}
		content := strings.Join(lines, "\n") + "\n"
		if _, err := io.WriteString(writer, content); err != nil {
			return err
		}
		written[entry] = true
	}

	// Write data entries as empty files (preserving structure)
	for _, part := range parts {
		if part.Type != model.PartData {
			continue
		}
		data, ok := part.Resource.(*model.Data)
		if !ok {
			continue
		}
		entry := data.Properties["entry"]
		if entry == "" || written[entry] {
			continue
		}
		if _, err := zw.Create(entry); err != nil {
			return err
		}
		written[entry] = true
	}

	return nil
}

// Close flushes output and cleans up any temp files.
func (w *Writer) Close() error {
	if w.ownsTmpFile && w.originalZip != "" {
		os.Remove(w.originalZip)
		w.originalZip = ""
		w.ownsTmpFile = false
	}
	return w.BaseFormatWriter.Close()
}

// isSubfilteredLayer returns true if the layer was created by the subfilter mechanism.
// Archive's own child layers (for text entries) have Format "archive" and no subfilter property.
func isSubfilteredLayer(layer *model.Layer) bool {
	if layer.Properties == nil {
		return false
	}
	_, ok := layer.Properties["subfilter.source"]
	return ok
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}
