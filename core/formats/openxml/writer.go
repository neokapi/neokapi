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

	// If we have a skeleton store, use skeleton-based reconstruction
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("openxml: skeleton flush: %w", err)
		}
		if err := w.writeFromSkeleton(origZR, zw, &buf, info, blocks); err != nil {
			return err
		}
		_, err = w.Output.Write(buf.Bytes())
		return err
	}

	// Fallback: copy original unchanged
	if err := w.writeFromReparse(origZR, zw, &buf, blocks); err != nil {
		return err
	}
	_, err = w.Output.Write(buf.Bytes())
	return err
}

// writeFromSkeleton reconstructs translatable XML parts using the skeleton store.
// The skeleton stream contains part-boundary markers (skelPartStartPrefix/skelPartEndPrefix)
// that delimit each XML part's skeleton content. The writer collects each part's
// reconstructed bytes, then writes the output ZIP with replacements.
func (w *Writer) writeFromSkeleton(origZR *zip.Reader, zw *zip.Writer, buf *bytes.Buffer,
	info *containerInfo, blocks map[string]*model.Block) error {

	// Read all skeleton entries, splitting by part-boundary markers
	partContents := make(map[string][]byte)
	var currentPart string
	var currentBuf bytes.Buffer

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

			// Regular block ref — render and write
			if currentPart != "" {
				if block, ok := blocks[refID]; ok {
					currentBuf.WriteString(w.renderBlock(block, info.docType))
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
			// Clear data descriptor fields to avoid checksum issues
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
			if err := zw.Copy(f); err != nil {
				return err
			}
		}
	}

	return zw.Close()
}

// writeFromReparse copies the original ZIP unchanged (no skeleton available).
func (w *Writer) writeFromReparse(origZR *zip.Reader, zw *zip.Writer, buf *bytes.Buffer,
	blocks map[string]*model.Block) error {

	for _, f := range origZR.File {
		if err := zw.Copy(f); err != nil {
			return err
		}
	}

	return zw.Close()
}

// renderBlock converts a block's content back to the appropriate XML dialect.
func (w *Writer) renderBlock(block *model.Block, dt docType) string {
	frag := w.getFragment(block)
	if frag == nil {
		return ""
	}

	// Core properties blocks are plain text (no XML wrapping needed).
	if block.Type == "property" {
		return xmlEscape(frag.Text())
	}

	switch dt {
	case docTypeDOCX:
		return w.renderWMLBlock(frag)
	case docTypePPTX:
		return w.renderDMLBlock(frag)
	case docTypeXLSX:
		return w.renderSMLBlock(frag, block)
	default:
		return w.renderWMLBlock(frag)
	}
}

// renderWMLBlock renders a fragment as WordprocessingML runs.
func (w *Writer) renderWMLBlock(frag *model.Fragment) string {
	if !frag.HasSpans() {
		return `<w:r><w:t xml:space="preserve">` + xmlEscape(frag.CodedText) + `</w:t></w:r>`
	}

	var buf strings.Builder
	var inRun bool
	var runProps string
	spanIdx := 0

	for _, r := range frag.CodedText {
		switch r {
		case model.MarkerOpening:
			span := frag.Spans[spanIdx]
			spanIdx++
			if span.Type == TypeHyperlink {
				if inRun {
					buf.WriteString(`</w:t></w:r>`)
					inRun = false
				}
				buf.WriteString(span.Data)
			} else {
				// Accumulate formatting for next run's rPr
				runProps = w.addWMLProp(runProps, span.Type)
			}

		case model.MarkerClosing:
			span := frag.Spans[spanIdx]
			spanIdx++
			if span.Type == TypeHyperlink {
				if inRun {
					buf.WriteString(`</w:t></w:r>`)
					inRun = false
				}
				buf.WriteString(span.Data)
			} else {
				runProps = w.removeWMLProp(runProps, span.Type)
			}

		case model.MarkerPlaceholder:
			span := frag.Spans[spanIdx]
			spanIdx++
			if inRun {
				buf.WriteString(`</w:t></w:r>`)
				inRun = false
			}
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
				buf.WriteString(`<w:r>`)
				if runProps != "" {
					buf.WriteString(`<w:rPr>`)
					buf.WriteString(runProps)
					buf.WriteString(`</w:rPr>`)
				}
				buf.WriteString(`<w:t xml:space="preserve">`)
				inRun = true
			}
			xmlEscapeRune(&buf, r)
		}
	}

	if inRun {
		buf.WriteString(`</w:t></w:r>`)
	}
	return buf.String()
}

// addWMLProp adds a formatting property element to the accumulated rPr content.
func (w *Writer) addWMLProp(current, spanType string) string {
	switch spanType {
	case TypeBold:
		return current + "<w:b/>"
	case TypeItalic:
		return current + "<w:i/>"
	case TypeUnderline:
		return current + `<w:u w:val="single"/>`
	case TypeStrikethrough:
		return current + "<w:strike/>"
	case TypeSuperscript:
		return current + `<w:vertAlign w:val="superscript"/>`
	case TypeSubscript:
		return current + `<w:vertAlign w:val="subscript"/>`
	}
	return current
}

// removeWMLProp removes a formatting property from the accumulated rPr content.
func (w *Writer) removeWMLProp(current, spanType string) string {
	switch spanType {
	case TypeBold:
		return strings.ReplaceAll(current, "<w:b/>", "")
	case TypeItalic:
		return strings.ReplaceAll(current, "<w:i/>", "")
	case TypeUnderline:
		return strings.ReplaceAll(current, `<w:u w:val="single"/>`, "")
	case TypeStrikethrough:
		return strings.ReplaceAll(current, "<w:strike/>", "")
	case TypeSuperscript:
		return strings.ReplaceAll(current, `<w:vertAlign w:val="superscript"/>`, "")
	case TypeSubscript:
		return strings.ReplaceAll(current, `<w:vertAlign w:val="subscript"/>`, "")
	}
	return current
}

// renderDMLBlock renders a fragment as DrawingML runs.
func (w *Writer) renderDMLBlock(frag *model.Fragment) string {
	if !frag.HasSpans() {
		return `<a:r><a:t>` + xmlEscape(frag.CodedText) + `</a:t></a:r>`
	}

	var buf strings.Builder
	var inRun bool
	var runPropsAttrs []string
	spanIdx := 0

	for _, r := range frag.CodedText {
		switch r {
		case model.MarkerOpening:
			span := frag.Spans[spanIdx]
			spanIdx++
			runPropsAttrs = w.addDMLProp(runPropsAttrs, span.Type)

		case model.MarkerClosing:
			span := frag.Spans[spanIdx]
			spanIdx++
			if inRun {
				buf.WriteString(`</a:t></a:r>`)
				inRun = false
			}
			runPropsAttrs = w.removeDMLProp(runPropsAttrs, span.Type)

		case model.MarkerPlaceholder:
			span := frag.Spans[spanIdx]
			spanIdx++
			if inRun {
				buf.WriteString(`</a:t></a:r>`)
				inRun = false
			}
			if span.Type == TypeBreak {
				buf.WriteString(`<a:br/>`)
			} else {
				buf.WriteString(span.Data)
			}

		default:
			if !inRun {
				buf.WriteString(`<a:r>`)
				if len(runPropsAttrs) > 0 {
					buf.WriteString(`<a:rPr `)
					buf.WriteString(strings.Join(runPropsAttrs, " "))
					buf.WriteString(`/>`)
				}
				buf.WriteString(`<a:t>`)
				inRun = true
			}
			xmlEscapeRune(&buf, r)
		}
	}

	if inRun {
		buf.WriteString(`</a:t></a:r>`)
	}
	return buf.String()
}

func (w *Writer) addDMLProp(attrs []string, spanType string) []string {
	switch spanType {
	case TypeBold:
		return append(attrs, `b="1"`)
	case TypeItalic:
		return append(attrs, `i="1"`)
	case TypeUnderline:
		return append(attrs, `u="sng"`)
	case TypeStrikethrough:
		return append(attrs, `strike="sngStrike"`)
	case TypeSuperscript:
		return append(attrs, `baseline="30000"`)
	case TypeSubscript:
		return append(attrs, `baseline="-25000"`)
	}
	return attrs
}

func (w *Writer) removeDMLProp(attrs []string, spanType string) []string {
	var target string
	switch spanType {
	case TypeBold:
		target = `b="1"`
	case TypeItalic:
		target = `i="1"`
	case TypeUnderline:
		target = `u="sng"`
	case TypeStrikethrough:
		target = `strike="sngStrike"`
	case TypeSuperscript:
		target = `baseline="30000"`
	case TypeSubscript:
		target = `baseline="-25000"`
	default:
		return attrs
	}
	var result []string
	for _, a := range attrs {
		if a != target {
			result = append(result, a)
		}
	}
	return result
}

// renderSMLBlock renders a fragment as SpreadsheetML content.
func (w *Writer) renderSMLBlock(frag *model.Fragment, block *model.Block) string {
	blockType := block.Type

	if blockType == "shared-string" {
		return w.renderSMLSharedString(frag)
	}

	// Cell content — wrap in <v> element as inline string type
	text := frag.CodedText
	if frag.HasSpans() {
		text = frag.Text()
	}
	return `<v>` + xmlEscape(text) + `</v>`
}

// renderSMLSharedString renders a fragment as a shared string <si> content.
func (w *Writer) renderSMLSharedString(frag *model.Fragment) string {
	if !frag.HasSpans() {
		return `<t>` + xmlEscape(frag.CodedText) + `</t>`
	}

	// Rich text shared string — emit <r> elements
	var buf strings.Builder
	var inRun bool
	var currentProps []string
	spanIdx := 0

	for _, r := range frag.CodedText {
		switch r {
		case model.MarkerOpening:
			span := frag.Spans[spanIdx]
			spanIdx++
			if inRun {
				buf.WriteString(`</t></r>`)
				inRun = false
			}
			currentProps = w.addSMLProp(currentProps, span.Type)

		case model.MarkerClosing:
			span := frag.Spans[spanIdx]
			spanIdx++
			if inRun {
				buf.WriteString(`</t></r>`)
				inRun = false
			}
			currentProps = w.removeSMLProp(currentProps, span.Type)

		case model.MarkerPlaceholder:
			// Skip placeholders in shared strings
			spanIdx++

		default:
			if !inRun {
				buf.WriteString(`<r>`)
				if len(currentProps) > 0 {
					buf.WriteString(`<rPr>`)
					for _, p := range currentProps {
						buf.WriteString(p)
					}
					buf.WriteString(`</rPr>`)
				}
				buf.WriteString(`<t>`)
				inRun = true
			}
			xmlEscapeRune(&buf, r)
		}
	}

	if inRun {
		buf.WriteString(`</t></r>`)
	}
	return buf.String()
}

func (w *Writer) addSMLProp(props []string, spanType string) []string {
	switch spanType {
	case TypeBold:
		return append(props, `<b/>`)
	case TypeItalic:
		return append(props, `<i/>`)
	case TypeUnderline:
		return append(props, `<u/>`)
	case TypeStrikethrough:
		return append(props, `<strike/>`)
	case TypeSuperscript:
		return append(props, `<vertAlign val="superscript"/>`)
	case TypeSubscript:
		return append(props, `<vertAlign val="subscript"/>`)
	}
	return props
}

func (w *Writer) removeSMLProp(props []string, spanType string) []string {
	var target string
	switch spanType {
	case TypeBold:
		target = `<b/>`
	case TypeItalic:
		target = `<i/>`
	case TypeUnderline:
		target = `<u/>`
	case TypeStrikethrough:
		target = `<strike/>`
	case TypeSuperscript:
		target = `<vertAlign val="superscript"/>`
	case TypeSubscript:
		target = `<vertAlign val="subscript"/>`
	default:
		return props
	}
	var result []string
	for _, p := range props {
		if p != target {
			result = append(result, p)
		}
	}
	return result
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
