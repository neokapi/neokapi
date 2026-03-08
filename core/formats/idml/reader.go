package idml

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for Adobe InDesign IDML files.
//
// IDML is a ZIP package containing XML story files (Stories/Story_*.xml),
// spread files, master spread files, and various resources. The reader
// extracts translatable text from story XML files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new IDML reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "idml",
			FormatDisplayName: "Adobe InDesign Markup Language",
			FormatMimeType:    "application/vnd.adobe.indesign-idml-package",
			FormatExtensions:  []string{".idml"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/vnd.adobe.indesign-idml-package"},
		Extensions: []string{".idml"},
		MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}}, // PK ZIP header
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("idml: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Read all content into memory (ZIP requires random access)
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("idml: reading: %w", err)}
		return
	}

	// Open as ZIP
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("idml: not a valid ZIP archive: %w", err)}
		return
	}

	// Find story files
	storyFiles := r.findStoryFiles(zr)
	if len(storyFiles) == 0 {
		ch <- model.PartResult{Error: fmt.Errorf("idml: no story files found in archive")}
		return
	}

	// Emit root layer
	rootLayer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "idml",
		Locale:   locale,
		Encoding: "UTF-8",
		MimeType: "application/vnd.adobe.indesign-idml-package",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: rootLayer}) {
		return
	}

	blockCounter := 0

	// Process each story file
	for _, sf := range storyFiles {
		zf := zipFileByName(zr, sf)
		if zf == nil {
			continue
		}

		storyData, err := readZipFile(zf)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("idml: reading %s: %w", sf, err)}
			return
		}

		// Emit child layer for this story
		childLayer := &model.Layer{
			ID:       "layer-" + sf,
			Name:     sf,
			Locale:   locale,
			ParentID: rootLayer.ID,
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
			return
		}

		// Emit skeleton part-boundary marker
		r.skelPartStart(sf)

		// Parse story XML and extract blocks
		if err := r.parseStory(ctx, ch, storyData, sf, &blockCounter); err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("idml: parsing %s: %w", sf, err)}
			return
		}

		r.skelPartEnd(sf)

		// End child layer
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer}) {
			return
		}
	}

	// End root layer
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
}

// findStoryFiles returns sorted list of story XML file paths from the ZIP.
func (r *Reader) findStoryFiles(zr *zip.Reader) []string {
	var stories []string
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "Stories/") && strings.HasSuffix(f.Name, ".xml") {
			stories = append(stories, f.Name)
		}
	}
	sort.Strings(stories)
	return stories
}

// parseStory parses a single story XML file and emits blocks for translatable content.
//
// IDML story XML structure:
//
//	<Story>
//	  <ParagraphStyleRange AppliedParagraphStyle="...">
//	    <CharacterStyleRange AppliedCharacterStyle="...">
//	      <Content>translatable text</Content>
//	    </CharacterStyleRange>
//	    <Br/> <!-- line break -->
//	    <CharacterStyleRange>
//	      <Content>more text</Content>
//	      <Footnote>
//	        <ParagraphStyleRange>
//	          <CharacterStyleRange>
//	            <Content>footnote text</Content>
//	          </CharacterStyleRange>
//	        </ParagraphStyleRange>
//	      </Footnote>
//	    </CharacterStyleRange>
//	  </ParagraphStyleRange>
//	</Story>
func (r *Reader) parseStory(ctx context.Context, ch chan<- model.PartResult,
	data []byte, storyPath string, blockCounter *int) error {

	d := xml.NewDecoder(bytes.NewReader(data))

	var skelBuf bytes.Buffer
	var inContent bool
	var noteDepth int // >0 when inside a Note/Footnote/Endnote element
	var textBuf strings.Builder

	// Style tracking uses a stack so nested ParagraphStyleRange/CharacterStyleRange
	// (e.g. inside footnotes, tables) are handled correctly.
	type styleState struct {
		paragraphStyle string
		charStyle      string
	}
	var styleStack []styleState
	currentStyle := styleState{}

	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("parsing XML: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "ParagraphStyleRange":
				styleStack = append(styleStack, currentStyle)
				currentStyle.paragraphStyle = attrVal(t.Attr, "AppliedParagraphStyle")
				r.skelWriteStartElement(&skelBuf, t)

			case "CharacterStyleRange":
				styleStack = append(styleStack, currentStyle)
				currentStyle.charStyle = attrVal(t.Attr, "AppliedCharacterStyle")
				r.skelWriteStartElement(&skelBuf, t)

			case "Content":
				inContent = true
				textBuf.Reset()

			case "Br":
				// Line break — skeleton only
				r.skelWriteStartElement(&skelBuf, t)

			case "Note", "Footnote", "Endnote":
				noteDepth++
				r.skelWriteStartElement(&skelBuf, t)

			default:
				r.skelWriteStartElement(&skelBuf, t)
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "Content":
				if inContent {
					text := textBuf.String()
					if r.cfg.SkipDiscretionaryHyphens {
						text = strings.ReplaceAll(text, "\u00AD", "")
					}

					trimmed := strings.TrimSpace(text)
					inNote := noteDepth > 0

					if trimmed == "" || (inNote && !r.cfg.ExtractNotes) {
						// Non-translatable: write to skeleton as text
						r.skelText(&skelBuf, xmlEscape(text))
					} else {
						// Translatable content: emit block
						*blockCounter++
						blockID := fmt.Sprintf("tu%d", *blockCounter)

						r.skelRef(&skelBuf, blockID)

						block := &model.Block{
							ID:           blockID,
							Translatable: true,
							Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(text)}},
							Targets:      make(map[model.LocaleID][]*model.Segment),
							Properties: map[string]string{
								"storyPath":      storyPath,
								"paragraphStyle": currentStyle.paragraphStyle,
								"characterStyle": currentStyle.charStyle,
							},
							Annotations: make(map[string]model.Annotation),
						}
						if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
							return nil
						}
					}

					inContent = false
					textBuf.Reset()
				}

			case "ParagraphStyleRange":
				r.skelWriteEndElement(&skelBuf, t)
				if len(styleStack) > 0 {
					currentStyle = styleStack[len(styleStack)-1]
					styleStack = styleStack[:len(styleStack)-1]
				}

			case "CharacterStyleRange":
				r.skelWriteEndElement(&skelBuf, t)
				if len(styleStack) > 0 {
					currentStyle = styleStack[len(styleStack)-1]
					styleStack = styleStack[:len(styleStack)-1]
				}

			case "Note", "Footnote", "Endnote":
				noteDepth--
				r.skelWriteEndElement(&skelBuf, t)

			default:
				r.skelWriteEndElement(&skelBuf, t)
			}

		case xml.CharData:
			if inContent {
				textBuf.Write(t)
			} else {
				r.skelText(&skelBuf, xmlEscape(string(t)))
			}

		case xml.ProcInst:
			r.skelText(&skelBuf, "<?"+t.Target+" "+string(t.Inst)+"?>")
		}
	}

	// Flush remaining skeleton data
	r.skelFlush(&skelBuf)

	return nil
}

// Skeleton part-boundary markers compatible with the OpenXML pattern.
const (
	skelPartStartPrefix = "@@SKEL_PART_START@@"
	skelPartEndPrefix   = "@@SKEL_PART_END@@"
)

func (r *Reader) skelPartStart(partPath string) {
	if r.skeletonStore != nil {
		_ = r.skeletonStore.WriteRef(skelPartStartPrefix + partPath)
	}
}

func (r *Reader) skelPartEnd(partPath string) {
	if r.skeletonStore != nil {
		_ = r.skeletonStore.WriteRef(skelPartEndPrefix + partPath)
	}
}

func (r *Reader) skelText(buf *bytes.Buffer, s string) {
	if r.skeletonStore != nil {
		buf.WriteString(s)
	}
}

func (r *Reader) skelRef(buf *bytes.Buffer, id string) {
	if r.skeletonStore != nil {
		if buf.Len() > 0 {
			_ = r.skeletonStore.WriteText(buf.Bytes())
			buf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

func (r *Reader) skelFlush(buf *bytes.Buffer) {
	if r.skeletonStore != nil && buf.Len() > 0 {
		_ = r.skeletonStore.WriteText(buf.Bytes())
		buf.Reset()
	}
}

func (r *Reader) skelWriteStartElement(buf *bytes.Buffer, t xml.StartElement) {
	if r.skeletonStore == nil {
		return
	}
	buf.WriteString("<")
	writeElementName(buf, t.Name)
	for _, a := range t.Attr {
		buf.WriteString(" ")
		writeAttrName(buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
}

func (r *Reader) skelWriteEndElement(buf *bytes.Buffer, t xml.EndElement) {
	if r.skeletonStore == nil {
		return
	}
	buf.WriteString("</")
	writeElementName(buf, t.Name)
	buf.WriteString(">")
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}

// Helper functions

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func zipFileByName(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func attrVal(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func xmlEscape(s string) string {
	var buf strings.Builder
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func xmlEscapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func writeElementName(buf *bytes.Buffer, name xml.Name) {
	if name.Space != "" {
		prefix := nsPrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
	}
	buf.WriteString(name.Local)
}

func writeAttrName(buf *bytes.Buffer, name xml.Name) {
	if name.Space != "" {
		prefix := nsPrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
	}
	buf.WriteString(name.Local)
}

// nsPrefix returns a namespace prefix for known IDML namespaces.
func nsPrefix(ns string) string {
	switch ns {
	case "http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging":
		return "idPkg"
	case "http://www.w3.org/XML/1998/namespace":
		return "xml"
	default:
		return ""
	}
}
