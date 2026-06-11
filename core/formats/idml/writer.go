package idml

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for IDML files.
//
// The writer reconstructs an IDML ZIP package by replacing translatable
// text in story XML files with translated content, preserving all other
// ZIP entries unchanged.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
	// originalContent holds the source archive bytes when the caller hands
	// them over via SetOriginalContent. When sourcePath is set instead
	// (SourcePathSetter), the source is re-opened from disk in Write and
	// these bytes are never held — avoiding a full second copy of the
	// archive in memory (#608, S2).
	originalContent []byte
	sourcePath      string
}

var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.OriginalContentSetter = (*Writer)(nil)
var _ format.SourcePathSetter = (*Writer)(nil)

// NewWriter creates a new IDML writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "idml",
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming reconstruction.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// SetOriginalContent sets the original document bytes for reconstruction.
func (w *Writer) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// SetSourcePath records the path to the original IDML so Write can
// re-open it from disk instead of holding a full in-memory copy. When
// set it takes precedence over SetOriginalContent (#608, S2).
func (w *Writer) SetSourcePath(path string) {
	w.sourcePath = path
}

// Write consumes Parts and writes the reconstructed IDML document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Collect all blocks keyed by ID
	blocks := make(map[string]*model.Block)
	for part := range parts {
		if part.Type == model.PartBlock {
			if b, ok := part.Resource.(*model.Block); ok {
				blocks[b.ID] = b
			}
		}
	}

	// Resolve the source archive: prefer re-opening from the path (no
	// second in-memory copy) and fall back to held bytes.
	if w.sourcePath == "" && w.originalContent == nil {
		return errors.New("idml: writer requires original content for reconstruction")
	}
	var origZR *zip.Reader
	if w.sourcePath != "" {
		zrc, err := zip.OpenReader(w.sourcePath)
		if err != nil {
			return fmt.Errorf("idml: open source %q: %w", w.sourcePath, err)
		}
		defer zrc.Close()
		origZR = &zrc.Reader
	} else {
		zr, err := zip.NewReader(bytes.NewReader(w.originalContent), int64(len(w.originalContent)))
		if err != nil {
			return fmt.Errorf("idml: invalid original ZIP: %w", err)
		}
		origZR = zr
	}

	// Stream the output ZIP straight to w.Output rather than buffering the
	// entire reconstructed package in memory first. zip.Writer produces
	// byte-identical output regardless of the underlying writer.
	zw := zip.NewWriter(w.Output)

	// If we have a skeleton store, use skeleton-based reconstruction
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("idml: skeleton flush: %w", err)
		}
		if err := w.writeFromSkeleton(origZR, zw, blocks); err != nil {
			return err
		}
		return zw.Close()
	}

	// Fallback: copy original unchanged
	if err := w.writeOriginal(origZR, zw); err != nil {
		return err
	}
	return zw.Close()
}

// writeFromSkeleton reconstructs translatable story XML parts using the skeleton store.
//
// The skeleton stream emits each story bounded by part-start/part-end markers,
// with all of that story's text and block refs arriving in between. The output
// ZIP, however, must preserve the original entry order (origZR.File), in which
// story files are interleaved with non-story entries. Because that order
// differs from the skeleton emission order, reconstructed stories are collected
// into partContents keyed by ZIP entry name, then emitted in ZIP order below.
//
// Each story is reconstructed into its own buffer so the stored slice is never
// aliased or reused for a later story — that lets partContents hold the buffer
// bytes directly without a defensive copy.
func (w *Writer) writeFromSkeleton(origZR *zip.Reader, zw *zip.Writer,
	blocks map[string]*model.Block) error {

	// Read all skeleton entries, splitting by part-boundary markers
	partContents := make(map[string][]byte)
	var currentPart string
	var currentBuf *bytes.Buffer

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("idml: reading skeleton: %w", err)
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
				// Fresh buffer per part: its backing array is handed off to
				// partContents at part-end and never reused, so no copy is needed.
				currentBuf = &bytes.Buffer{}
				continue
			}
			if after, ok := strings.CutPrefix(refID, skelPartEndPrefix); ok {
				partPath := after
				if currentBuf != nil && currentBuf.Len() > 0 {
					partContents[partPath] = currentBuf.Bytes()
				}
				currentPart = ""
				currentBuf = nil
				continue
			}

			// Regular block ref — render and write
			if currentPart != "" {
				if block, ok := blocks[refID]; ok {
					currentBuf.WriteString(w.renderBlock(block))
				}
			}
		}
	}

	// Write output ZIP: replace translatable parts with skeleton-reconstructed content
	for _, f := range origZR.File {
		if content, ok := partContents[f.Name]; ok && len(content) > 0 {
			// Replace with skeleton-reconstructed content
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
		} else {
			// Copy unchanged
			if err := zw.Copy(f); err != nil {
				return err
			}
		}
	}

	return nil
}

// writeOriginal copies the original ZIP unchanged (no skeleton available).
func (w *Writer) writeOriginal(origZR *zip.Reader, zw *zip.Writer) error {
	for _, f := range origZR.File {
		if err := zw.Copy(f); err != nil {
			return err
		}
	}
	return nil
}

// renderBlock converts a block's content back to plain text for IDML Content elements.
func (w *Writer) renderBlock(block *model.Block) string {
	runs := w.blockRuns(block)
	if runs == nil {
		return ""
	}
	return xmlEscape(model.FlattenRuns(runs))
}

// blockRuns returns the target or source Run sequence for the block,
// preferring the configured target locale when present.
func (w *Writer) blockRuns(block *model.Block) []model.Run {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		if runs := block.TargetRuns(w.Locale); len(runs) > 0 {
			return runs
		}
	}
	if len(block.Source) > 0 {
		return block.Source
	}
	return nil
}
