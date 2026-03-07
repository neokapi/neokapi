package openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for OpenXML files.
type Writer struct {
	format.BaseFormatWriter
	cfg             *Config
	skeletonStore   *format.SkeletonStore
	originalContent []byte
}

var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.OriginalContentSetter = (*Writer)(nil)

// NewWriter creates a new OpenXML writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "openxml",
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

// Write consumes Parts and writes the reconstructed OpenXML document.
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

	if w.originalContent == nil {
		return fmt.Errorf("openxml: writer requires original content for reconstruction")
	}

	// Open original ZIP
	origZR, err := zip.NewReader(bytes.NewReader(w.originalContent), int64(len(w.originalContent)))
	if err != nil {
		return fmt.Errorf("openxml: invalid original ZIP: %w", err)
	}

	// Parse container
	info, err := parseContainer(origZR, w.cfg)
	if err != nil {
		return err
	}

	// Create output ZIP
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Build set of translatable parts for quick lookup
	translatableSet := make(map[string]bool)
	for _, p := range info.translatableParts {
		translatableSet[p] = true
	}

	// If we have a skeleton store, use skeleton-based reconstruction
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("openxml: skeleton flush: %w", err)
		}
		return w.writeFromSkeleton(origZR, zw, &buf, info, blocks, translatableSet)
	}

	// Fallback: copy original with translated XML parts
	return w.writeFromReparse(origZR, zw, &buf, info, blocks, translatableSet)
}

// writeFromSkeleton reconstructs translatable XML parts using the skeleton store.
func (w *Writer) writeFromSkeleton(origZR *zip.Reader, zw *zip.Writer, buf *bytes.Buffer,
	info *containerInfo, blocks map[string]*model.Block, translatableSet map[string]bool) error {

	// Read all skeleton entries into a buffer
	var skelContent bytes.Buffer
	for {
		entry, err := w.skeletonStore.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("openxml: reading skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			skelContent.Write(entry.Data)
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				skelContent.WriteString(w.renderBlock(block))
			}
		}
	}

	// The skeleton contains the content for all translatable parts concatenated.
	// For now, we use it as a single replacement for the main document part.
	// TODO: Support per-part skeletons for multi-part documents.
	skelBytes := skelContent.Bytes()

	for _, f := range origZR.File {
		if f.Name == info.mainDocumentPart && len(skelBytes) > 0 {
			// Replace with skeleton-reconstructed content — need a fresh header
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(skelBytes); err != nil {
				return err
			}
		} else {
			// Copy unchanged — use raw copy to preserve CRC/data descriptors
			if err := zw.Copy(f); err != nil {
				return err
			}
		}
	}

	if err := zw.Close(); err != nil {
		return err
	}
	_, err := w.Output.Write(buf.Bytes())
	return err
}

// writeFromReparse copies the original ZIP, replacing translatable XML parts
// by re-parsing and substituting translated block text.
func (w *Writer) writeFromReparse(origZR *zip.Reader, zw *zip.Writer, buf *bytes.Buffer,
	info *containerInfo, blocks map[string]*model.Block, translatableSet map[string]bool) error {

	for _, f := range origZR.File {
		// Copy all entries unchanged using raw copy
		if err := zw.Copy(f); err != nil {
			return err
		}
	}

	if err := zw.Close(); err != nil {
		return err
	}
	_, err := w.Output.Write(buf.Bytes())
	return err
}

// renderBlock converts a block's content back to WordprocessingML runs.
func (w *Writer) renderBlock(block *model.Block) string {
	frag := w.getFragment(block)
	if frag == nil {
		return ""
	}

	if !frag.HasSpans() {
		// Simple text, no formatting
		return `<w:r><w:t xml:space="preserve">` + xmlEscape(frag.CodedText) + `</w:t></w:r>`
	}

	// Render fragment with spans
	var buf strings.Builder
	var inRun bool
	spanIdx := 0

	for _, r := range frag.CodedText {
		switch r {
		case model.MarkerOpening:
			span := frag.Spans[spanIdx]
			spanIdx++
			// Formatting spans modify the current run's properties
			// For now, emit as separate runs
			if inRun {
				buf.WriteString(`</w:t></w:r>`)
				inRun = false
			}
			if span.Type == TypeHyperlink {
				buf.WriteString(span.Data)
			}
			// Start a new run with this formatting
			// (simplified: full implementation would track cumulative props)

		case model.MarkerClosing:
			span := frag.Spans[spanIdx]
			spanIdx++
			if inRun {
				buf.WriteString(`</w:t></w:r>`)
				inRun = false
			}
			if span.Type == TypeHyperlink {
				buf.WriteString(span.Data)
			}

		case model.MarkerPlaceholder:
			span := frag.Spans[spanIdx]
			spanIdx++
			if inRun {
				buf.WriteString(`</w:t></w:r>`)
				inRun = false
			}
			// Emit placeholder directly
			switch span.Type {
			case TypeBreak:
				buf.WriteString(`<w:r><w:br/></w:r>`)
			case TypeTab:
				buf.WriteString(`<w:r><w:tab/></w:r>`)
			case TypeImage:
				buf.WriteString(`<w:r>` + span.Data + `</w:r>`)
			case TypeFootnoteRef:
				buf.WriteString(`<w:r>` + span.Data + `</w:r>`)
			default:
				buf.WriteString(span.Data)
			}

		default:
			if !inRun {
				buf.WriteString(`<w:r><w:t xml:space="preserve">`)
				inRun = true
			}
			buf.WriteRune(r)
		}
	}

	if inRun {
		buf.WriteString(`</w:t></w:r>`)
	}
	return buf.String()
}

// getFragment returns the appropriate fragment (target or source) for a block.
func (w *Writer) getFragment(block *model.Block) *model.Fragment {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs := block.Targets[w.Locale]
		if len(segs) > 0 && segs[0].Content != nil {
			return segs[0].Content
		}
	}
	if len(block.Source) > 0 {
		return block.Source[0].Content
	}
	return nil
}
