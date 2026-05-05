package openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// wmlStrippableElementRE matches self-closing WordprocessingML elements
// that the okapi reference filter strips during round-trip:
//   - <w:lang .../>          (run-property language, also paragraph-rPr)
//   - <w:bidiVisual .../>    (paragraph-property right-to-left visual hint)
//
// Both live under <w:rPr>/<w:pPr> and are skipped by okapi's
// SkippableElements.Default(BLOCK_PROPERTY_BIDI_VISUAL,
// RUN_PROPERTY_LANGUAGE) wired into BlockPropertiesFactory and
// WordStyleDefinition.DocumentDefaults; preserving them in pass-through
// XML parts is the dominant openxml writer parity gap.
var wmlStrippableElementRE = regexp.MustCompile(`<w:(?:lang|bidiVisual)\b[^>]*/>`)

// stripWMLSkippableElements removes <w:lang/> and <w:bidiVisual/> from an
// XML part to mirror okapi's BlockProperties/RunProperties stripping.
// Returns the original slice if no element was matched (cheap fast path).
func stripWMLSkippableElements(data []byte) []byte {
	if !bytes.Contains(data, []byte("<w:lang")) && !bytes.Contains(data, []byte("<w:bidiVisual")) {
		return data
	}
	return wmlStrippableElementRE.ReplaceAll(data, nil)
}

// shouldStripWMLLang reports whether the given ZIP entry path is a
// WordprocessingML XML part where okapi's lang/bidiVisual stripping
// applies. Other parts (drawings, themes, settings.xml) are untouched.
func shouldStripWMLLang(name string) bool {
	if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
		return false
	}
	switch {
	case name == "word/document.xml",
		name == "word/styles.xml",
		name == "word/footnotes.xml",
		name == "word/endnotes.xml",
		name == "word/comments.xml":
		return true
	case strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml"),
		strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml"):
		return true
	}
	return false
}

// Writer implements DataFormatWriter for OpenXML files.
type Writer struct {
	format.BaseFormatWriter
	cfg             *Config
	skeletonStore   *format.SkeletonStore
	originalContent []byte

	// mediaReplacements maps ZIP entry paths (e.g., "word/media/image1.png")
	// to replacement binary content for locale-variant media substitution (Bowrain AD-007).
	mediaReplacements map[string][]byte
}

var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.OriginalContentSetter = (*Writer)(nil)

// SetMediaReplacement registers a locale-variant media file to substitute
// during output reconstruction. The zipPath should match the original
// entry path (e.g., "word/media/image1.png").
func (w *Writer) SetMediaReplacement(zipPath string, data []byte) {
	if w.mediaReplacements == nil {
		w.mediaReplacements = make(map[string][]byte)
	}
	w.mediaReplacements[zipPath] = data
}

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
		return errors.New("openxml: writer requires original content for reconstruction")
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
		if errors.Is(err, io.EOF) {
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

	// Write output ZIP: replace translatable parts with skeleton-reconstructed content,
	// and substitute locale-variant media files (Bowrain AD-007).
	isDOCX := info.docType == docTypeDOCX
	for _, f := range origZR.File {
		if content, ok := partContents[f.Name]; ok && len(content) > 0 {
			// Replace with skeleton-reconstructed content
			if isDOCX && shouldStripWMLLang(f.Name) {
				content = stripWMLSkippableElements(content)
			}
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
		} else if replacement, ok := w.mediaReplacements[f.Name]; ok {
			// Replace with locale-variant media (Bowrain AD-007).
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(replacement); err != nil {
				return err
			}
		} else if isDOCX && shouldStripWMLLang(f.Name) {
			// Pass-through WordprocessingML part (e.g. word/styles.xml)
			// that needs okapi-style lang/bidiVisual stripping. Read,
			// transform, re-emit with a recompressed header.
			data, err := readZipFile(f)
			if err != nil {
				return err
			}
			data = stripWMLSkippableElements(data)
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(data); err != nil {
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

// writeFromReparse copies the original ZIP, substituting locale-variant media (Bowrain AD-007).
func (w *Writer) writeFromReparse(origZR *zip.Reader, zw *zip.Writer, buf *bytes.Buffer,
	blocks map[string]*model.Block) error {

	for _, f := range origZR.File {
		if replacement, ok := w.mediaReplacements[f.Name]; ok {
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(replacement); err != nil {
				return err
			}
		} else {
			if err := zw.Copy(f); err != nil {
				return err
			}
		}
	}

	return zw.Close()
}

// renderBlock converts a block's content back to the appropriate XML dialect.
func (w *Writer) renderBlock(block *model.Block, dt docType) string {
	runs := w.preferredRuns(block)
	if runs == nil {
		return ""
	}

	// Core properties and table column names are plain text (no XML wrapping needed).
	if block.Type == "property" || block.Type == "table-column" {
		return xmlEscapeAttr(model.FlattenRuns(runs))
	}

	switch dt {
	case docTypeDOCX:
		return w.renderWMLBlock(runs)
	case docTypePPTX:
		return w.renderDMLBlock(runs)
	case docTypeXLSX:
		return w.renderSMLBlock(runs, block)
	default:
		return w.renderWMLBlock(runs)
	}
}

// runsHaveInlineCodes reports whether the run sequence contains any
// non-text runs (placeholders or paired codes). The fast path for a
// plain-text block short-circuits the walker below.
func runsHaveInlineCodes(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// renderWMLBlock renders a run sequence as WordprocessingML runs.
func (w *Writer) renderWMLBlock(runs []model.Run) string {
	if !runsHaveInlineCodes(runs) {
		return `<w:r><w:t xml:space="preserve">` + xmlEscape(model.FlattenRuns(runs)) + `</w:t></w:r>`
	}

	var buf strings.Builder
	var inRun bool
	var runProps string

	closeRun := func() {
		if inRun {
			buf.WriteString(`</w:t></w:r>`)
			inRun = false
		}
	}

	for _, r := range runs {
		switch {
		case r.Text != nil:
			for _, ch := range r.Text.Text {
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
				xmlEscapeRune(&buf, ch)
			}

		case r.PcOpen != nil:
			if r.PcOpen.Type == TypeHyperlink {
				closeRun()
				buf.WriteString(r.PcOpen.Data)
			} else {
				runProps = w.addWMLProp(runProps, r.PcOpen.Type)
			}

		case r.PcClose != nil:
			if r.PcClose.Type == TypeHyperlink {
				closeRun()
				buf.WriteString(r.PcClose.Data)
			} else {
				runProps = w.removeWMLProp(runProps, r.PcClose.Type)
			}

		case r.Ph != nil:
			closeRun()
			switch r.Ph.Type {
			case TypeBreak:
				buf.WriteString(`<w:r><w:br/></w:r>`)
			case TypeTab:
				buf.WriteString(`<w:r><w:tab/></w:r>`)
			case TypeImage:
				buf.WriteString(`<w:r>` + r.Ph.Data + `</w:r>`)
			case TypeFootnoteRef:
				buf.WriteString(`<w:r>` + r.Ph.Data + `</w:r>`)
			default:
				buf.WriteString(r.Ph.Data)
			}
		}
	}

	closeRun()
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

// renderDMLBlock renders a run sequence as DrawingML runs.
func (w *Writer) renderDMLBlock(runs []model.Run) string {
	if !runsHaveInlineCodes(runs) {
		return `<a:r><a:t>` + xmlEscape(model.FlattenRuns(runs)) + `</a:t></a:r>`
	}

	var buf strings.Builder
	var inRun bool
	var runPropsAttrs []string

	closeRun := func() {
		if inRun {
			buf.WriteString(`</a:t></a:r>`)
			inRun = false
		}
	}

	for _, r := range runs {
		switch {
		case r.Text != nil:
			for _, ch := range r.Text.Text {
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
				xmlEscapeRune(&buf, ch)
			}

		case r.PcOpen != nil:
			runPropsAttrs = w.addDMLProp(runPropsAttrs, r.PcOpen.Type)

		case r.PcClose != nil:
			closeRun()
			runPropsAttrs = w.removeDMLProp(runPropsAttrs, r.PcClose.Type)

		case r.Ph != nil:
			closeRun()
			if r.Ph.Type == TypeBreak {
				buf.WriteString(`<a:br/>`)
			} else {
				buf.WriteString(r.Ph.Data)
			}
		}
	}

	closeRun()
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

// renderSMLBlock renders a run sequence as SpreadsheetML content.
func (w *Writer) renderSMLBlock(runs []model.Run, block *model.Block) string {
	if block.Type == "shared-string" {
		return w.renderSMLSharedString(runs)
	}

	// Cell content — wrap in <v> element as inline string type. Flatten
	// to plain text: inline codes in cell values are rare and the legacy
	// path stripped markers via Fragment.Text().
	return `<v>` + xmlEscape(model.FlattenRuns(runs)) + `</v>`
}

// renderSMLSharedString renders a run sequence as shared string <si> content.
func (w *Writer) renderSMLSharedString(runs []model.Run) string {
	if !runsHaveInlineCodes(runs) {
		return `<t>` + xmlEscape(model.FlattenRuns(runs)) + `</t>`
	}

	// Rich text shared string — emit <r> elements
	var buf strings.Builder
	var inRun bool
	var currentProps []string

	closeRun := func() {
		if inRun {
			buf.WriteString(`</t></r>`)
			inRun = false
		}
	}

	for _, r := range runs {
		switch {
		case r.Text != nil:
			for _, ch := range r.Text.Text {
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
				xmlEscapeRune(&buf, ch)
			}

		case r.PcOpen != nil:
			closeRun()
			currentProps = w.addSMLProp(currentProps, r.PcOpen.Type)

		case r.PcClose != nil:
			closeRun()
			currentProps = w.removeSMLProp(currentProps, r.PcClose.Type)

		case r.Ph != nil:
			// Placeholders are skipped in shared strings (legacy behaviour).
		}
	}

	closeRun()
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

// preferredRuns returns the target runs for the writer's locale when
// present, falling back to the source runs. Returns nil if neither is
// available, matching the earlier getFragment contract.
func (w *Writer) preferredRuns(block *model.Block) []model.Run {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs := block.Targets[w.Locale]
		if len(segs) > 0 && len(segs[0].Runs) > 0 {
			return segs[0].Runs
		}
	}
	if len(block.Source) > 0 && len(block.Source[0].Runs) > 0 {
		return block.Source[0].Runs
	}
	return nil
}
