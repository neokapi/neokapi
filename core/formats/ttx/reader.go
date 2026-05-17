package ttx

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/encoding"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Trados TagEditor TTX files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new TTX reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "ttx",
			FormatDisplayName: "Trados TagEditor TTX",
			FormatMimeType:    "application/x-ttx+xml",
			FormatExtensions:  []string{".ttx"},
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
		MIMETypes:  []string{"application/x-ttx+xml"},
		Extensions: []string{".ttx"},
		Sniff: func(data []byte) bool {
			// Trados emits .ttx as UTF-16 LE with BOM by convention,
			// so a raw UTF-8 substring check misses every native
			// Trados file. Transcode via BOM detection before
			// scanning for the root element.
			text, _, err := encoding.ToUTF8(data)
			if err != nil {
				return false
			}
			return strings.Contains(string(text), "<TRADOStag")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("ttx: nil document or reader")
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

// segPosition records the byte position of a <Seg> content region.
type segPosition struct {
	startOffset int // byte offset after <Seg> start tag
	endOffset   int // byte offset before </Seg> end tag
	tuIdx       int // which TU (0-based)
	tuvIdx      int // which TUV within TU (0=source, 1=target)
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("ttx: reading: %w", err)}
		return
	}
	// Trados writes .ttx as UTF-16 LE with a BOM; UTF-8 with a BOM
	// shows up too. Transcode to BOM-stripped UTF-8 before parsing.
	decoded, detectedEnc, err := encoding.ToUTF8(content)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("ttx: decoding %s: %w", detectedEnc, err)}
		return
	}
	rawText := string(decoded)

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Surface the detected on-disk encoding on the Layer so downstream
	// stages (including the writer) can re-emit in the same encoding
	// without losing the Trados convention. Caller-provided Encoding
	// wins when set.
	layerEncoding := r.Doc.Encoding
	if layerEncoding == "" {
		layerEncoding = detectedEnc
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "ttx",
		Locale:   locale,
		Encoding: layerEncoding,
		MimeType: "application/x-ttx+xml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Determine effective segment mode
	mode := r.cfg.SegmentMode
	includeUnsegmented := false
	if mode == SegmentModeAll {
		includeUnsegmented = true
	} else if mode == SegmentModeAuto {
		// Auto-detect: scan for Tu elements first
		preDecoder := xml.NewDecoder(strings.NewReader(rawText))
		preDecoder.Strict = false
		hasTu := false
		for {
			ptok, perr := preDecoder.Token()
			if perr != nil {
				break
			}
			if start, ok := ptok.(xml.StartElement); ok && start.Name.Local == "Tu" {
				hasTu = true
				break
			}
		}
		// If no Tu elements found, extract all text
		includeUnsegmented = !hasTu
	}

	decoder := xml.NewDecoder(strings.NewReader(rawText))
	decoder.Strict = false

	blockCounter := 0
	tuCount := 0
	inRaw := false

	var segPositions []segPosition
	var unsegmentedText strings.Builder

	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("ttx: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "Raw":
				inRaw = true
			case "Tu":
				// Flush any unsegmented text before a Tu
				if includeUnsegmented && inRaw {
					text := strings.TrimSpace(unsegmentedText.String())
					if text != "" {
						blockCounter++
						block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
						block.Name = fmt.Sprintf("tu%d", blockCounter)
						block.Properties["unsegmented"] = "true"
						if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
							return
						}
					}
					unsegmentedText.Reset()
				}

				blockCounter++
				matchPercent := attrVal(t.Attr, "MatchPercent")
				var segs []segPosition
				block := r.parseTransUnitWithSkeleton(decoder, locale, blockCounter, matchPercent, tuCount, &segs)
				segPositions = append(segPositions, segs...)
				tuCount++
				if block != nil {
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "Raw" {
				// Flush trailing unsegmented text
				if includeUnsegmented {
					text := strings.TrimSpace(unsegmentedText.String())
					if text != "" {
						blockCounter++
						block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
						block.Name = fmt.Sprintf("tu%d", blockCounter)
						block.Properties["unsegmented"] = "true"
						if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
							return
						}
					}
					unsegmentedText.Reset()
				}
				inRaw = false
			}
		case xml.CharData:
			if includeUnsegmented && inRaw {
				unsegmentedText.Write(t)
			}
		}
	}

	// Build skeleton from collected seg positions
	if r.skeletonStore != nil && len(segPositions) > 0 {
		skelPos := 0
		for _, sp := range segPositions {
			if sp.startOffset > skelPos {
				r.skelText(rawText[skelPos:sp.startOffset])
			}
			refID := fmt.Sprintf("%d:%d", sp.tuIdx, sp.tuvIdx)
			r.skelRef(refID)
			skelPos = sp.endOffset
		}
		if skelPos < len(rawText) {
			r.skelText(rawText[skelPos:])
		}
		r.skelFlush()
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// parseTransUnitWithSkeleton parses a <Tu> element, collecting seg positions for skeleton.
func (r *Reader) parseTransUnitWithSkeleton(decoder *xml.Decoder, sourceLocale model.LocaleID, counter int, matchPercent string, tuIdx int, segs *[]segPosition) *model.Block {
	var sourceText string
	var targetText string
	var targetLang model.LocaleID
	var sourceLang model.LocaleID
	tuvIdx := 0

	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return nil
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "Tuv" {
				lang := model.LocaleID(attrVal(t.Attr, "Lang"))
				segText := r.parseTuvWithSkeleton(decoder, tuIdx, tuvIdx, segs)
				depth-- // parseTuv consumed end element

				if sourceLang.IsEmpty() {
					sourceLang = lang
					sourceText = segText
				} else {
					targetLang = lang
					targetText = segText
				}
				tuvIdx++
			}
		case xml.EndElement:
			depth--
		}
	}

	if sourceText == "" {
		return nil
	}

	block := model.NewBlock(fmt.Sprintf("tu%d", counter), sourceText)
	block.Name = fmt.Sprintf("tu%d", counter)
	if matchPercent != "" {
		block.Properties["match-percent"] = matchPercent
	}
	if !sourceLang.IsEmpty() {
		block.Properties["source-lang"] = string(sourceLang)
	}

	if targetText != "" && !targetLang.IsEmpty() {
		block.SetTargetText(targetLang, targetText)
	}

	return block
}

// parseTuvWithSkeleton parses a <Tuv> element, recording seg positions.
func (r *Reader) parseTuvWithSkeleton(decoder *xml.Decoder, tuIdx, tuvIdx int, segs *[]segPosition) string {
	depth := 1
	var segText string

	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return ""
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "Seg" {
				segStartOff := decoder.InputOffset()
				segText = readSegContent(decoder)
				depth-- // readSegContent consumed end element

				if r.skeletonStore != nil {
					endOff := decoder.InputOffset()
					segEndTag := "</Seg>"
					segEndPos := int(endOff) - len(segEndTag)
					if segEndPos < 0 {
						segEndPos = 0
					}
					*segs = append(*segs, segPosition{
						startOffset: int(segStartOff),
						endOffset:   segEndPos,
						tuIdx:       tuIdx,
						tuvIdx:      tuvIdx,
					})
				}
			}
		case xml.EndElement:
			depth--
		}
	}

	return segText
}

// readSegContent reads the text content of a <Seg> element, handling inline tags.
func readSegContent(decoder *xml.Decoder) string {
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
			// Skip inline elements like <ut>, <df>, <it> — just read their text content
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
