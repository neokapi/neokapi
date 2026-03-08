package tex

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// noTextCommands are commands whose arguments are non-translatable.
var noTextCommands = map[string]bool{
	"label":           true,
	"ref":             true,
	"cite":            true,
	"include":         true,
	"input":           true,
	"bibliography":    true,
	"bibliographystyle": true,
	"pageref":         true,
	"eqref":           true,
	"documentclass":   true,
	"usepackage":      true,
	"newcommand":      true,
	"renewcommand":    true,
	"setlength":       true,
	"setcounter":      true,
	"addtocounter":    true,
	"pagestyle":       true,
	"thispagestyle":   true,
	"pagenumbering":   true,
}

// inlineTextCommands are commands whose arguments contain inline translatable text.
var inlineTextCommands = map[string]bool{
	"textbf":       true,
	"textit":       true,
	"emph":         true,
	"texttt":       true,
	"textsf":       true,
	"textrm":       true,
	"textsc":       true,
	"textsl":       true,
	"underline":    true,
	"mbox":         true,
	"fbox":         true,
	"footnote":     true,
	"footnotetext": true,
}

// paragraphTextCommands produce separate text units for their arguments.
var paragraphTextCommands = map[string]bool{
	"section":       true,
	"subsection":    true,
	"subsubsection": true,
	"chapter":       true,
	"part":          true,
	"paragraph":     true,
	"subparagraph":  true,
	"title":         true,
	"author":        true,
	"date":          true,
	"caption":       true,
}

// nonTranslatableEnvironments that should be emitted as Data.
var nonTranslatableEnvironments = map[string]bool{
	"verbatim":  true,
	"lstlisting": true,
	"equation":  true,
	"equation*": true,
	"align":     true,
	"align*":    true,
	"gather":    true,
	"gather*":   true,
	"multline":  true,
	"multline*": true,
	"eqnarray":  true,
	"eqnarray*": true,
	"math":      true,
	"displaymath": true,
}

// headerTextCommands are commands in the preamble whose arguments ARE translatable.
var headerTextCommands = map[string]bool{
	"title":  true,
	"author": true,
	"date":   true,
}

// Reader implements DataFormatReader for TeX/LaTeX files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new TeX/LaTeX reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "tex",
			FormatDisplayName: "TeX/LaTeX",
			FormatMimeType:    "application/x-tex",
			FormatExtensions:  []string{".tex", ".latex"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/x-tex", "text/x-tex"},
		Extensions: []string{".tex", ".latex"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("tex: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		if err := r.readContent(ctx, ch); err != nil {
			ch <- model.PartResult{Error: err}
		}
	}()
	return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) error {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "tex",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-tex",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return nil
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		return fmt.Errorf("tex: reading: %w", err)
	}

	p := &parser{
		source:       string(content),
		pos:          0,
		blockCounter: 0,
		dataCounter:  0,
	}
	p.parse(ctx, ch, r)

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
	return nil
}

// parser holds state for TeX parsing.
type parser struct {
	source       string
	pos          int
	blockCounter int
	dataCounter  int
	inHeader     bool // true when between \documentclass and \begin{document}
}

func (p *parser) parse(ctx context.Context, ch chan<- model.PartResult, r *Reader) {
	// Detect header area
	if idx := strings.Index(p.source, `\documentclass`); idx >= 0 {
		p.inHeader = true
	}

	var textBuf strings.Builder
	var rawBuf strings.Builder // raw TeX for Data reconstruction

	flushText := func() {
		text := strings.TrimSpace(textBuf.String())
		if text != "" {
			p.blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", p.blockCounter), text)
			block.Name = fmt.Sprintf("para%d", p.blockCounter)
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
		textBuf.Reset()
		rawBuf.Reset()
	}

	flushData := func(content string) {
		if content == "" {
			return
		}
		p.dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", p.dataCounter),
			Name: "tex-structure",
			Properties: map[string]string{
				"content": content,
			},
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
	}

	for p.pos < len(p.source) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		ch0 := p.source[p.pos]

		// Check for \begin{document} — end of header
		if p.inHeader && p.pos+16 <= len(p.source) && p.source[p.pos:p.pos+16] == `\begin{document}` {
			flushText()
			flushData(`\begin{document}`)
			p.pos += 16
			p.inHeader = false
			continue
		}

		// Check for \end{document}
		if p.pos+14 <= len(p.source) && p.source[p.pos:p.pos+14] == `\end{document}` {
			flushText()
			flushData(`\end{document}`)
			p.pos += 14
			continue
		}

		// Comment: % to end of line
		if ch0 == '%' && (p.pos == 0 || p.source[p.pos-1] != '\\') {
			flushText()
			comment := p.readToEndOfLine()
			flushData(comment)
			continue
		}

		// Inline math: $...$
		if ch0 == '$' && (p.pos == 0 || p.source[p.pos-1] != '\\') {
			// Check for display math $$...$$
			if p.pos+1 < len(p.source) && p.source[p.pos+1] == '$' {
				flushText()
				math := p.readDisplayMathDollar()
				flushData(math)
				continue
			}
			// Inline math $...$
			flushText()
			math := p.readInlineMathDollar()
			flushData(math)
			continue
		}

		// Backslash commands
		if ch0 == '\\' {
			cmd, cmdEnd := p.peekCommand()
			if cmd != "" {
				// Display math \[...\]
				if cmd == "[" {
					flushText()
					math := p.readDisplayMathBracket()
					flushData(math)
					continue
				}

				// \begin{...} environment
				if cmd == "begin" {
					envName := p.peekBraceArg(cmdEnd)
					if envName != "" {
						if nonTranslatableEnvironments[envName] {
							flushText()
							env := p.readEnvironment(envName)
							flushData(env)
							continue
						}
						// Translatable environment — emit \begin{...} as data and continue
						flushText()
						beginTag := p.readBeginTag()
						flushData(beginTag)
						continue
					}
				}

				// \end{...} environment
				if cmd == "end" {
					flushText()
					endTag := p.readEndTag()
					flushData(endTag)
					continue
				}

				// In header, only headerTextCommands produce blocks
				if p.inHeader {
					if headerTextCommands[cmd] {
						flushText()
						p.pos = cmdEnd
						arg := p.readBraceArgText()
						p.blockCounter++
						block := model.NewBlock(fmt.Sprintf("tu%d", p.blockCounter), arg)
						block.Name = cmd
						block.Type = cmd
						r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
						continue
					}
					// Non-text header command — emit as data
					raw := p.readCommandRaw()
					rawBuf.WriteString(raw)
					continue
				}

				// Paragraph-text commands
				if paragraphTextCommands[cmd] {
					flushText()
					p.pos = cmdEnd
					// Skip optional argument [...]
					p.skipOptionalArg()
					arg := p.readBraceArgText()
					if arg == "" {
						// No brace argument — emit command as data
						flushData("\\" + cmd)
						continue
					}
					p.blockCounter++
					block := model.NewBlock(fmt.Sprintf("tu%d", p.blockCounter), arg)
					block.Name = cmd
					block.Type = cmd
					r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
					continue
				}

				// No-text commands — argument is non-translatable
				if noTextCommands[cmd] {
					flushText()
					raw := p.readCommandRaw()
					flushData(raw)
					continue
				}

				// Inline-text commands — argument text is part of current paragraph
				if inlineTextCommands[cmd] {
					p.pos = cmdEnd
					arg := p.readBraceArgContent(&textBuf, cmd)
					_ = arg
					continue
				}

				// Unknown command — include in text flow
				p.pos = cmdEnd
				// If it has a brace argument, include that too
				if p.pos < len(p.source) && p.source[p.pos] == '{' {
					raw := "\\" + cmd + p.readBraceArgRaw()
					textBuf.WriteString(raw)
				} else {
					textBuf.WriteString("\\" + cmd)
					// Add trailing space if original had one
					if p.pos < len(p.source) && p.source[p.pos] == ' ' {
						textBuf.WriteByte(' ')
						p.pos++
					}
				}
				continue
			}

			// Special character sequences
			if p.pos+1 < len(p.source) {
				next := p.source[p.pos+1]
				switch next {
				case '\\': // line break \\
					textBuf.WriteString(`\\`)
					p.pos += 2
					continue
				case '&', '%', '$', '#', '_', '{', '}': // escaped special chars
					textBuf.WriteByte(next)
					p.pos += 2
					continue
				case '~': // non-breaking space
					textBuf.WriteString(`\~`)
					p.pos += 2
					continue
				}
			}

			// Unrecognized backslash sequence
			textBuf.WriteByte('\\')
			p.pos++
			continue
		}

		// Double newline — paragraph break
		if ch0 == '\n' && p.pos+1 < len(p.source) && p.source[p.pos+1] == '\n' {
			flushText()
			// Skip all consecutive blank lines
			for p.pos < len(p.source) && p.source[p.pos] == '\n' {
				p.pos++
			}
			continue
		}

		// Tilde — non-breaking space (in TeX, ~ is a non-breaking space)
		if ch0 == '~' {
			textBuf.WriteByte(' ')
			p.pos++
			continue
		}

		// Regular character
		_, size := utf8.DecodeRuneInString(p.source[p.pos:])
		textBuf.WriteString(p.source[p.pos : p.pos+size])
		p.pos += size
	}

	flushText()
}

// peekCommand returns the command name starting at p.pos (which should be \).
// Returns ("", 0) if not a valid command.
func (p *parser) peekCommand() (string, int) {
	if p.pos >= len(p.source) || p.source[p.pos] != '\\' {
		return "", 0
	}
	start := p.pos + 1
	if start >= len(p.source) {
		return "", 0
	}

	// Special single-char commands
	ch := p.source[start]
	if ch == '[' || ch == ']' {
		return string(ch), start + 1
	}

	// Alpha command names
	if !isAlpha(ch) {
		return "", 0
	}
	end := start
	for end < len(p.source) && isAlpha(p.source[end]) {
		end++
	}
	// Include trailing * for starred commands
	if end < len(p.source) && p.source[end] == '*' {
		end++
	}
	return p.source[start:end], end
}

// peekBraceArg looks for {argtext} starting at pos. Returns the text inside braces.
func (p *parser) peekBraceArg(pos int) string {
	if pos >= len(p.source) || p.source[pos] != '{' {
		return ""
	}
	depth := 1
	start := pos + 1
	i := start
	for i < len(p.source) && depth > 0 {
		switch p.source[i] {
		case '{':
			if i == 0 || p.source[i-1] != '\\' {
				depth++
			}
		case '}':
			if i == 0 || p.source[i-1] != '\\' {
				depth--
			}
		}
		if depth > 0 {
			i++
		}
	}
	if depth != 0 {
		return ""
	}
	return p.source[start:i]
}

// readBraceArgText reads a {text} argument and returns the text inside.
// Advances p.pos past the closing brace.
func (p *parser) readBraceArgText() string {
	p.skipSpaces()
	if p.pos >= len(p.source) || p.source[p.pos] != '{' {
		return ""
	}
	p.pos++ // skip {
	depth := 1
	var buf strings.Builder
	for p.pos < len(p.source) && depth > 0 {
		ch := p.source[p.pos]
		switch ch {
		case '{':
			if p.pos == 0 || p.source[p.pos-1] != '\\' {
				depth++
			}
			if depth > 1 {
				buf.WriteByte(ch)
			}
		case '}':
			if p.pos == 0 || p.source[p.pos-1] != '\\' {
				depth--
			}
			if depth > 0 {
				buf.WriteByte(ch)
			}
		case '\\':
			// Check for inline text commands within brace args
			cmd, cmdEnd := p.peekCommand()
			if cmd != "" && inlineTextCommands[cmd] {
				oldPos := p.pos
				p.pos = cmdEnd
				inner := p.readBraceArgText()
				buf.WriteString(inner)
				_ = oldPos
				continue
			}
			buf.WriteByte(ch)
		default:
			buf.WriteByte(ch)
		}
		p.pos++
	}
	return buf.String()
}

// readBraceArgContent reads a brace argument and appends text to the builder,
// handling inline-text commands with span markup.
func (p *parser) readBraceArgContent(buf *strings.Builder, cmd string) string {
	p.skipSpaces()
	if p.pos >= len(p.source) || p.source[p.pos] != '{' {
		return ""
	}
	p.pos++ // skip {
	depth := 1
	var inner strings.Builder
	for p.pos < len(p.source) && depth > 0 {
		ch := p.source[p.pos]
		switch ch {
		case '{':
			if p.pos == 0 || p.source[p.pos-1] != '\\' {
				depth++
			}
			if depth > 1 {
				inner.WriteByte(ch)
			}
		case '}':
			if p.pos == 0 || p.source[p.pos-1] != '\\' {
				depth--
			}
			if depth > 0 {
				inner.WriteByte(ch)
			}
		default:
			inner.WriteByte(ch)
		}
		p.pos++
	}
	text := inner.String()
	buf.WriteString(text)
	return text
}

// readBraceArgRaw reads a {text} argument including braces and returns the raw string.
func (p *parser) readBraceArgRaw() string {
	if p.pos >= len(p.source) || p.source[p.pos] != '{' {
		return ""
	}
	start := p.pos
	p.pos++ // skip {
	depth := 1
	for p.pos < len(p.source) && depth > 0 {
		switch p.source[p.pos] {
		case '{':
			if p.source[p.pos-1] != '\\' {
				depth++
			}
		case '}':
			if p.source[p.pos-1] != '\\' {
				depth--
			}
		}
		p.pos++
	}
	return p.source[start:p.pos]
}

// readCommandRaw reads a command and its arguments as raw text.
func (p *parser) readCommandRaw() string {
	start := p.pos
	cmd, cmdEnd := p.peekCommand()
	if cmd == "" {
		p.pos++
		return p.source[start:p.pos]
	}
	p.pos = cmdEnd

	// Read optional args
	p.skipOptionalArg()

	// Read brace args
	for p.pos < len(p.source) && p.source[p.pos] == '{' {
		p.readBraceArgRaw()
	}

	return p.source[start:p.pos]
}

// readToEndOfLine reads from current position to end of line (including the newline).
func (p *parser) readToEndOfLine() string {
	start := p.pos
	for p.pos < len(p.source) && p.source[p.pos] != '\n' {
		p.pos++
	}
	if p.pos < len(p.source) {
		p.pos++ // include the newline
	}
	return p.source[start:p.pos]
}

// readInlineMathDollar reads $...$ inline math.
func (p *parser) readInlineMathDollar() string {
	start := p.pos
	p.pos++ // skip opening $
	for p.pos < len(p.source) {
		if p.source[p.pos] == '$' && (p.pos == 0 || p.source[p.pos-1] != '\\') {
			p.pos++ // skip closing $
			return p.source[start:p.pos]
		}
		p.pos++
	}
	return p.source[start:p.pos]
}

// readDisplayMathDollar reads $$...$$ display math.
func (p *parser) readDisplayMathDollar() string {
	start := p.pos
	p.pos += 2 // skip opening $$
	for p.pos+1 < len(p.source) {
		if p.source[p.pos] == '$' && p.source[p.pos+1] == '$' {
			p.pos += 2 // skip closing $$
			return p.source[start:p.pos]
		}
		p.pos++
	}
	p.pos = len(p.source)
	return p.source[start:p.pos]
}

// readDisplayMathBracket reads \[...\] display math.
func (p *parser) readDisplayMathBracket() string {
	start := p.pos
	p.pos += 2 // skip \[
	for p.pos+1 < len(p.source) {
		if p.source[p.pos] == '\\' && p.source[p.pos+1] == ']' {
			p.pos += 2 // skip \]
			return p.source[start:p.pos]
		}
		p.pos++
	}
	p.pos = len(p.source)
	return p.source[start:p.pos]
}

// readEnvironment reads \begin{name}...\end{name} as a single raw string.
func (p *parser) readEnvironment(name string) string {
	start := p.pos
	endTag := `\end{` + name + `}`
	// Skip past \begin{name}
	p.pos += len(`\begin{` + name + `}`)
	idx := strings.Index(p.source[p.pos:], endTag)
	if idx >= 0 {
		p.pos += idx + len(endTag)
	} else {
		p.pos = len(p.source)
	}
	return p.source[start:p.pos]
}

// readBeginTag reads \begin{name} tag (just the tag, not the content).
func (p *parser) readBeginTag() string {
	start := p.pos
	_, cmdEnd := p.peekCommand()
	p.pos = cmdEnd
	raw := p.readBraceArgRaw()
	_ = raw
	return p.source[start:p.pos]
}

// readEndTag reads \end{name} tag.
func (p *parser) readEndTag() string {
	start := p.pos
	_, cmdEnd := p.peekCommand()
	p.pos = cmdEnd
	raw := p.readBraceArgRaw()
	_ = raw
	return p.source[start:p.pos]
}

// skipSpaces skips whitespace (but not newlines).
func (p *parser) skipSpaces() {
	for p.pos < len(p.source) && (p.source[p.pos] == ' ' || p.source[p.pos] == '\t') {
		p.pos++
	}
}

// skipOptionalArg skips an optional [arg] if present.
func (p *parser) skipOptionalArg() {
	p.skipSpaces()
	if p.pos >= len(p.source) || p.source[p.pos] != '[' {
		return
	}
	depth := 1
	p.pos++ // skip [
	for p.pos < len(p.source) && depth > 0 {
		switch p.source[p.pos] {
		case '[':
			depth++
		case ']':
			depth--
		}
		p.pos++
	}
}

func isAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
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
