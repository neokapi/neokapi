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

// inlineTextCommands are commands whose arguments contain inline
// translatable text. Mirrors Okapi TEXFilter's oneArgInlineText for
// the `\cmd{…}` form. Bare style switches (\bf, \em, \tt) used as
// `{\bf text}` follow a different code path in okapi (processOpenCurly
// → processOneArgInlineText) and are intentionally not on this list —
// the native equivalent doesn't yet model the brace-first form. Other
// styling commands (\textrm, \textsc, \textsl, \underline, \fbox,
// \footnotetext, …) fall through to the unknown-command path so their
// arguments stay non-translatable, matching okapi's body-mode behavior
// for any command not on this list.
var inlineTextCommands = map[string]bool{
	"emph":       true,
	"footnote":   true,
	"hbox":       true,
	"mbox":       true,
	"textbackit": true,
	"textbf":     true,
	"texttt":     true,
	"textsf":     true,
	"textit":     true,
	"vbox":       true,
}

// paragraphTextCommands produce separate text units for their arguments
// when encountered in the body. Mirrors Okapi TEXFilter's oneArgParText
// list verbatim — `\subsubsection`, `\paragraph`, `\subparagraph`, and
// `\part` are intentionally NOT here because Okapi treats them as
// unknown commands in body mode (resulting in a non-translatable
// document part). Adding them as paragraph-text commands would extract
// `{Typefaces and Sizes:}` from `\subsubsection{Typefaces and Sizes:}`
// as translatable text, diverging from okapi's reference output. The
// trailing-running-head spelling `\titlerunning` and the silent
// `\typeout` / index entries are mirrored from okapi for completeness.
var paragraphTextCommands = map[string]bool{
	"author":       true,
	"Chapter":      true,
	"chapter":      true,
	"index":        true,
	"typeout":      true,
	"title":        true,
	"titlerunning": true,
	"section":      true,
	"subsection":   true,
	"caption":      true,
}

// accentMap maps LaTeX accent commands to their Unicode equivalent.
// Mirrors a subset of Okapi TEXEncoder's reset() map — only the
// entries actually exercised by the upstream fixture set are listed
// here. Both the brace and braceless forms are recognised
// (`\={a}` and `\=a` both → ā). Special forms `\={\i}` / `\=\i`
// (dotless i with macron) map to ī.
//
// On extraction the matched bytes are replaced with the Unicode
// character, producing the same source text Okapi emits after its
// writer-side TEXEncoder.convertCodesToLetters pass. Without this,
// accent commands survive into the translatable text stream and end
// up either pseudo-translated (`\={\i}` → `\={\ĩ}` because pseudo
// hits the bare `i`) or rendered raw, both diverging from the okapi
// reference for fixtures that mix accent commands and translatable
// surrounding text.
var accentMap = map[string]string{
	// \v{X} caron (háček) — selected entries.
	`\v{S}`: "Š", `\v{s}`: "š",
	`\v{C}`: "Č", `\v{c}`: "č",
	`\v{Z}`: "Ž", `\v{z}`: "ž",
	`\v{N}`: "Ň", `\v{n}`: "ň",
	`\v{R}`: "Ř", `\v{r}`: "ř",
	`\v{T}`: "Ť", `\v{t}`: "ť",
	`\v{D}`: "Ď", `\v{d}`: "ď",
	`\v{L}`: "Ľ", `\v{l}`: "ľ",
	`\v{E}`: "Ě", `\v{e}`: "ě",
	`\v S`: "Š", `\v s`: "š",
	`\v C`: "Č", `\v c`: "č",
	`\v Z`: "Ž", `\v z`: "ž",
	// \={X} macron — selected entries.
	`\={A}`: "Ā", `\={a}`: "ā",
	`\={E}`: "Ē", `\={e}`: "ē",
	`\={I}`: "Ī",
	`\={O}`: "Ō", `\={o}`: "ō",
	`\={U}`: "Ū", `\={u}`: "ū",
	`\={\i}`: "ī",
	`\=A`:    "Ā", `\=a`: "ā",
	`\=E`: "Ē", `\=e`: "ē",
	`\=I`: "Ī",
	`\=O`: "Ō", `\=o`: "ō",
	`\=U`: "Ū", `\=u`: "ū",
	`\=\i`: "ī",
}

// matchAccentAt returns the Unicode replacement and the byte length
// of the matched LaTeX accent command starting at p.source[start],
// or ("", 0) when no entry in accentMap matches. Longest-match wins
// — `\={\i}` is preferred over `\={\` (which doesn't exist) and
// `\=A` over `\=` alone — so the caller can advance p.pos by the
// returned length to consume the original bytes.
func (p *parser) matchAccentAt(start int) (string, int) {
	if start >= len(p.source) || p.source[start] != '\\' {
		return "", 0
	}
	// Try lengths up to 8 bytes (`\={X}` is 5; `\v{X}` is 5; `\={\i}` is 6).
	maxLen := 8
	if start+maxLen > len(p.source) {
		maxLen = len(p.source) - start
	}
	bestVal := ""
	bestLen := 0
	for n := 2; n <= maxLen; n++ {
		key := p.source[start : start+n]
		if v, ok := accentMap[key]; ok {
			if n > bestLen {
				bestVal = v
				bestLen = n
			}
		}
	}
	return bestVal, bestLen
}

// nonTranslatableEnvironments that should be emitted as Data.
//
// table / table* / figure / figure* mirror Okapi TEXFilter's
// processTable / processFigure: the entire `\begin{table}…\end{table}`
// (or figure) span — including any nested `\caption`, `\label`, or
// `\begin{tabular}` content — is captured as one document part.
// Translatable text inside these structural environments is heuristic
// at best (column headers, table cells, captions are often pure data),
// and matching Okapi's "the whole table is opaque" stance keeps body
// paragraph extraction byte-equal on shared fixtures.
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
	"table":       true,
	"table*":      true,
	"figure":      true,
	"figure*":     true,
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

	// bodyRuns accumulates inline-code runs (PcOpen/PcClose for \emph,
	// \textbf, …) for the current body paragraph. textBuf accumulates
	// plain text between code runs. When an inline-text command is
	// encountered, the current textBuf is flushed to bodyRuns as a
	// TextRun, then PcOpen / inner runs / PcClose runs are appended.
	// On flushText, the final textBuf flushes to bodyRuns and a
	// NewRunsBlock is emitted (or NewBlock when bodyRuns is empty —
	// preserving the simple-paragraph fast path verbatim).
	var bodyRuns []model.Run
	pcCounter := 0

	flushBodyText := func() {
		if textBuf.Len() > 0 {
			bodyRuns = append(bodyRuns, model.Run{Text: &model.TextRun{Text: textBuf.String()}})
			textBuf.Reset()
		}
	}

	flushText := func() {
		raw := textBuf.String()
		// When bodyRuns has accumulated inline codes (e.g. \emph spans),
		// render the full sequence for trim / empty checks. Otherwise
		// the simple-paragraph path (textBuf only) keeps the previous
		// behavior byte-for-byte.
		hasRuns := len(bodyRuns) > 0
		var fullText string
		if hasRuns {
			var sb strings.Builder
			for _, r := range bodyRuns {
				if r.Text != nil {
					sb.WriteString(r.Text.Text)
				}
			}
			sb.WriteString(raw)
			fullText = sb.String()
		} else {
			fullText = raw
		}
		text := strings.TrimSpace(fullText)
		if text != "" {
			// Trailing whitespace eaten by TrimSpace lives in the
			// inter-block skeleton: the writer renders only the trimmed
			// translated text, so without this side-channel the bytes
			// that separated the text from the next data part vanish.
			// Leading whitespace stays with the block's rawSource and
			// is re-applied via extractLeadingWhitespace in the writer.
			leadingWS := fullText[:len(fullText)-len(strings.TrimLeft(fullText, " \t\n\r"))]
			trailingWS := fullText[len(strings.TrimRight(fullText, " \t\n\r")):]
			p.blockCounter++
			blockID := fmt.Sprintf("tu%d", p.blockCounter)
			var block *model.Block
			if hasRuns {
				// Build runs: bodyRuns + tail textBuf, then strip
				// leading WS from the first text run and trailing WS
				// from the last text run. Both go to skeleton (leading
				// is preserved via the writer's extractLeadingWhitespace
				// on tex.rawSource; trailing is emitted right after the
				// block ref). Without this, the WS would survive
				// pseudo-translation inside the run and double up
				// against the writer's prefix/skeleton mechanisms.
				runs := append([]model.Run(nil), bodyRuns...)
				if textBuf.Len() > 0 {
					tail := raw
					if trailingWS != "" && strings.HasSuffix(tail, trailingWS) {
						tail = tail[:len(tail)-len(trailingWS)]
					}
					if tail != "" {
						runs = append(runs, model.Run{Text: &model.TextRun{Text: tail}})
					}
				} else if trailingWS != "" && len(runs) > 0 {
					// Trailing WS was inside the last bodyRun text.
					last := &runs[len(runs)-1]
					if last.Text != nil && strings.HasSuffix(last.Text.Text, trailingWS) {
						last.Text = &model.TextRun{Text: last.Text.Text[:len(last.Text.Text)-len(trailingWS)]}
					}
				}
				if leadingWS != "" && len(runs) > 0 {
					first := &runs[0]
					if first.Text != nil && strings.HasPrefix(first.Text.Text, leadingWS) {
						first.Text = &model.TextRun{Text: first.Text.Text[len(leadingWS):]}
					}
				}
				// Drop any empty TextRuns that result from trim
				// stripping all of a run's content.
				cleaned := runs[:0]
				for _, run := range runs {
					if run.Text != nil && run.Text.Text == "" {
						continue
					}
					cleaned = append(cleaned, run)
				}
				runs = cleaned
				block = model.NewRunsBlock(blockID, runs)
			} else {
				block = model.NewBlock(blockID, text)
			}
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
				// Block's rawSource excludes any trailing whitespace —
				// that whitespace is emitted right after the ref so it
				// survives translation byte-for-byte.
				rawEnd := p.pos - len(trailingWS)
				if rawEnd < skelEnd {
					rawEnd = skelEnd
				}
				block.Properties["tex.rawSource"] = p.source[skelEnd:rawEnd]
				r.skelRef(blockID)
				if trailingWS != "" {
					r.skelText(trailingWS)
				}
				p.lastSkelPos = p.pos
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		} else if raw != "" && r.skeletonStore != nil {
			// Whitespace-only text that didn't form a block still
			// occupies real source bytes — flush them to skeleton so
			// the writer reproduces them verbatim. Without this, e.g.
			// a stray "\n" between two unknown-command data parts
			// vanishes on round-trip.
			if p.pos > p.lastSkelPos {
				r.skelText(p.source[p.lastSkelPos:p.pos])
				p.lastSkelPos = p.pos
			}
		}
		textBuf.Reset()
		rawBuf.Reset()
		bodyRuns = nil
		pcCounter = 0
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
			// Accent commands (\v{S}, \={\i}, \=A, …) — match against
			// the static accentMap and substitute the Unicode char
			// directly into the body text. Must be done BEFORE the
			// generic command-dispatch path below: `\v{S}` would
			// otherwise be classified as an unknown command and split
			// the paragraph on its `\v{S}` tokens, leaving the
			// surrounding `\=` accent fragments stranded as raw text.
			// Mirrors Okapi TEXEncoder.convertCodesToLetters which
			// runs on the writer side; doing it here keeps Block
			// source text aligned with okapi's emitted text.
			if val, n := p.matchAccentAt(p.pos); n > 0 {
				if textStartPos < 0 {
					textStartPos = p.pos
				}
				textBuf.WriteString(val)
				p.pos += n
				continue
			}
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
						runs := p.readBraceArgRuns()
						if len(runs) == 0 {
							// No brace argument — drop the command;
							// nothing translatable to emit.
							continue
						}
						p.blockCounter++
						blockID := fmt.Sprintf("tu%d", p.blockCounter)
						block := model.NewRunsBlock(blockID, runs)
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
					cmdStart := p.pos
					p.pos = cmdEnd
					// Mirror Okapi TEXParser.parse — extend the cmd past
					// any trailing DEFAULT chars and one whitespace so
					// the synthetic-space rule below fires only when the
					// peek-next token (post-extension) still starts with
					// a space. See body-mode branch + extendCmdRun for
					// rationale.
					extEnd := p.extendCmdRun(cmdEnd)
					p.pos = extEnd
					nextIsSpace := p.pos < len(p.source) && p.source[p.pos] == ' '
					p.skipOptionalArg()
					hasBraceArg := false
					for p.pos < len(p.source) && p.source[p.pos] == '{' {
						p.readBraceArgRaw()
						hasBraceArg = true
					}
					if !hasBraceArg && nextIsSpace {
						// Append synthetic separator space so the command
						// stays visually separated from following text
						// after translation. Mirrors Okapi TEXFilter
						// "addDocumentPart(token + ' ')" — see body-mode
						// branch below for full rationale.
						rawCmd := p.source[cmdStart:p.pos]
						if r.skeletonStore != nil {
							// Flush any unwritten skeleton bytes between
							// the previous emit and the command's start
							// (whitespace, paragraph breaks, …) before
							// the command's own bytes — without this
							// preceding `\n\n` runs vanish and the writer
							// concatenates the previous line directly to
							// the cmd, dropping the blank-line separator.
							if cmdStart > p.lastSkelPos {
								r.skelText(p.source[p.lastSkelPos:cmdStart])
							}
							r.skelText(rawCmd)
							r.skelText(" ")
							p.lastSkelPos = p.pos
						}
						p.dataCounter++
						d := &model.Data{
							ID:   fmt.Sprintf("d%d", p.dataCounter),
							Name: "tex-structure",
							Properties: map[string]string{
								"content": rawCmd + " ",
							},
						}
						r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d})
						continue
					}
					flushData(p.source[cmdStart:p.pos])
					continue
				}

				// Paragraph-text commands
				if paragraphTextCommands[cmd] {
					flushText()
					cmdStartPos := p.pos
					p.pos = cmdEnd
					// Skip optional argument [...]
					p.skipOptionalArg()
					runs := p.readBraceArgRuns()
					if len(runs) == 0 {
						// No brace argument — emit command as data
						flushData("\\" + cmd)
						continue
					}
					p.blockCounter++
					blockID := fmt.Sprintf("tu%d", p.blockCounter)
					block := model.NewRunsBlock(blockID, runs)
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

				// Inline-text commands — argument text is part of the
				// current paragraph but with PcOpen/PcClose markers
				// around the inner content so pseudo-translation leaves
				// the command bytes alone (mirrors Okapi TEXFilter's
				// processOneArgInlineText: emit `\cmd{` as opening Code,
				// inner text as text, `}` as closing Code).
				if inlineTextCommands[cmd] {
					if textStartPos < 0 {
						textStartPos = p.pos
					}
					cmdStart := p.pos
					p.pos = cmdEnd
					// Look for the opening brace (skip optional inter-
					// vening spaces — match readBraceArgContent's
					// skipSpaces).
					p.skipSpaces()
					if p.pos >= len(p.source) || p.source[p.pos] != '{' {
						// No brace argument — fall back to flattening
						// just the command name into text.
						textBuf.WriteString(p.source[cmdStart:p.pos])
						continue
					}
					// Capture opening "\cmd{" verbatim as PcOpen Data.
					// p.pos now points at '{'.
					openData := p.source[cmdStart : p.pos+1]
					p.pos++ // consume '{'
					flushBodyText()
					pcCounter++
					pcID := fmt.Sprintf("c%d", pcCounter)
					bodyRuns = append(bodyRuns, model.Run{PcOpen: &model.PcOpenRun{
						ID:    pcID,
						Type:  "tex:inline",
						Data:  openData,
						Equiv: cmd,
					}})
					// Read inner content into textBuf / nested runs
					// until the matching closing brace.
					p.readInlineCmdInner(&textBuf, &bodyRuns, &pcCounter, flushBodyText)
					// Append PcClose with Data="}".
					flushBodyText()
					closeData := "}"
					bodyRuns = append(bodyRuns, model.Run{PcClose: &model.PcCloseRun{
						ID:    pcID,
						Type:  "tex:inline",
						Data:  closeData,
						Equiv: cmd,
					}})
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
				// Mirror Okapi TEXParser's command tokenization: extend
				// the command past trailing DEFAULT chars (`,` `.` …)
				// and absorb one trailing whitespace byte. Without this
				// `\LaTeX,` is tokenised as `\LaTeX` only, but okapi
				// emits the cmd as `\LaTeX, ` (8 chars) so the
				// synthetic-space rule below + the remaining spaces in
				// the source produce one more output space than native.
				extEnd := p.extendCmdRun(cmdEnd)
				p.pos = extEnd
				// Detect whether the next non-command byte (after the
				// extended cmd run) is still a space — must do this
				// BEFORE skipOptionalArg, which silently consumes
				// leading spaces while looking for `[`. Okapi
				// TEXFilter peeks at the next token's content for a
				// leading space and, if present, appends a single space
				// to the command's data part WITHOUT consuming the
				// source byte (so the original spaces remain in the
				// next skeleton segment).
				nextIsSpace := p.pos < len(p.source) && p.source[p.pos] == ' '
				p.skipOptionalArg()
				hasBraceArg := false
				for p.pos < len(p.source) && p.source[p.pos] == '{' {
					p.readBraceArgRaw()
					hasBraceArg = true
				}
				if !hasBraceArg && nextIsSpace {
					// Flush any unwritten skeleton bytes between the
					// previous emit and the command's start — typically
					// the inter-paragraph whitespace that flushText
					// folded into the body's textBuf — before emitting
					// the command itself + the synthetic separator
					// space. Without this, a body unknown command that
					// follows two newlines (`paragraph\n\n\foo `) loses
					// the blank-line separator and the writer fuses the
					// preceding paragraph into the cmd.
					rawCmd := p.source[cmdStart:p.pos]
					if r.skeletonStore != nil {
						if cmdStart > p.lastSkelPos {
							r.skelText(p.source[p.lastSkelPos:cmdStart])
						}
						r.skelText(rawCmd)
						r.skelText(" ")
						p.lastSkelPos = p.pos
					}
					p.dataCounter++
					d := &model.Data{
						ID:   fmt.Sprintf("d%d", p.dataCounter),
						Name: "tex-structure",
						Properties: map[string]string{
							"content": rawCmd + " ",
						},
					}
					r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d})
					continue
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
					// Synthetic space rule (mirrors Okapi TEXFilter's
					// UnknownCommand path): when an unknown command's
					// next text token starts with whitespace, append a
					// single separator space so the cmd stays visually
					// detached after translation. `\\` (line break) is
					// classified as UnknownCommand by okapi, so e.g.
					// `verse \\             %` extends to 14 spaces in
					// the okapi reference (1 synthetic + 13 source).
					if p.pos < len(p.source) && p.source[p.pos] == ' ' {
						textBuf.WriteByte(' ')
					}
					continue
				case '&', '%', '$', '#', '_', '{', '}':
					// Escaped reserved characters — model as a leading
					// `\` Ph (carrying the backslash byte verbatim)
					// followed by the literal char in TextRun. The Ph
					// keeps the escape out of the TextRun so it
					// survives pseudo-translation as an opaque code,
					// while the TextRun ensures Block.SourceText() sees
					// the decoded literal (matches the spec contract:
					// `\%` → `%` extracted, `\%` rendered on output).
					if textStartPos < 0 {
						textStartPos = p.pos
					}
					flushBodyText()
					pcCounter++
					bodyRuns = append(bodyRuns, model.Run{Ph: &model.PlaceholderRun{
						ID:    fmt.Sprintf("c%d", pcCounter),
						Type:  "tex:escape",
						Data:  `\`,
						Equiv: "",
					}})
					textBuf.WriteByte(next)
					p.pos += 2
					// Synthetic space rule (mirrors Okapi TEXFilter's
					// UnknownCommand path) for `\$`, `\&`, `\#`, `\{`,
					// `\}`, `\_`. Okapi treats these as 2-char
					// UnknownCommand tokens whose data part absorbs a
					// trailing space when the next text token also
					// starts with whitespace. `\%` is the exception:
					// okapi registers it in accentedCharsNonLetters and
					// routes it through processAccentedChar, which does
					// NOT inject a synthetic space — so we skip the
					// synthetic for `\%` here.
					if next != '%' && p.pos < len(p.source) && p.source[p.pos] == ' ' {
						textBuf.WriteByte(' ')
					}
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

			// Default-cmd path: `\<defaultchar>` (e.g. `\,`, `\`this'`)
			// — okapi's tokenizer treats these as opaque commands that
			// extend through following alphanums/defaults until a
			// whitespace or special. Capture the matched bytes as a Ph
			// run so they round-trip verbatim and never reach the
			// pseudo translator (sample1.tex line 60: `\,\`this' `).
			if dEnd := p.extractDefaultCmd(p.pos); dEnd > p.pos+1 {
				if textStartPos < 0 {
					textStartPos = p.pos
				}
				cmdBytes := p.source[p.pos:dEnd]
				flushBodyText()
				pcCounter++
				bodyRuns = append(bodyRuns, model.Run{Ph: &model.PlaceholderRun{
					ID:    fmt.Sprintf("c%d", pcCounter),
					Type:  "tex:cmd",
					Data:  cmdBytes,
					Equiv: cmdBytes,
				}})
				p.pos = dEnd
				continue
			}

			// Backslash-space (`\ ` — TeX inter-word space command):
			// okapi's TEXParser tokenizes this as a 2-char cmd `\ `
			// (consuming one whitespace), and the unknown-command path
			// in TEXFilter appends a synthetic space ONLY when the next
			// text token also starts with whitespace. Emit it as a Data
			// part (mirroring the unknown alpha-cmd body path) so the
			// synthetic separator survives the writer's empty-paragraph
			// fold (where surrounding-only-whitespace blocks discard
			// any accumulated bodyRuns). Without this, sample1.tex
			// line 83 (`\ldots\               % comment`) loses one
			// space vs okapi.
			if p.pos+1 < len(p.source) && (p.source[p.pos+1] == ' ' || p.source[p.pos+1] == '\t') {
				flushText()
				cmdStart := p.pos
				p.pos += 2 // consume `\` + one whitespace byte
				addSynthetic := false
				if p.pos < len(p.source) {
					nb := p.source[p.pos]
					if nb == ' ' || nb == '\t' {
						addSynthetic = true
					}
				}
				rawCmd := p.source[cmdStart:p.pos]
				if addSynthetic {
					if r.skeletonStore != nil {
						if cmdStart > p.lastSkelPos {
							r.skelText(p.source[p.lastSkelPos:cmdStart])
						}
						r.skelText(rawCmd)
						r.skelText(" ")
						p.lastSkelPos = p.pos
					}
					p.dataCounter++
					d := &model.Data{
						ID:   fmt.Sprintf("d%d", p.dataCounter),
						Name: "tex-structure",
						Properties: map[string]string{
							"content": rawCmd + " ",
						},
					}
					r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d})
				} else {
					flushData(rawCmd)
				}
				continue
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

		// Tilde — TeX's non-breaking inter-word space. okapi's TEXFilter
		// emits the literal `~` byte as a DocumentPart (TEXTokenType.TILDE
		// → addDocumentPartToEventBuilder), so the source byte round-trips
		// verbatim. Modeling it as a Ph keeps it inline with the
		// surrounding text run while preventing pseudo from touching it.
		if ch0 == '~' {
			if textStartPos < 0 {
				textStartPos = p.pos
			}
			flushBodyText()
			pcCounter++
			bodyRuns = append(bodyRuns, model.Run{Ph: &model.PlaceholderRun{
				ID:    fmt.Sprintf("c%d", pcCounter),
				Type:  "tex:tilde",
				Data:  "~",
				Equiv: "~",
			}})
			p.pos++
			continue
		}

		// Superscript/subscript with brace argument — `^{…}` / `_{…}`.
		// Mirrors Okapi TEXFilter's flow: `^` and `_` emit as
		// addDocumentPartToEventBuilder (their own one-byte token), then
		// the immediately-following `{` triggers processOpenCurly. Since
		// the next token after `{` is TEXT (not COMMAND), processOpenCurly
		// falls into the "non-translatable text between brackets" branch
		// and absorbs the whole `{…}` as a document part. The composite
		// `^{2n}` / `_{i}` therefore round-trips verbatim and the inner
		// content (alphas like `n`, `i`) never reaches the pseudo
		// translator. Without this branch the inner alphas get pseudo'd
		// (sample1.tex line 128 produces `^{2ń}` instead of `^{2n}`).
		// Only fires when the brace's first non-whitespace byte is NOT a
		// `\` (i.e. not a COMMAND in okapi's token stream); a leading
		// `\` would route okapi to processOneArgInlineText and the
		// content stays translatable — fall through to the regular-char
		// path in that case so existing inline-text handling kicks in.
		if (ch0 == '^' || ch0 == '_') && p.pos+1 < len(p.source) && p.source[p.pos+1] == '{' {
			braceStart := p.pos + 1
			// Peek inside the brace, skipping leading whitespace.
			peek := braceStart + 1
			for peek < len(p.source) && (p.source[peek] == ' ' || p.source[peek] == '\t') {
				peek++
			}
			if peek < len(p.source) && p.source[peek] != '\\' {
				// Find matching `}` at depth 0.
				depth := 1
				end := braceStart + 1
				for end < len(p.source) && depth > 0 {
					switch p.source[end] {
					case '{':
						if end == 0 || p.source[end-1] != '\\' {
							depth++
						}
					case '}':
						if end == 0 || p.source[end-1] != '\\' {
							depth--
						}
					}
					if depth > 0 {
						end++
					}
				}
				if depth == 0 {
					end++ // include the closing `}`
					if textStartPos < 0 {
						textStartPos = p.pos
					}
					flushBodyText()
					pcCounter++
					raw := p.source[p.pos:end]
					bodyRuns = append(bodyRuns, model.Run{Ph: &model.PlaceholderRun{
						ID:    fmt.Sprintf("c%d", pcCounter),
						Type:  "tex:script",
						Data:  raw,
						Equiv: string(ch0),
					}})
					p.pos = end
					continue
				}
			}
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

// isOkapiTexDefault reports whether b is a "DEFAULT" character per
// Okapi's TEXParser.getCharType — i.e. anything outside the set of
// significant TeX characters that can be appended to an in-progress
// command token without ending it. Mirrors TEXParser.getCharType so
// our cmd-extension logic in extendCmdRun matches the upstream
// tokenizer byte-for-byte.
func isOkapiTexDefault(b byte) bool {
	switch b {
	case '\\', '{', '}', '$', '&', '\n', '\r', '#', '^', '_',
		' ', '\t', '~', '%', 0:
		return false
	}
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
		return false
	}
	return true
}

// extendCmdRun mirrors Okapi TEXParser.parse's command-tokenization
// loop for an alpha-named command: starting at end (the byte just past
// the command-name's last alpha char), it consumes any contiguous
// DEFAULT chars (`,` `.` `+` `-` …) and then a single trailing
// whitespace byte if present. Returns the new end position. The
// caller treats `[end:newEnd]` as part of the command token's
// rendered data — without this, `\LaTeX,                %` would be
// emitted with one less space than okapi's reference output because
// okapi's tokenizer consumes the comma + first space into the cmd
// token itself before the synthetic-space rule fires.
//
// Important: extendCmdRun must NOT cross into an optional argument
// `[…]` even though `[` itself is a DEFAULT character. Okapi's
// tokenizer special-cases `[` by greedily consuming up through the
// matching `]` into the command — but the rest of our reader expects
// `[…]` to be handled by skipOptionalArg, which re-reads the same
// bracket span. Eating `[T2A]` here would leave the rest of the
// `\usepackage[T2A]{fontenc}` line stranded in the body text stream,
// making `T2A]{fontenc}` translatable.
func (p *parser) extendCmdRun(end int) int {
	for end < len(p.source) && isOkapiTexDefault(p.source[end]) {
		if p.source[end] == '[' {
			break
		}
		end++
	}
	if end < len(p.source) && (p.source[end] == ' ' || p.source[end] == '\t') {
		end++
	}
	return end
}

// extractDefaultCmd attempts to tokenize an okapi-style "default
// command" starting at p.pos (where p.source[p.pos] == '\\'), in the
// case where the byte after `\` is a DEFAULT character (per
// Okapi TEXParser.getCharType — punctuation like `,`, `;`, “ ` “,
// `'`, `.`, `=`, `-` …) that is NOT one of our handled escape
// specials (`&`, `%`, `$`, `#`, `_`, `{`, `}`, `\`, `~`).
//
// Mirrors Okapi TEXParser.parse's cmd-loop after the first DEFAULT
// char is appended (so cmd.length() > 1): continue absorbing ALPHANUM
// and DEFAULT chars, append one trailing WHITESPACE if present, then
// stop. So `\,\`this' ` (sample1.tex line 60) tokenizes as one big
// command — okapi treats it as an AccentedChar + glued text and
// emits the bytes verbatim through the skeleton, so the inner `this`
// never reaches the pseudo translator.
//
// Returns end > start+1 when matched (caller emits p.source[start:end]
// as opaque). Returns start when no extending cmd applies (caller
// falls back to writing `\` as a stray byte).
func (p *parser) extractDefaultCmd(start int) int {
	if start+1 >= len(p.source) || p.source[start] != '\\' {
		return start
	}
	first := p.source[start+1]
	// Skip the escape-special bytes (handled by the dedicated escape
	// switch in the body parser) and alpha (handled by peekCommand).
	switch first {
	case '\\', '&', '%', '$', '#', '_', '{', '}', '~',
		'\n', '\r', ' ', '\t':
		return start
	}
	if isAlpha(first) {
		return start
	}
	// `[` would be an optional-argument bracket. Okapi's tokenizer
	// absorbs `[…]` into the command, but the rest of our reader uses
	// skipOptionalArg to handle `[…]`. Don't extend here — fall
	// through to the existing fallback.
	if first == '[' {
		return start
	}
	// `\(` and `\)` open and close inline-math regions in LaTeX.
	// Treating them as opaque commands here would absorb the space
	// after `\(` into the Ph and break paragraph segmentation around
	// the math (sample1.tex line 130 splits across 3 lines because
	// `\( \ip{A}{B} = \sum_{i} ... \)` no longer parses as one body
	// run). Defer those to the existing fallback (write `\` as text)
	// until a dedicated `\(...\)` math reader lands.
	if first == '(' || first == ')' {
		return start
	}
	end := start + 2 // consumed `\` + first DEFAULT char
	// Continue absorbing ALPHANUM and DEFAULT chars until we hit a
	// terminator (WHITESPACE — append one and stop, EOL, special, `[`).
	for end < len(p.source) {
		b := p.source[end]
		if b == '[' {
			break
		}
		// ALPHANUM
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
			end++
			continue
		}
		// DEFAULT (per isOkapiTexDefault)
		if isOkapiTexDefault(b) {
			end++
			continue
		}
		// WHITESPACE: append one and stop
		if b == ' ' || b == '\t' {
			end++
			break
		}
		// Anything else (newline, escape, group, math, etc.) → stop
		break
	}
	return end
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

// readBraceArgRuns reads a {…} argument starting at p.pos and returns
// it as a sequence of Runs. Plain text becomes TextRun runs; unknown
// `\cmd[…]{…}…` commands embedded in the argument become PlaceholderRun
// runs whose Data field carries the verbatim TeX source so the writer
// can splice them back in unchanged.
//
// Inline-text commands (\textbf, \emph, …) flatten into the surrounding
// text — same behaviour as readBraceArgText. We don't model them as
// paired codes yet because Okapi's TEXFilter doesn't either: it copies
// `{` `\command` and the closing `}` into the start/end skeleton
// markers and treats the inner content as part of the parent text.
//
// Advances p.pos past the closing brace.
func (p *parser) readBraceArgRuns() []model.Run {
	p.skipSpaces()
	if p.pos >= len(p.source) || p.source[p.pos] != '{' {
		return nil
	}
	p.pos++ // skip {
	depth := 1
	var runs []model.Run
	var textBuf strings.Builder
	flushText := func() {
		if textBuf.Len() > 0 {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: textBuf.String()}})
			textBuf.Reset()
		}
	}
	phCounter := 0
	for p.pos < len(p.source) && depth > 0 {
		ch := p.source[p.pos]
		// Embedded \command — peek to classify.
		if ch == '\\' {
			// Escaped reserved characters (`\&`, `\$`, `\%`, `\#`,
			// `\_`, `\{`, `\}`) inside a brace arg follow the same
			// pattern as in body mode (line 859-893): emit a leading
			// `\` Ph followed by the literal char as text. Okapi's
			// TEXParser tokenises `\&` as a 2-char COMMAND token (no
			// trailing whitespace absorbed because `&` triggers the
			// cmd.length()==1 break), then processCommand's
			// UnknownCommand branch peeks the next text token: when
			// it starts with " ", it appends a synthetic " " to the
			// command's data part. Mirror that here so e.g. the title
			// `... Conferences \& Symposia` round-trips byte-equal
			// (`\&  Symposia` — 2 spaces — in okapi's reference).
			if p.pos+1 < len(p.source) {
				next := p.source[p.pos+1]
				switch next {
				case '&', '%', '$', '#', '_', '{', '}':
					flushText()
					phCounter++
					runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
						ID:    fmt.Sprintf("c%d", phCounter),
						Type:  "tex:escape",
						Data:  `\`,
						Equiv: "",
					}})
					textBuf.WriteByte(next)
					p.pos += 2
					// Synthetic space rule (mirrors body parser). `\%`
					// is the exception: okapi registers it in
					// accentedCharsNonLetters and routes it through
					// processAccentedChar, which does NOT inject a
					// synthetic space — so we skip the synthetic for
					// `\%` here too.
					if next != '%' && p.pos < len(p.source) && p.source[p.pos] == ' ' {
						textBuf.WriteByte(' ')
					}
					continue
				}
			}
			cmd, cmdEnd := p.peekCommand()
			if cmd == "" {
				// Bare backslash; treat as text.
				textBuf.WriteByte('\\')
				p.pos++
				continue
			}
			// Inline-text commands flatten their argument back into the
			// surrounding text — Okapi's TEXFilter promotes them to
			// document-part skeleton, but for the brace arg of a
			// \title or \section we follow the same flatten rule the
			// readBraceArgText path used.
			if inlineTextCommands[cmd] {
				p.pos = cmdEnd
				p.readBraceArgContent(&textBuf, cmd)
				continue
			}
			// Unknown / no-text command — capture verbatim as a Ph run.
			cmdStart := p.pos
			p.pos = cmdEnd
			p.skipOptionalArg()
			for p.pos < len(p.source) && p.source[p.pos] == '{' {
				p.readBraceArgRaw()
			}
			flushText()
			phCounter++
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID:    fmt.Sprintf("c%d", phCounter),
				Type:  "tex:cmd",
				Data:  p.source[cmdStart:p.pos],
				Equiv: cmd,
			}})
			continue
		}
		switch ch {
		case '{':
			if p.pos == 0 || p.source[p.pos-1] != '\\' {
				depth++
			}
			if depth > 1 {
				textBuf.WriteByte(ch)
			}
			p.pos++
		case '}':
			if p.pos == 0 || p.source[p.pos-1] != '\\' {
				depth--
			}
			if depth > 0 {
				textBuf.WriteByte(ch)
				p.pos++
			} else {
				// Closing brace — consume and exit.
				p.pos++
			}
		case '%':
			// Comment inside the brace arg — okapi's TEXFilter routes
			// COMMENT tokens to addDocumentPartToEventBuilder, which
			// inside a TextUnit becomes a Code (placeholder) carrying
			// the verbatim comment bytes. The translatable text stream
			// thus excludes the comment, preventing things like
			// `\author{Alice% <-comment\nBob}` from pseudo-translating
			// the words inside the `% <-comment` line. We capture the
			// `%`-to-EOL run (including the trailing newline so its
			// `\thanks{...}` follow-on ends up on the next line) as a
			// Ph run whose Data round-trips verbatim.
			cmtStart := p.pos
			for p.pos < len(p.source) && p.source[p.pos] != '\n' {
				p.pos++
			}
			if p.pos < len(p.source) {
				p.pos++ // include trailing newline (matches readToEndOfLine)
			}
			flushText()
			phCounter++
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID:    fmt.Sprintf("c%d", phCounter),
				Type:  "tex:comment",
				Data:  p.source[cmtStart:p.pos],
				Equiv: "%",
			}})
		default:
			textBuf.WriteByte(ch)
			p.pos++
		}
	}
	flushText()
	return runs
}

// readInlineCmdInner reads the content of an inline-text command's
// brace argument (e.g. the `…` in `\emph{…}`), starting just AFTER
// the opening `{`. Plain text accumulates into textBuf; nested
// inline-text commands emit nested PcOpen/PcClose pairs into runs;
// unknown / no-text embedded commands emit Ph runs whose Data field
// carries the raw TeX bytes verbatim. Advances p.pos past the
// matching `}` (which is consumed; the caller emits its own PcClose
// using "}" as Data).
//
// The flushBodyText callback is the parent paragraph's textBuf →
// runs flush — required so nested code runs land between parent text
// chunks in the right order.
func (p *parser) readInlineCmdInner(textBuf *strings.Builder, runs *[]model.Run, pcCounter *int, flushBodyText func()) {
	depth := 1
	for p.pos < len(p.source) && depth > 0 {
		ch := p.source[p.pos]
		// Embedded backslash sequence — peek to classify.
		if ch == '\\' {
			cmd, cmdEnd := p.peekCommand()
			if cmd == "" {
				// Bare backslash; treat as text (e.g. `\\` line break
				// is handled by the parent loop, but inside braces a
				// stray '\' just becomes literal).
				textBuf.WriteByte('\\')
				p.pos++
				continue
			}
			// Nested inline-text command: emit a paired PcOpen/PcClose
			// around its inner content.
			if inlineTextCommands[cmd] {
				cmdStart := p.pos
				p.pos = cmdEnd
				p.skipSpaces()
				if p.pos >= len(p.source) || p.source[p.pos] != '{' {
					// No brace argument — flatten command name as text.
					textBuf.WriteString(p.source[cmdStart:p.pos])
					continue
				}
				openData := p.source[cmdStart : p.pos+1]
				p.pos++ // consume '{'
				flushBodyText()
				*pcCounter++
				pcID := fmt.Sprintf("c%d", *pcCounter)
				*runs = append(*runs, model.Run{PcOpen: &model.PcOpenRun{
					ID:    pcID,
					Type:  "tex:inline",
					Data:  openData,
					Equiv: cmd,
				}})
				p.readInlineCmdInner(textBuf, runs, pcCounter, flushBodyText)
				flushBodyText()
				*runs = append(*runs, model.Run{PcClose: &model.PcCloseRun{
					ID:    pcID,
					Type:  "tex:inline",
					Data:  "}",
					Equiv: cmd,
				}})
				continue
			}
			// Unknown / no-text command — capture as a Ph run so its
			// raw bytes (including any [opt] / {arg} sequences) splice
			// back verbatim through RenderRunsWithData.
			cmdStart := p.pos
			p.pos = cmdEnd
			p.skipOptionalArg()
			for p.pos < len(p.source) && p.source[p.pos] == '{' {
				p.readBraceArgRaw()
			}
			flushBodyText()
			*pcCounter++
			*runs = append(*runs, model.Run{Ph: &model.PlaceholderRun{
				ID:    fmt.Sprintf("c%d", *pcCounter),
				Type:  "tex:cmd",
				Data:  p.source[cmdStart:p.pos],
				Equiv: cmd,
			}})
			continue
		}
		switch ch {
		case '{':
			if p.pos == 0 || p.source[p.pos-1] != '\\' {
				depth++
			}
			if depth > 1 {
				textBuf.WriteByte(ch)
			}
			p.pos++
		case '}':
			if p.pos == 0 || p.source[p.pos-1] != '\\' {
				depth--
			}
			if depth > 0 {
				textBuf.WriteByte(ch)
				p.pos++
			} else {
				// Matching closing brace — consume and exit.
				p.pos++
			}
		default:
			textBuf.WriteByte(ch)
			p.pos++
		}
	}
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
