package rtf

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for RTF files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new RTF reader.
func NewReader() *Reader {
	cfg := &Config{}
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
		return fmt.Errorf("rtf: nil document or reader")
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
	typ      tokenType
	text     string // For text tokens: the actual text. For control: the control word.
	param    int    // Numeric parameter for control words (-1 if none).
	hasParam bool
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

// skipDestinations are RTF destinations whose content is non-translatable.
var skipDestinations = map[string]bool{
	"fonttbl":    true,
	"colortbl":   true,
	"stylesheet": true,
	"info":       true,
	"header":     true,
	"headerl":    true,
	"headerr":    true,
	"headerf":    true,
	"footer":     true,
	"footerl":    true,
	"footerr":    true,
	"footerf":    true,
	"pict":       true,
	"object":     true,
	"fldinst":    true,
	"xe":         true,
	"tc":         true,
	"rxe":        true,
	"bkmkstart":  true,
	"bkmkend":    true,
	"field":      false, // field itself is not skipped; fldinst inside is
	"fldrslt":    false,
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

	tokens := tokenize(data)
	r.emitParts(ctx, ch, tokens)

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// emitParts walks the token stream and emits Part events.
func (r *Reader) emitParts(ctx context.Context, ch chan<- model.PartResult, tokens []token) {
	blockCounter := 0
	dataCounter := 0

	// Track group depth and which groups to skip.
	type groupInfo struct {
		skip           bool
		destinationTag string
	}
	var groupStack []groupInfo
	depth := 0

	// Accumulate text for the current paragraph.
	var paraText strings.Builder
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
		paraText.Reset()
		if strings.TrimSpace(text) == "" {
			return
		}
		flushData()
		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
		block.Name = fmt.Sprintf("para.%d", blockCounter)
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
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
				groupStack = groupStack[:len(groupStack)-1]
			}

		case tokenControl:
			// Check if this is a destination control word at the start of a group.
			if len(groupStack) > 0 {
				gi := &groupStack[len(groupStack)-1]
				if gi.destinationTag == "" {
					gi.destinationTag = tok.text
					if skipDestinations[tok.text] {
						gi.skip = true
						continue
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
				}
			case "lquote":
				if inBody {
					paraText.WriteRune('\u2018')
				}
			case "rquote":
				if inBody {
					paraText.WriteRune('\u2019')
				}
			case "ldblquote":
				if inBody {
					paraText.WriteRune('\u201C')
				}
			case "rdblquote":
				if inBody {
					paraText.WriteRune('\u201D')
				}
			case "emdash":
				if inBody {
					paraText.WriteRune('\u2014')
				}
			case "endash":
				if inBody {
					paraText.WriteRune('\u2013')
				}
			case "bullet":
				if inBody {
					paraText.WriteRune('\u2022')
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
			} else {
				rawRTF.WriteString(tok.text)
			}

		case tokenHex:
			if shouldSkip() {
				continue
			}
			if inBody {
				paraText.WriteString(tok.text)
			}

		case tokenUnicode:
			if shouldSkip() {
				continue
			}
			if inBody {
				paraText.WriteString(tok.text)
			}
		}
	}

	// Flush any remaining paragraph text.
	flushParagraph()
	flushData()
}

// tokenize converts raw RTF bytes into a stream of tokens.
func tokenize(data []byte) []token {
	var tokens []token
	rd := bufio.NewReader(strings.NewReader(string(data)))

	for {
		b, err := rd.ReadByte()
		if err != nil {
			break
		}

		switch b {
		case '{':
			tokens = append(tokens, token{typ: tokenGroupStart})
		case '}':
			tokens = append(tokens, token{typ: tokenGroupEnd})
		case '\\':
			tok := parseControlWord(rd)
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
				if b2 == '{' || b2 == '}' || b2 == '\\' || b2 == '\r' || b2 == '\n' {
					_ = rd.UnreadByte()
					break
				}
				text.WriteByte(b2)
			}
			tokens = append(tokens, token{typ: tokenText, text: text.String()})
		}
	}

	return tokens
}

// parseControlWord reads a control word or special escape after '\'.
func parseControlWord(rd *bufio.Reader) token {
	b, err := rd.ReadByte()
	if err != nil {
		return token{typ: tokenText, text: "\\"}
	}

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
		if first == '-' || (first >= '0' && first <= '9') {
			numBuf.WriteByte(first)
			for {
				c, err := rd.ReadByte()
				if err != nil {
					break
				}
				if c >= '0' && c <= '9' {
					numBuf.WriteByte(c)
				} else {
					// Space delimiter is consumed; anything else is unread.
					if c != ' ' {
						_ = rd.UnreadByte()
					}
					break
				}
			}
			num, convErr := strconv.Atoi(numBuf.String())
			if convErr == nil {
				// Valid Unicode escape.
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
			return token{typ: tokenControl, text: "u" + numBuf.String()}
		}
		// Not a digit after \u - it's a control word starting with 'u'.
		_ = rd.UnreadByte()
		return readControlWordFrom(rd, "u")
	}

	// Regular control word: alphabetic characters followed optionally by digits.
	if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' {
		return readControlWordFrom(rd, string(b))
	}

	// Unknown - return as text.
	return token{typ: tokenText, text: "\\" + string(b)}
}

// readControlWordFrom reads the rest of a control word given its first character(s).
func readControlWordFrom(rd *bufio.Reader, prefix string) token {
	var word strings.Builder
	word.WriteString(prefix)

	// Read alphabetic characters.
	for {
		b, err := rd.ReadByte()
		if err != nil {
			return token{typ: tokenControl, text: word.String(), param: -1}
		}
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' {
			word.WriteByte(b)
		} else {
			_ = rd.UnreadByte()
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
		if b == '-' && numBuf.Len() == 0 {
			numBuf.WriteByte(b)
		} else if b >= '0' && b <= '9' {
			numBuf.WriteByte(b)
		} else {
			// Space delimiter after control word is consumed.
			if b != ' ' {
				_ = rd.UnreadByte()
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
