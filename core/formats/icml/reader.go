package icml

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

// Reader implements DataFormatReader for Adobe InCopy ICML files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new ICML reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "icml",
			FormatDisplayName: "ICML (Adobe InCopy)",
			FormatMimeType:    "application/x-icml+xml",
			FormatExtensions:  []string{".icml", ".wcml"},
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
		MIMETypes:  []string{"application/x-icml+xml"},
		Extensions: []string{".icml", ".wcml"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("icml: nil document or reader")
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

	// Read the full document.
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.emitError(ch, fmt.Errorf("icml: reading document: %w", err))
		return
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "icml",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-icml+xml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	if r.skeletonStore != nil {
		// Skeleton mode: track byte ranges and write skeleton entries.
		r.parseAndEmitSkeleton(ctx, ch, data)
	} else {
		// Legacy mode: emit raw document as Data for roundtrip reconstruction.
		docData := &model.Data{
			ID:   "d1",
			Name: "icml-document",
			Properties: map[string]string{
				"content": string(data),
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: docData}) {
			return
		}
		r.parseAndEmit(ctx, ch, data)
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// icmlContentRange records a byte range in the source for a Content element's text.
type icmlContentRange struct {
	blockID string
	start   int // byte offset inclusive
	end     int // byte offset exclusive
}

// parseAndEmitSkeleton walks the ICML XML, emits per-Content Blocks, and writes skeleton entries.
// Unlike parseAndEmit which aggregates Content texts per ParagraphStyleRange,
// skeleton mode emits one block per Content element for 1:1 skeleton ref mapping.
func (r *Reader) parseAndEmitSkeleton(ctx context.Context, ch chan<- model.PartResult, data []byte) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	blockCounter := 0

	inStory := false
	inContent := false
	nonTranslatableDepth := 0
	inTable := false
	noteDepth := 0
	paragraphStyle := ""

	// Note extraction (gated by cfg.ExtractNotes). Editorial <Note> content is
	// surfaced as a parity-safe NoteAnnotation on the next emitted Block rather
	// than as a translatable Block — the Note bytes stay in skeleton untouched.
	inNoteContent := false
	var noteText strings.Builder
	var pendingNotes []*model.NoteAnnotation

	var ranges []icmlContentRange

	for {
		offset := decoder.InputOffset()
		tok, err := decoder.Token()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				r.emitError(ch, fmt.Errorf("icml: parsing document: %w", err))
			}
			break
		}

		switch el := tok.(type) {
		case xml.StartElement:
			name := el.Name.Local

			if nonTranslatableElements[name] {
				nonTranslatableDepth++
				continue
			}
			if nonTranslatableDepth > 0 {
				continue
			}
			if noteDepth > 0 {
				if r.cfg.ExtractNotes && name == "Content" {
					inNoteContent = true
				}
				noteDepth++
				continue
			}

			switch name {
			case "Story":
				inStory = true
			case "ParagraphStyleRange":
				if inStory {
					paragraphStyle = attrValue(el, "AppliedParagraphStyle")
				}
			case "Table":
				inTable = true
			case "Note":
				noteDepth = 1
			case "Content":
				if inStory && nonTranslatableDepth == 0 && noteDepth == 0 {
					inContent = true
				}
			}

		case xml.EndElement:
			name := el.Name.Local

			if nonTranslatableElements[name] {
				nonTranslatableDepth--
				continue
			}
			if nonTranslatableDepth > 0 {
				continue
			}
			if noteDepth > 0 {
				if r.cfg.ExtractNotes && name == "Content" {
					inNoteContent = false
				}
				noteDepth--
				if noteDepth == 0 && r.cfg.ExtractNotes {
					if n := finalizeNote(&noteText); n != nil {
						pendingNotes = append(pendingNotes, n)
					}
				}
				continue
			}

			switch name {
			case "Story":
				inStory = false
			case "Table":
				inTable = false
			case "Content":
				inContent = false
			}

		case xml.CharData:
			if nonTranslatableDepth > 0 {
				continue
			}
			if noteDepth > 0 {
				if r.cfg.ExtractNotes && inNoteContent {
					noteText.WriteString(string(el))
				}
				continue
			}
			text := string(el)
			if inContent && strings.TrimSpace(text) != "" {
				endOffset := int(decoder.InputOffset())
				blockCounter++
				blockID := fmt.Sprintf("tu%d", blockCounter)
				block := model.NewBlock(blockID, text)
				if inTable {
					block.Name = fmt.Sprintf("cell.%d", blockCounter)
					block.Properties["table"] = "true"
				} else {
					block.Name = fmt.Sprintf("para.%d", blockCounter)
				}
				if paragraphStyle != "" {
					block.Properties["paragraphStyle"] = paragraphStyle
				}
				if r.cfg.ExtractNotes && len(pendingNotes) > 0 {
					for _, n := range pendingNotes {
						block.AddNote(n)
					}
					pendingNotes = pendingNotes[:0]
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}

				ranges = append(ranges, icmlContentRange{
					blockID: blockID,
					start:   int(offset),
					end:     endOffset,
				})
			}
			_ = offset
		}
	}

	r.writeSkeletonEntries(data, ranges)
}

// writeSkeletonEntries writes skeleton text and ref entries from the collected ranges.
func (r *Reader) writeSkeletonEntries(content []byte, ranges []icmlContentRange) {
	pos := 0
	for _, ref := range ranges {
		if ref.start > pos {
			_ = r.skeletonStore.WriteText(content[pos:ref.start])
		}
		_ = r.skeletonStore.WriteRef(ref.blockID)
		pos = ref.end
	}
	if pos < len(content) {
		_ = r.skeletonStore.WriteText(content[pos:])
	}
}

// nonTranslatableElements lists ICML elements whose content is not translatable.
var nonTranslatableElements = map[string]bool{
	"Properties":       true,
	"PathPointArray":   true,
	"PathPointType":    true,
	"PathGeometry":     true,
	"GeometryPathType": true,
}

// parseAndEmit walks the ICML XML and emits Blocks for translatable content.
func (r *Reader) parseAndEmit(ctx context.Context, ch chan<- model.PartResult, data []byte) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	blockCounter := 0

	// Track whether we are inside a Story element.
	inStory := false
	// Track nesting inside ParagraphStyleRange / CharacterStyleRange.
	inParagraphRange := false
	paragraphStyle := ""
	// Accumulate text segments from Content elements within a ParagraphStyleRange.
	var textSegments []string
	// Track whether we are inside a non-translatable element.
	nonTranslatableDepth := 0
	// Track whether we are inside a Table/Cell for separate TU handling.
	inTable := false
	inCell := false
	// Track Note elements (depth-based to handle nested elements inside Notes).
	noteDepth := 0

	// Note extraction (gated by cfg.ExtractNotes). Editorial <Note> content is
	// surfaced as a parity-safe NoteAnnotation on the next emitted Block rather
	// than as a translatable Block — the Note bytes stay in skeleton untouched.
	inNoteContent := false
	var noteText strings.Builder
	var pendingNotes []*model.NoteAnnotation
	attachPendingNotes := func(block *model.Block) {
		if !r.cfg.ExtractNotes || len(pendingNotes) == 0 {
			return
		}
		for _, n := range pendingNotes {
			block.AddNote(n)
		}
		pendingNotes = pendingNotes[:0]
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				r.emitError(ch, fmt.Errorf("icml: parsing document: %w", err))
			}
			break
		}

		switch el := tok.(type) {
		case xml.StartElement:
			name := el.Name.Local

			if nonTranslatableElements[name] {
				nonTranslatableDepth++
				continue
			}
			if nonTranslatableDepth > 0 {
				continue
			}

			// If inside a Note, just track depth (and capture its <Content> text
			// when note extraction is on).
			if noteDepth > 0 {
				if r.cfg.ExtractNotes && name == "Content" {
					inNoteContent = true
				}
				noteDepth++
				continue
			}

			switch name {
			case "Story":
				inStory = true

			case "ParagraphStyleRange":
				if inStory && !inTable {
					inParagraphRange = true
					textSegments = nil
					paragraphStyle = attrValue(el, "AppliedParagraphStyle")
				}

			case "Table":
				// Flush any accumulated text before the table.
				if inParagraphRange && len(textSegments) > 0 {
					text := joinSegments(textSegments)
					if text != "" {
						blockCounter++
						block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
						block.Name = fmt.Sprintf("para.%d", blockCounter)
						if paragraphStyle != "" {
							block.Properties["paragraphStyle"] = paragraphStyle
						}
						attachPendingNotes(block)
						if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
							return
						}
					}
					textSegments = nil
				}
				inTable = true

			case "Cell":
				if inTable {
					inCell = true
					textSegments = nil
				}

			case "Note":
				noteDepth = 1

			case "Br":
				if inParagraphRange {
					if r.cfg.NewTUOnBr && len(textSegments) > 0 {
						// Emit current accumulated text as a block.
						text := joinSegments(textSegments)
						if text != "" {
							blockCounter++
							block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
							block.Name = fmt.Sprintf("para.%d", blockCounter)
							if paragraphStyle != "" {
								block.Properties["paragraphStyle"] = paragraphStyle
							}
							attachPendingNotes(block)
							if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
								return
							}
						}
						textSegments = nil
					}
					// If not newTuOnBr, the <Br/> is just ignored (text continues).
				}
			}

		case xml.EndElement:
			name := el.Name.Local

			if nonTranslatableElements[name] {
				nonTranslatableDepth--
				continue
			}
			if nonTranslatableDepth > 0 {
				continue
			}

			// If inside a Note, just track depth; on close, drain the captured
			// note text into a pending NoteAnnotation.
			if noteDepth > 0 {
				if r.cfg.ExtractNotes && name == "Content" {
					inNoteContent = false
				}
				noteDepth--
				if noteDepth == 0 && r.cfg.ExtractNotes {
					if n := finalizeNote(&noteText); n != nil {
						pendingNotes = append(pendingNotes, n)
					}
				}
				continue
			}

			switch name {
			case "Story":
				inStory = false

			case "ParagraphStyleRange":
				if inParagraphRange && !inTable {
					// Emit accumulated text as a block.
					text := joinSegments(textSegments)
					if text != "" {
						blockCounter++
						block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
						block.Name = fmt.Sprintf("para.%d", blockCounter)
						if paragraphStyle != "" {
							block.Properties["paragraphStyle"] = paragraphStyle
						}
						attachPendingNotes(block)
						if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
							return
						}
					}
					textSegments = nil
				}
				inParagraphRange = false

			case "Cell":
				if inCell {
					// Emit cell content as a separate block.
					text := joinSegments(textSegments)
					if text != "" {
						blockCounter++
						block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
						block.Name = fmt.Sprintf("cell.%d", blockCounter)
						block.Properties["table"] = "true"
						attachPendingNotes(block)
						if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
							return
						}
					}
					textSegments = nil
					inCell = false
				}

			case "Table":
				inTable = false
			}

		case xml.CharData:
			if nonTranslatableDepth > 0 {
				continue
			}
			if noteDepth > 0 {
				if r.cfg.ExtractNotes && inNoteContent {
					noteText.WriteString(string(el))
				}
				continue
			}
			text := string(el)
			if inStory && (inParagraphRange || inCell) && strings.TrimSpace(text) != "" {
				textSegments = append(textSegments, text)
			}
		}
	}
}

// finalizeNote drains the accumulated <Note> text into a NoteAnnotation,
// resetting the buffer. It returns nil when the note carried no text. ICML
// Notes are editorial annotations, so the text rides along as block-scoped
// metadata (parity-safe; not in the canonical part stream) rather than as a
// translatable Block.
func finalizeNote(buf *strings.Builder) *model.NoteAnnotation {
	txt := strings.TrimSpace(buf.String())
	buf.Reset()
	if txt == "" {
		return nil
	}
	return &model.NoteAnnotation{
		Text:      txt,
		From:      "icml",
		Annotates: "general",
	}
}

// joinSegments joins text segments, trimming leading/trailing whitespace
// from each segment but preserving internal spacing.
func joinSegments(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	return strings.Join(segments, "")
}

// attrValue returns the value of a named attribute from a start element.
func attrValue(el xml.StartElement, name string) string {
	for _, attr := range el.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *Reader) emitError(ch chan<- model.PartResult, err error) {
	ch <- model.PartResult{Error: err}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
