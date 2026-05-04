package tex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// noTextCommands are commands whose arguments are non-translatable.
var noTextCommands = map[string]bool{
	"label":             true,
	"ref":               true,
	"cite":              true,
	"include":           true,
	"input":             true,
	"bibliography":      true,
	"bibliographystyle": true,
	"pageref":           true,
	"eqref":             true,
	"documentclass":     true,
	"usepackage":        true,
	"newcommand":        true,
	"renewcommand":      true,
	"setlength":         true,
	"setcounter":        true,
	"addtocounter":      true,
	"pagestyle":         true,
	"thispagestyle":     true,
	"pagenumbering":     true,
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

// paragraphTextCommands produce separate text units for their arguments
// when encountered in the body. Mirrors Okapi TEXFilter's oneArgParText
// list — \date is intentionally excluded because Okapi treats it as an
// unknown command in body mode (resulting in a non-translatable
// document part). \date in the preamble is still translatable via
// headerTextCommands.
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
	"caption":       true,
}

// nonTranslatableEnvironments that should be emitted as Data.
var nonTranslatableEnvironments = map[string]bool{
	"verbatim":    true,
	"lstlisting":  true,
	"equation":    true,
	"equation*":   true,
	"align":       true,
	"align*":      true,
	"gather":      true,
	"gather*":     true,
	"multline":    true,
	"multline*":   true,
	"eqnarray":    true,
	"eqnarray*":   true,
	"math":        true,
	"displaymath": true,
}

// headerTextCommands are commands in the preamble whose arguments ARE
// translatable. Mirrors Okapi TEXFilter's oneArgParText for the
// header (\title, \author). \date is intentionally excluded because
// Okapi treats it as a non-translatable document part — date strings
// are usually programmatically formatted ("January 21, 1994") and not
// meaningful translation targets.
var headerTextCommands = map[string]bool{
	"title":  true,
	"author": true,
}

// Reader implements DataFormatReader for TeX/LaTeX files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

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

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
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
		return errors.New("tex: nil document or reader")
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

	r.skelFlush()

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
	lastSkelPos  int  // tracks position for skeleton output
}

func (p *parser) parse(ctx context.Context, ch chan<- model.PartResult, r *Reader) {
	// Detect header area
	if idx := strings.Index(p.source, `\documentclass`); idx >= 0 {
		p.inHeader = true
	}

	var textBuf strings.Builder
	var rawBuf strings.Builder // raw TeX for Data reconstruction
	textStartPos := -1         // source position where text accumulation started

	flushText := func() {
		text := strings.TrimSpace(textBuf.String())
		if text != "" {
			p.blockCounter++
			blockID := fmt.Sprintf("tu%d", p.blockCounter)
			block := model.NewBlock(blockID, text)
			block.Name = fmt.Sprintf("para%d", p.blockCounter)
			// For skeleton: the bytes between lastSkelPos and the
			// position where translatable text actually started belong
			// to skeleton (preceding whitespace, skipped commands,
			// etc.); only the bytes from textStartPos to p.pos are
			// the block's raw source. Splitting these correctly is
			// what lets the writer round-trip the preamble verbatim
			// when one of the body paragraphs is the first translatable
			// unit.
			if r.skeletonStore != nil {
				skelEnd := textStartPos
				if skelEnd < p.lastSkelPos || skelEnd < 0 {
					skelEnd = p.lastSkelPos
				}
				if skelEnd > p.lastSkelPos {
					r.skelText(p.source[p.lastSkelPos:skelEnd])
				}
				block.Properties["tex.rawSource"] = p.source[skelEnd:p.pos]
				r.skelRef(blockID)
				p.lastSkelPos = p.pos
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		}
		textBuf.Reset()
		rawBuf.Reset()
		textStartPos = -1
	}

	flushData := func(content string) {
		if content == "" {
			return
		}
		// In skeleton mode, route the raw source bytes to the
		// skeleton store so the writer reproduces them verbatim. The
		// Data part is still emitted for non-skeleton writers (and
		// for downstream tools that observe Data events), but the
		// skeleton path uses the byte-exact original.
		if r.skeletonStore != nil && p.pos > p.lastSkelPos {
			r.skelText(p.source[p.lastSkelPos:p.pos])
			p.lastSkelPos = p.pos
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
						cmdStartPos := p.pos
						p.pos = cmdEnd
						arg := p.readBraceArgText()
						p.blockCounter++
						blockID := fmt.Sprintf("tu%d", p.blockCounter)
						block := model.NewBlock(blockID, arg)
						block.Name = cmd
						block.Type = cmd
						if r.skeletonStore != nil {
							// Include any whitespace/data before this command in skeleton text
							if cmdStartPos > p.lastSkelPos {
								r.skelText(p.source[p.lastSkelPos:cmdStartPos])
							}
							block.Properties["tex.rawSource"] = p.source[cmdStartPos:p.pos]
							r.skelRef(blockID)
							p.lastSkelPos = p.pos
						}
						r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
						continue
					}
					// Non-text header command — emit as data so its raw
					// bytes round-trip through the skeleton. Previously
					// these were appended to a never-emitted local buffer
					// and silently lost.
					raw := p.readCommandRaw()
					flushData(raw)
					continue
				}

				// Paragraph-text commands
				if paragraphTextCommands[cmd] {
					flushText()
					cmdStartPos := p.pos
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
					blockID := fmt.Sprintf("tu%d", p.blockCounter)
					block := model.NewBlock(blockID, arg)
					block.Name = cmd
					block.Type = cmd
					if r.skeletonStore != nil {
						if cmdStartPos > p.lastSkelPos {
							r.skelText(p.source[p.lastSkelPos:cmdStartPos])
						}
						block.Properties["tex.rawSource"] = p.source[cmdStartPos:p.pos]
						r.skelRef(blockID)
						p.lastSkelPos = p.pos
					}
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
					if textStartPos < 0 {
						textStartPos = p.pos
					}
					p.pos = cmdEnd
					arg := p.readBraceArgContent(&textBuf, cmd)
					_ = arg
					continue
				}

				// Unknown command — Okapi TEXFilter classifies as
				// OneArgNoText: the command and its (optional) brace
				// argument become a non-translatable document part.
				// Keeping unknown commands out of the translatable
				// text flow prevents user-visible commands like
				// \tableofcontents, \maketitle, \LaTeX from being
				// pseudo-translated as if they were words.
				flushText()
				cmdStart := p.pos
				p.pos = cmdEnd
				// Read any optional [arg] then any number of brace args.
				p.skipOptionalArg()
				for p.pos < len(p.source) && p.source[p.pos] == '{' {
					p.readBraceArgRaw()
				}
				// Preserve a trailing space when no brace argument
				// followed, mirroring the Okapi behavior of keeping
				// the separator between a bare command and the
				// following text.
				if p.pos < len(p.source) && p.source[p.pos] == ' ' && p.pos == cmdEnd {
					p.pos++
				}
				flushData(p.source[cmdStart:p.pos])
				continue
			}

			// Special character sequences
			if p.pos+1 < len(p.source) {
				next := p.source[p.pos+1]
				switch next {
				case '\\': // line break \\
					if textStartPos < 0 {
						textStartPos = p.pos
					}
					textBuf.WriteString(`\\`)
					p.pos += 2
					continue
				case '&', '%', '$', '#', '_', '{', '}': // escaped special chars
					if textStartPos < 0 {
						textStartPos = p.pos
					}
					textBuf.WriteByte(next)
					p.pos += 2
					continue
				case '~': // non-breaking space
					if textStartPos < 0 {
						textStartPos = p.pos
					}
					textBuf.WriteString(`\~`)
					p.pos += 2
					continue
				}
			}

			// Unrecognized backslash sequence
			if textStartPos < 0 {
				textStartPos = p.pos
			}
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
			if textStartPos < 0 {
				textStartPos = p.pos
			}
			textBuf.WriteByte(' ')
			p.pos++
			continue
		}

		// Regular character
		if textStartPos < 0 {
			textStartPos = p.pos
		}
		_, size := utf8.DecodeRuneInString(p.source[p.pos:])
		textBuf.WriteString(p.source[p.pos : p.pos+size])
		p.pos += size
	}

	flushText()

	// Write any remaining source as skeleton text
	if r.skeletonStore != nil && p.lastSkelPos < len(p.source) {
		r.skelText(p.source[p.lastSkelPos:])
	}
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
