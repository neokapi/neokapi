package ts

import (
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

// Reader implements DataFormatReader for Qt TS (Qt Linguist) files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new Qt TS reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "ts",
			FormatDisplayName: "Qt TS",
			FormatMimeType:    "application/x-ts",
			FormatExtensions:  []string{".ts"},
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
		MIMETypes:  []string{"application/x-ts", "application/x-linguist"},
		Extensions: []string{}, // Don't auto-detect .ts (conflicts with TypeScript)
		Sniff: func(data []byte) bool {
			return bytes.Contains(data, []byte("<TS")) && bytes.Contains(data, []byte("</TS>"))
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("ts: nil document or reader")
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

// readContent uses streaming XML parsing to handle Qt TS features.
func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("ts: reading: %w", err)}
		return
	}
	rawText := string(content)

	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	// Skeleton tracking: collect source/translation content positions
	type elemPos struct {
		startOffset int    // byte offset after opening tag
		endOffset   int    // byte offset before closing tag
		blockIdx    int    // 0-based block index
		elemType    string // "source" or "translation"
	}
	var elemPositions []elemPos
	var elemStartOff int64

	var (
		tsVersion           string
		tsLanguage          string
		tsSrcLanguage       string
		blockCount          int
		contextName         string
		contextCount        int
		inContext           bool
		inMessage           bool
		inSource            bool
		inTranslation       bool
		inComment           bool
		inExtraComment      bool
		inTransComment      bool
		inNumerusForm       bool
		inContextName       bool
		messageID           string
		messageNumerus      bool
		transType           string
		sourceBuilder       strings.Builder
		transBuilder        strings.Builder
		commentBuilder      strings.Builder
		extraCommentBuilder strings.Builder
		transCommentBuilder strings.Builder
		contextNameBuilder  strings.Builder
		numerusForms        []string
		sourceFrag          *model.Fragment
		sourceByteElems     []byteElem
		transByteElems      []byteElem
		buildingSourceFrag  bool
		buildingTransFrag   bool
		transFrag           *model.Fragment
	)

	layer := &model.Layer{
		ID:             "doc1",
		Name:           r.Doc.URI,
		Format:         "ts",
		Locale:         locale,
		Encoding:       r.Doc.Encoding,
		MimeType:       "application/x-ts",
		IsMultilingual: true,
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("ts: parsing: %w", err)}
			return
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "TS":
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "version":
						tsVersion = attr.Value
					case "language":
						tsLanguage = attr.Value
					case "sourcelanguage":
						tsSrcLanguage = attr.Value
					}
				}
				// Emit TS metadata as Data part
				dataProps := map[string]string{
					"version":        tsVersion,
					"language":       tsLanguage,
					"sourcelanguage": tsSrcLanguage,
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: &model.Data{
					ID:         "d1",
					Name:       "ts-header",
					Properties: dataProps,
				}}) {
					return
				}

			case "context":
				inContext = true
				contextCount++
				contextName = ""
				contextNameBuilder.Reset()

			case "name":
				if inContext && !inMessage {
					inContextName = true
					contextNameBuilder.Reset()
				}

			case "message":
				inMessage = true
				messageID = ""
				messageNumerus = false
				transType = ""
				sourceBuilder.Reset()
				transBuilder.Reset()
				commentBuilder.Reset()
				extraCommentBuilder.Reset()
				transCommentBuilder.Reset()
				numerusForms = nil
				sourceFrag = nil
				transFrag = nil
				sourceByteElems = nil
				transByteElems = nil
				buildingSourceFrag = false
				buildingTransFrag = false
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "id":
						messageID = attr.Value
					case "numerus":
						messageNumerus = attr.Value == "yes"
					}
				}

			case "source":
				if inMessage {
					inSource = true
					sourceBuilder.Reset()
					sourceFrag = &model.Fragment{}
					sourceByteElems = nil
					buildingSourceFrag = true
					elemStartOff = decoder.InputOffset()
				}

			case "translation":
				if inMessage {
					inTranslation = true
					transBuilder.Reset()
					transFrag = &model.Fragment{}
					transByteElems = nil
					buildingTransFrag = true
					for _, attr := range t.Attr {
						if attr.Name.Local == "type" {
							transType = attr.Value
						}
					}
					elemStartOff = decoder.InputOffset()
				}

			case "numerusform":
				if inTranslation && messageNumerus {
					inNumerusForm = true
					transBuilder.Reset()
				}

			case "comment":
				if inMessage && !inSource && !inTranslation {
					inComment = true
					commentBuilder.Reset()
				}

			case "extracomment":
				if inMessage && !inSource && !inTranslation {
					inExtraComment = true
					extraCommentBuilder.Reset()
				}

			case "translatorcomment":
				if inMessage && !inSource && !inTranslation {
					inTransComment = true
					transCommentBuilder.Reset()
				}

			case "byte":
				if inSource || inTranslation {
					var byteVal string
					for _, attr := range t.Attr {
						if attr.Name.Local == "value" {
							byteVal = attr.Value
						}
					}
					be := byteElem{value: byteVal}
					if buildingSourceFrag && inSource {
						sourceByteElems = append(sourceByteElems, be)
						// Add a placeholder span for the byte element
						sourceFrag.AppendSpan(&model.Span{
							SpanType: model.SpanPlaceholder,
							ID:       fmt.Sprintf("b%d", len(sourceByteElems)),
							Type:     "byte",
							Data:     fmt.Sprintf(`<byte value="%s"/>`, byteVal),
						})
					} else if buildingTransFrag && inTranslation && !inNumerusForm {
						transByteElems = append(transByteElems, be)
						transFrag.AppendSpan(&model.Span{
							SpanType: model.SpanPlaceholder,
							ID:       fmt.Sprintf("b%d", len(transByteElems)),
							Type:     "byte",
							Data:     fmt.Sprintf(`<byte value="%s"/>`, byteVal),
						})
					}
				}
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "name":
				if inContextName {
					inContextName = false
					contextName = contextNameBuilder.String()
					// Emit GroupStart for context
					if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{
						ID:   fmt.Sprintf("ctx%d", contextCount),
						Name: contextName,
						Type: "context",
						Properties: map[string]string{
							"name": contextName,
						},
					}}) {
						return
					}
				}

			case "context":
				if inContext {
					inContext = false
					// Emit GroupEnd
					if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{
						ID: fmt.Sprintf("ctx%d", contextCount),
					}}) {
						return
					}
				}

			case "source":
				if inSource {
					if r.skeletonStore != nil {
						endOff := decoder.InputOffset()
						closeTag := "</source>"
						endPos := int(endOff) - len(closeTag)
						if endPos < 0 {
							endPos = 0
						}
						elemPositions = append(elemPositions, elemPos{
							startOffset: int(elemStartOff),
							endOffset:   endPos,
							blockIdx:    blockCount, // will be incremented when </message> fires
							elemType:    "source",
						})
					}
					inSource = false
					buildingSourceFrag = false
				}

			case "translation":
				if inTranslation {
					if r.skeletonStore != nil && !messageNumerus {
						endOff := decoder.InputOffset()
						closeTag := "</translation>"
						endPos := int(endOff) - len(closeTag)
						if endPos < 0 {
							endPos = 0
						}
						elemPositions = append(elemPositions, elemPos{
							startOffset: int(elemStartOff),
							endOffset:   endPos,
							blockIdx:    blockCount,
							elemType:    "translation",
						})
					}
					inTranslation = false
					buildingTransFrag = false
				}

			case "numerusform":
				if inNumerusForm {
					inNumerusForm = false
					numerusForms = append(numerusForms, transBuilder.String())
					transBuilder.Reset()
				}

			case "comment":
				if inComment {
					inComment = false
				}

			case "extracomment":
				if inExtraComment {
					inExtraComment = false
				}

			case "translatorcomment":
				if inTransComment {
					inTransComment = false
				}

			case "message":
				if inMessage {
					inMessage = false
					blockCount++

					// Determine block ID
					blockID := messageID
					if blockID == "" {
						blockID = fmt.Sprintf("tu%d", blockCount)
					}

					// Build source text
					sourceText := sourceBuilder.String()

					// Build block
					var block *model.Block
					if sourceFrag != nil && sourceFrag.HasSpans() {
						// Source has inline elements (byte codes)
						sourceFrag.AppendText("") // ensure coded text is set
						block = &model.Block{
							ID:           blockID,
							Name:         contextName,
							Translatable: transType != "obsolete",
							Source:       []*model.Segment{{ID: "s1", Content: sourceFrag}},
							Targets:      make(map[model.LocaleID][]*model.Segment),
							Properties:   make(map[string]string),
							Annotations:  make(map[string]model.Annotation),
						}
					} else {
						block = model.NewBlock(blockID, sourceText)
						block.Name = contextName
						if transType == "obsolete" {
							block.Translatable = false
						}
					}

					// Store translation type
					if transType != "" {
						block.Properties["type"] = transType
					}

					// Store context name
					if contextName != "" {
						block.Properties["context"] = contextName
					}

					// Store numerus flag
					if messageNumerus {
						block.Properties["numerus"] = "yes"
					}

					// Set target locale
					targetLocale := model.LocaleID(tsLanguage)
					if targetLocale == "" {
						targetLocale = r.Doc.SourceLocale
					}

					// Set target text
					if messageNumerus && len(numerusForms) > 0 {
						// Store plural forms as properties
						for i, form := range numerusForms {
							block.Properties[fmt.Sprintf("numerusform:%d", i)] = form
						}
						// Set first form as target text
						block.SetTargetText(targetLocale, numerusForms[0])
					} else {
						targetText := transBuilder.String()
						if targetText != "" || transType == "unfinished" {
							if transFrag != nil && transFrag.HasSpans() {
								block.SetTargetFragment(targetLocale, transFrag)
							} else {
								block.SetTargetText(targetLocale, targetText)
							}
						}
					}

					// Store comments as annotations
					var noteText string
					comment := commentBuilder.String()
					extraComment := extraCommentBuilder.String()
					transComment := transCommentBuilder.String()

					var noteParts []string
					if comment != "" {
						noteParts = append(noteParts, comment)
						block.Properties["comment"] = comment
					}
					if extraComment != "" {
						noteParts = append(noteParts, extraComment)
						block.Properties["extracomment"] = extraComment
					}
					if transComment != "" {
						noteParts = append(noteParts, transComment)
						block.Properties["translatorcomment"] = transComment
					}
					if len(noteParts) > 0 {
						noteText = strings.Join(noteParts, "\n")
						block.Annotations["note"] = &model.NoteAnnotation{
							Text: noteText,
						}
					}

					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
				}
			}

		case xml.CharData:
			text := string(t)
			if inContextName {
				contextNameBuilder.WriteString(text)
			} else if inNumerusForm {
				transBuilder.WriteString(text)
			} else if inSource {
				sourceBuilder.WriteString(text)
				if buildingSourceFrag {
					sourceFrag.AppendText(text)
				}
			} else if inTranslation {
				if !inNumerusForm {
					transBuilder.WriteString(text)
					if buildingTransFrag {
						transFrag.AppendText(text)
					}
				}
			} else if inComment {
				commentBuilder.WriteString(text)
			} else if inExtraComment {
				extraCommentBuilder.WriteString(text)
			} else if inTransComment {
				transCommentBuilder.WriteString(text)
			}
		}
	}

	// Build skeleton from collected element positions
	if r.skeletonStore != nil && len(elemPositions) > 0 {
		skelPos := 0
		for _, ep := range elemPositions {
			// Write skeleton text from skelPos to element content start
			if ep.startOffset > skelPos {
				r.skelText(rawText[skelPos:ep.startOffset])
			}
			// Write skeleton ref: "blockIdx:elemType"
			refID := fmt.Sprintf("%d:%s", ep.blockIdx, ep.elemType)
			r.skelRef(refID)
			skelPos = ep.endOffset
		}
		// Write remaining skeleton text
		if skelPos < len(rawText) {
			r.skelText(rawText[skelPos:])
		}
		r.skelFlush()
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// byteElem holds a <byte value="xx"/> element.
type byteElem struct {
	value string // hex or decimal value
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
