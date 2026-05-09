package rtf

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for RTF files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new RTF reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "rtf",
			FormatDisplayName: "Rich Text Format",
			FormatMimeType:    "application/rtf",
			FormatExtensions:  []string{".rtf"},
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
		MIMETypes:  []string{"application/rtf", "text/rtf"},
		Extensions: []string{".rtf"},
		MagicBytes: [][]byte{[]byte("{\\rtf")},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("rtf: nil document or reader")
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

// token represents a parsed RTF token.
type token struct {
	typ       tokenType
	text      string // For text tokens: the actual text. For control: the control word.
	param     int    // Numeric parameter for control words (-1 if none).
	hasParam  bool
	byteStart int // byte offset of this token in the raw input
	byteEnd   int // byte offset past the end of this token
}

type tokenType int

const (
	tokenText       tokenType = iota // Plain text content
	tokenGroupStart                  // {
	tokenGroupEnd                    // }
	tokenControl                     // \keyword or \keywordN
	tokenHex                         // \'HH
	tokenUnicode                     // \uN
)

// alwaysSkipDestinations are RTF destinations that are always non-translatable.
var alwaysSkipDestinations = map[string]bool{
	"fonttbl":    true,
	"colortbl":   true,
	"stylesheet": true,
	"info":       true,
	"pict":       true,
	"object":     true,
	"fldinst":    true,
	"xe":         true,
	"tc":         true,
	"rxe":        true,
	"field":      false, // field itself is not skipped; fldinst inside is
	"fldrslt":    false,
}

// headerFooterDestinations are RTF destinations for headers/footers.
var headerFooterDestinations = map[string]bool{
	"header":  true,
	"headerl": true,
	"headerr": true,
	"headerf": true,
	"footer":  true,
	"footerl": true,
	"footerr": true,
	"footerf": true,
}

// annotationDestinations are RTF destinations for comments/annotations.
var annotationDestinations = map[string]bool{
	"atnid":      true,
	"atnauthor":  true,
	"annotation": true,
}

// bookmarkDestinations are RTF destinations for bookmarks.
var bookmarkDestinations = map[string]bool{
	"bkmkstart": true,
	"bkmkend":   true,
}

// isSkipDestination returns whether the given destination should be skipped
// based on the reader config.
func (r *Reader) isSkipDestination(dest string) bool {
	if skip, ok := alwaysSkipDestinations[dest]; ok {
		return skip
	}
	if headerFooterDestinations[dest] {
		return !r.cfg.ExtractHeadersFooters
	}
	if annotationDestinations[dest] {
		return !r.cfg.ExtractAnnotations
	}
	if bookmarkDestinations[dest] {
		return !r.cfg.ExtractBookmarks
	}
	return false
}

// textRef records the byte position of a text token and its block association.
type textRef struct {
	startOffset int // byte offset of the text content in raw input
	endOffset   int // byte offset past the text content
	blockIdx    int // which block (0-based)
	tokenIdx    int // index of this text token within the block (0-based)
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "rtf",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/rtf",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.emitErr(ctx, ch, fmt.Errorf("rtf: read error: %w", err))
		return
	}
	rawText := string(data)

	tokens := tokenize(data)
	var textRefs []textRef
	r.emitParts(ctx, ch, tokens, &textRefs)

	// Build skeleton if needed
	if r.skeletonStore != nil && len(textRefs) > 0 {
		skelPos := 0
		for _, tr := range textRefs {
			if tr.startOffset > skelPos {
				r.skelText(rawText[skelPos:tr.startOffset])
			}
			// Ref format: "blockIdx:tokenIdx:originalLen"
			// originalLen is the length of the original raw token so the writer
			// knows how many characters of the block text to assign here.
			origLen := tr.endOffset - tr.startOffset
			refID := fmt.Sprintf("%d:%d:%d", tr.blockIdx, tr.tokenIdx, origLen)
			r.skelRef(refID)
			skelPos = tr.endOffset
		}
		if skelPos < len(rawText) {
			r.skelText(rawText[skelPos:])
		}
		r.skelFlush()
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// emitParts walks the token stream and emits Part events.
func (r *Reader) emitParts(ctx context.Context, ch chan<- model.PartResult, tokens []token, textRefs *[]textRef) {
	blockCounter := 0
	dataCounter := 0

	// Track group depth and which groups to skip.
	type groupInfo struct {
		skip           bool
		destinationTag string
		prevInBody     bool // saved inBody state to restore on group end
		setInBody      bool // did this group set inBody?
	}
	var groupStack []groupInfo
	depth := 0

	// Accumulate text for the current paragraph.
	var paraText strings.Builder
	// Accumulate text token byte ranges for the current paragraph.
	type tokenRange struct {
		start, end int
	}
	var paraTokenRanges []tokenRange
	// Accumulate raw RTF for data parts.
	var rawRTF strings.Builder
	inBody := false

	flushData := func() {
		if rawRTF.Len() > 0 {
			dataCounter++
			d := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: fmt.Sprintf("rtf-structure.%d", dataCounter),
				Properties: map[string]string{
					"raw": rawRTF.String(),
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
				return
			}
			rawRTF.Reset()
		}
	}

	flushParagraph := func() {
		text := paraText.String()
		ranges := paraTokenRanges
		paraText.Reset()
		paraTokenRanges = nil
		if strings.TrimSpace(text) == "" {
			return
		}
		flushData()
		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
		block.Name = fmt.Sprintf("para.%d", blockCounter)
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})

		// Record text references for skeleton
		if r.skeletonStore != nil {
			for i, tr := range ranges {
				*textRefs = append(*textRefs, textRef{
					startOffset: tr.start,
					endOffset:   tr.end,
					blockIdx:    blockCounter - 1, // 0-based
					tokenIdx:    i,
				})
			}
		}
	}

	shouldSkip := func() bool {
		for i := len(groupStack) - 1; i >= 0; i-- {
			if groupStack[i].skip {
				return true
			}
		}
		return false
	}

	for _, tok := range tokens {
		switch tok.typ {
		case tokenGroupStart:
			depth++
			groupStack = append(groupStack, groupInfo{})

		case tokenGroupEnd:
			if depth > 0 {
				depth--
				gi := groupStack[len(groupStack)-1]
				if gi.setInBody {
					flushParagraph()
					inBody = gi.prevInBody
				}
				groupStack = groupStack[:len(groupStack)-1]
			}

		case tokenControl:
			// Check if this is a destination control word at the start of a group.
			if len(groupStack) > 0 {
				gi := &groupStack[len(groupStack)-1]
				if gi.destinationTag == "" {
					gi.destinationTag = tok.text
					if r.isSkipDestination(tok.text) {
						gi.skip = true
						continue
					}
					// For header/footer/annotation groups that are NOT skipped,
					// set inBody so their text content is extracted.
					if headerFooterDestinations[tok.text] || annotationDestinations[tok.text] {
						gi.prevInBody = inBody
						gi.setInBody = true
						inBody = true
					}
				}
			}

			if shouldSkip() {
				continue
			}

			switch tok.text {
			case "par", "line":
				// Paragraph or line break - flush current paragraph.
				flushParagraph()
				inBody = true
			case "pard":
				// Paragraph reset - signals we're in the document body.
				inBody = true
			case "tab":
				if inBody {
					paraText.WriteRune('\t')
					paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
				}
			case "lquote":
				if inBody {
					paraText.WriteRune('\u2018')
					paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
				}
			case "rquote":
				if inBody {
					paraText.WriteRune('\u2019')
					paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
				}
			case "ldblquote":
				if inBody {
					paraText.WriteRune('\u201C')
					paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
				}
			case "rdblquote":
				if inBody {
					paraText.WriteRune('\u201D')
					paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
				}
			case "emdash":
				if inBody {
					paraText.WriteRune('\u2014')
					paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
				}
			case "endash":
				if inBody {
					paraText.WriteRune('\u2013')
					paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
				}
			case "bullet":
				if inBody {
					paraText.WriteRune('\u2022')
					paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
				}
			default:
				// Store formatting control words as raw RTF data.
				if !inBody {
					rawRTF.WriteString("\\")
					rawRTF.WriteString(tok.text)
					if tok.hasParam {
						rawRTF.WriteString(strconv.Itoa(tok.param))
					}
				}
			}

		case tokenText:
			if shouldSkip() {
				continue
			}
			if inBody || depth <= 1 {
				inBody = true
				paraText.WriteString(tok.text)
				paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
			} else {
				rawRTF.WriteString(tok.text)
			}

		case tokenHex:
			if shouldSkip() {
				continue
			}
			if inBody {
				paraText.WriteString(tok.text)
				paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
			}

		case tokenUnicode:
			if shouldSkip() {
				continue
			}
			if inBody {
				paraText.WriteString(tok.text)
				paraTokenRanges = append(paraTokenRanges, tokenRange{tok.byteStart, tok.byteEnd})
			}
		}
	}

	// Flush any remaining paragraph text.
	flushParagraph()
	flushData()
}

// tokenize converts raw RTF bytes into a stream of tokens with byte offsets.
func tokenize(data []byte) []token {
	var tokens []token
	rd := bufio.NewReader(strings.NewReader(string(data)))
	pos := 0 // current byte position

	for {
		b, err := rd.ReadByte()
		if err != nil {
			break
		}

		startPos := pos
		pos++

		switch b {
		case '{':
			tokens = append(tokens, token{typ: tokenGroupStart, byteStart: startPos, byteEnd: pos})
		case '}':
			tokens = append(tokens, token{typ: tokenGroupEnd, byteStart: startPos, byteEnd: pos})
		case '\\':
			tok := parseControlWord(rd, &pos)
			tok.byteStart = startPos
			tok.byteEnd = pos
			tokens = append(tokens, tok)
		case '\r', '\n':
			// RTF ignores CR/LF outside of control words.
			continue
		default:
			// Plain text - accumulate until a special character.
			var text strings.Builder
			text.WriteByte(b)
			for {
				b2, err := rd.ReadByte()
				if err != nil {
					break
				}
				pos++
				if b2 == '{' || b2 == '}' || b2 == '\\' || b2 == '\r' || b2 == '\n' {
					_ = rd.UnreadByte()
					pos--
					break
				}
				text.WriteByte(b2)
			}
			tokens = append(tokens, token{typ: tokenText, text: text.String(), byteStart: startPos, byteEnd: pos})
		}
	}

	return tokens
}

// skipUnicodeFallback consumes count ANSI fallback "characters" that follow
// a \uN unicode escape per the RTF spec. By default (\uc1) one fallback
// character follows. A fallback "character" is one of:
//   - a single literal byte (typical case: '?'), OR
//   - a \'HH hex escape (4 bytes), OR
//   - a \keyword control word, OR
//   - a balanced {...} group.
//
// Without this, the fallback bytes leak through the tokenizer as plain
// text, producing artifacts like "\u171?" being read as "«?".
func skipUnicodeFallback(rd *bufio.Reader, pos *int, count int) {
	for range count {
		b, err := rd.ReadByte()
		if err != nil {
			return
		}
		*pos++
		switch b {
		case '\\':
			next, err := rd.ReadByte()
			if err != nil {
				return
			}
			*pos++
			switch {
			case next == '\'':
				// \'HH — consume two hex digits.
				if _, err := rd.ReadByte(); err != nil {
					return
				}
				*pos++
				if _, err := rd.ReadByte(); err != nil {
					return
				}
				*pos++
			case (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z'):
				// \keyword[N][ ] — consume the rest of the control word.
				for {
					c, err := rd.ReadByte()
					if err != nil {
						return
					}
					*pos++
					if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
						(c >= '0' && c <= '9') || c == '-' {
						continue
					}
					if c != ' ' {
						_ = rd.UnreadByte()
						*pos--
					}
					break
				}
			default:
				// \<single-char> escape — already consumed both bytes.
			}
		case '{':
			// Skip a balanced group.
			depth := 1
			for depth > 0 {
				c, err := rd.ReadByte()
				if err != nil {
					return
				}
				*pos++
				switch c {
				case '\\':
					// Skip the next byte to avoid mis-counting \{ or \}.
					if _, err := rd.ReadByte(); err != nil {
						return
					}
					*pos++
				case '{':
					depth++
				case '}':
					depth--
				}
			}
		default:
			// Single-byte literal fallback — already consumed.
		}
	}
}

// parseControlWord reads a control word or special escape after '\'.
func parseControlWord(rd *bufio.Reader, pos *int) token {
	b, err := rd.ReadByte()
	if err != nil {
		return token{typ: tokenText, text: "\\"}
	}
	*pos++

	// Special characters.
	switch b {
	case '{', '}', '\\':
		return token{typ: tokenText, text: string(b)}
	case '~':
		// Non-breaking space.
		return token{typ: tokenText, text: "\u00A0"}
	case '-':
		// Optional hyphen.
		return token{typ: tokenText, text: "\u00AD"}
	case '_':
		// Non-breaking hyphen.
		return token{typ: tokenText, text: "\u2011"}
	case '*':
		// Ignorable destination - handled by group logic.
		return token{typ: tokenControl, text: "*"}
	case '\'':
		// Hex character \'HH.
		hex1, err1 := rd.ReadByte()
		hex2, err2 := rd.ReadByte()
		*pos += 2
		if err1 != nil || err2 != nil {
			return token{typ: tokenText, text: "'"}
		}
		val, convErr := strconv.ParseUint(string([]byte{hex1, hex2}), 16, 8)
		if convErr != nil {
			return token{typ: tokenText, text: string([]byte{hex1, hex2})}
		}
		return token{typ: tokenHex, text: string(rune(val))}
	case 'u':
		// Could be \uN (Unicode) or a control word starting with 'u'.
		// Try to read a number.
		var numBuf strings.Builder
		first, err := rd.ReadByte()
		if err != nil {
			return token{typ: tokenControl, text: "u"}
		}
		*pos++
		if first == '-' || (first >= '0' && first <= '9') {
			numBuf.WriteByte(first)
			for {
				c, err := rd.ReadByte()
				if err != nil {
					break
				}
				*pos++
				if c >= '0' && c <= '9' {
					numBuf.WriteByte(c)
				} else {
					// Space delimiter is consumed; anything else is unread.
					if c != ' ' {
						_ = rd.UnreadByte()
						*pos--
					}
					break
				}
			}
			num, convErr := strconv.Atoi(numBuf.String())
			if convErr == nil {
				// Valid Unicode escape. Per the RTF spec, \uN is followed by
				// one or more ANSI fallback "characters" (count controlled by
				// \ucN, default 1) that conformant readers must skip. Without
				// this, the fallback bytes leak through the tokenizer as
				// plain text — e.g. "\u171?" yields "«?" instead of "«".
				skipUnicodeFallback(rd, pos, 1)
				r := rune(num)
				if r < 0 {
					r += 65536
				}
				buf := make([]byte, utf8.UTFMax)
				n := utf8.EncodeRune(buf, r)
				return token{typ: tokenUnicode, text: string(buf[:n])}
			}
			// Not a valid number - treat as control word "u" + what we read.
			_ = rd.UnreadByte()
			*pos--
			return token{typ: tokenControl, text: "u" + numBuf.String()}
		}
		// Not a digit after \u - it's a control word starting with 'u'.
		_ = rd.UnreadByte()
		*pos--
		return readControlWordFrom(rd, "u", pos)
	}

	// Regular control word: alphabetic characters followed optionally by digits.
	if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' {
		return readControlWordFrom(rd, string(b), pos)
	}

	// Unknown - return as text.
	return token{typ: tokenText, text: "\\" + string(b)}
}

// readControlWordFrom reads the rest of a control word given its first character(s).
func readControlWordFrom(rd *bufio.Reader, prefix string, pos *int) token {
	var word strings.Builder
	word.WriteString(prefix)

	// Read alphabetic characters.
	for {
		b, err := rd.ReadByte()
		if err != nil {
			return token{typ: tokenControl, text: word.String(), param: -1}
		}
		*pos++
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' {
			word.WriteByte(b)
		} else {
			_ = rd.UnreadByte()
			*pos--
			break
		}
	}

	// Read optional numeric parameter.
	var numBuf strings.Builder
	for {
		b, err := rd.ReadByte()
		if err != nil {
			break
		}
		*pos++
		if b == '-' && numBuf.Len() == 0 {
			numBuf.WriteByte(b)
		} else if b >= '0' && b <= '9' {
			numBuf.WriteByte(b)
		} else {
			// Space delimiter after control word is consumed.
			if b != ' ' {
				_ = rd.UnreadByte()
				*pos--
			}
			break
		}
	}

	tok := token{typ: tokenControl, text: word.String(), param: -1}
	if numBuf.Len() > 0 {
		if num, err := strconv.Atoi(numBuf.String()); err == nil {
			tok.param = num
			tok.hasParam = true
		}
	}

	return tok
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

func (r *Reader) emitErr(ctx context.Context, ch chan<- model.PartResult, err error) {
	select {
	case ch <- model.PartResult{Error: err}:
	case <-ctx.Done():
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
