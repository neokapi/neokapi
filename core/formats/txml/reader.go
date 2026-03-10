package txml

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for Trados XML (TXML) files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new TXML reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "txml",
			FormatDisplayName: "Trados XML",
			FormatMimeType:    "application/x-txml+xml",
			FormatExtensions:  []string{".txml"},
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
		MIMETypes:  []string{"application/x-txml+xml"},
		Extensions: []string{".txml"},
		Sniff: func(data []byte) bool {
			s := string(data)
			return strings.Contains(s, "<txml")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("txml: nil document or reader")
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

// elemPosition records the byte position of a <source> or <target> content region.
type elemPosition struct {
	startOffset int    // byte offset after start tag
	endOffset   int    // byte offset before end tag
	segIdx      int    // which segment (0-based)
	elemType    string // "source" or "target"
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("txml: reading: %w", err)}
		return
	}
	rawText := string(content)

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Parse root <txml> attributes for locale info
	var sourceLocale, targetLocale string
	decoder := xml.NewDecoder(strings.NewReader(rawText))
	decoder.Strict = false

	// Find the <txml> root element to get locale info
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "txml" {
			sourceLocale = attrVal(start.Attr, "locale")
			targetLocale = attrVal(start.Attr, "targetlocale")
			break
		}
	}

	if sourceLocale != "" {
		locale = model.LocaleID(sourceLocale)
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "txml",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-txml+xml",
		Properties: map[string]string{
			"target-locale": targetLocale,
		},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Re-parse from start for segments
	decoder = xml.NewDecoder(strings.NewReader(rawText))
	decoder.Strict = false

	blockCounter := 0
	inBody := false
	var elemPositions []elemPosition

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("txml: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "body":
				inBody = true
			case "segment":
				if !inBody {
					continue
				}
				blockCounter++
				segType := attrVal(t.Attr, "segtype")
				block := r.parseSegment(decoder, locale, model.LocaleID(targetLocale), blockCounter, segType, &elemPositions)
				if block != nil {
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "body" {
				inBody = false
			}
		}
	}

	// Build skeleton from collected positions
	if r.skeletonStore != nil && len(elemPositions) > 0 {
		skelPos := 0
		for _, ep := range elemPositions {
			if ep.startOffset > skelPos {
				r.skelText(rawText[skelPos:ep.startOffset])
			}
			refID := fmt.Sprintf("%d:%s", ep.segIdx, ep.elemType)
			r.skelRef(refID)
			skelPos = ep.endOffset
		}
		if skelPos < len(rawText) {
			r.skelText(rawText[skelPos:])
		}
		r.skelFlush()
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// parseSegment parses a <segment> element.
func (r *Reader) parseSegment(decoder *xml.Decoder, sourceLang, targetLang model.LocaleID, counter int, segType string, positions *[]elemPosition) *model.Block {
	var sourceText string
	var targetText string
	segIdx := counter - 1 // 0-based

	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return nil
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "source":
				startOff := decoder.InputOffset()
				sourceText = readElementContent(decoder)
				depth-- // readElementContent consumed end element

				if r.skeletonStore != nil {
					endOff := decoder.InputOffset()
					endTag := "</source>"
					endPos := int(endOff) - len(endTag)
					if endPos < 0 {
						endPos = 0
					}
					*positions = append(*positions, elemPosition{
						startOffset: int(startOff),
						endOffset:   endPos,
						segIdx:      segIdx,
						elemType:    "source",
					})
				}
			case "target":
				startOff := decoder.InputOffset()
				targetText = readElementContent(decoder)
				depth--

				if r.skeletonStore != nil {
					endOff := decoder.InputOffset()
					endTag := "</target>"
					endPos := int(endOff) - len(endTag)
					if endPos < 0 {
						endPos = 0
					}
					*positions = append(*positions, elemPosition{
						startOffset: int(startOff),
						endOffset:   endPos,
						segIdx:      segIdx,
						elemType:    "target",
					})
				}
			}
		case xml.EndElement:
			depth--
		}
	}

	if sourceText == "" {
		return nil
	}

	block := model.NewBlock(fmt.Sprintf("seg%d", counter), sourceText)
	block.Name = fmt.Sprintf("seg%d", counter)
	if segType != "" {
		block.Properties["segtype"] = segType
	}

	if targetText != "" && !targetLang.IsEmpty() {
		block.SetTargetText(targetLang, targetText)
	}

	return block
}

// readElementContent reads text content of an element, handling inline elements.
func readElementContent(decoder *xml.Decoder) string {
	var buf strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			// Skip inline elements like <bpt>, <ept>, <ph>, <it>
		case xml.EndElement:
			depth--
		case xml.CharData:
			buf.Write(t)
		}
	}
	return buf.String()
}

// attrVal returns the value of named attribute, or "".
func attrVal(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// skelText appends text to the skeleton buffer if active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		r.skelBuf.WriteString(s)
	}
}

// skelRef flushes buffered text and writes a block reference to the skeleton store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
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
