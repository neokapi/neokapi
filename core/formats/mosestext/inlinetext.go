package mosestext

import (
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// This file implements Moses InlineText decoding — the inline-markup
// convention Okapi's MosesTextFilter recognises inside each Moses text
// entry. It is a direct port of two pieces of
// net.sf.okapi.filters.mosestext.MosesTextFilter:
//
//   - the entry-grouping loop in next(), which folds `<mrk mtype="seg">…
//     </mrk>` segment annotations (possibly spanning several physical
//     lines) into a single text-unit body; and
//   - fromPseudoXLIFF(), which decodes the pseudo-XLIFF surface of an
//     entry: XML entities, paired `<g id="N">…</g>` codes, isolated
//     `<x|bx|ex id="N"/>` codes, and `<lb/>` hard line breaks.
//
// The native reader keeps full parity with the upstream regexes and
// replacement order so the extracted source text matches Okapi
// character-for-character.

var (
	// startSegment matches the opening of a `<mrk mtype="seg" …>`
	// segment annotation at the start of a line. Mirrors
	// MosesTextFilter.STARTSEGMENT.
	startSegment = regexp.MustCompile(`<mrk\s+mtype\s*=\s*?["']seg["'].*?>`)

	// endSegment is the literal segment terminator. Mirrors
	// MosesTextFilter.ENDSEGMENT.
	endSegment = "</mrk>"

	// openClose matches a paired inline code: an opening `<g id="N">`
	// (group 1, id in group 3) or a closing `</g>` (group 4). Mirrors
	// MosesTextFilter.OPENCLOSE.
	openClose = regexp.MustCompile(`(<g(\s+)id=['"](.*?)['"]>)|(</g>)`)

	// isolated matches a self-closing `<x|bx|ex id="N"/>` code. Mirrors
	// MosesTextFilter.ISOLATED.
	isolated = regexp.MustCompile(`<(bx|ex|x)(\s+)id=['"](.*?)['"](\s*?)/>`)

	// lineBreak matches the `<lb/>` hard line break. Mirrors
	// MosesTextFilter.LINEBREAK.
	lineBreak = regexp.MustCompile(`(<lb\s*?/>)`)

	// crEntity matches the carriage-return numeric character references
	// `&#13;` / `&#xD;` (with optional leading zeros). Mirrors the first
	// replaceAll in MosesTextFilter.fromPseudoXLIFF.
	crEntity = regexp.MustCompile(`(&#13;)|(&#x0*?[dD];)`)
)

// propEncode is the Block property key the reader sets on Moses
// InlineText-decoded blocks. Its presence tells the writer to re-encode
// the body to pseudo-XLIFF on output (so a read → write round trip
// reproduces the input byte-for-byte). Code-finder blocks omit it and
// are rendered verbatim instead.
const propEncode = "mosestext:encode"

// encodeInlineTextValue marks a block as Moses InlineText so the writer
// re-encodes it.
const encodeInlineTextValue = "inlinetext"

// hasInlineMarkup reports whether an entry body needs the full
// pseudo-XLIFF decode. Mirrors the early-out in fromPseudoXLIFF: an
// entry with neither `<` nor `&` is plain text and is returned verbatim.
func hasInlineMarkup(text string) bool {
	return strings.ContainsRune(text, '<') || strings.ContainsRune(text, '&')
}

// encodeInlineText re-encodes a decoded Run sequence back to the Moses
// InlineText (pseudo-XLIFF) surface, the inverse of decodeInlineText.
// Text-run characters are escaped exactly as Okapi's MosesTextEncoder
// does (`<`→`&lt;`, `&`→`&amp;`, `\r`→`&#13;`, `\n`→`<lb/>`); inline-code
// runs re-emit their captured original tag (`Data`) verbatim. This is
// the writer-side counterpart that keeps read → write byte-exact.
func encodeInlineText(runs []model.Run) string {
	var b strings.Builder
	for _, r := range runs {
		switch {
		case r.Text != nil:
			encodeMosesText(&b, r.Text.Text)
		case r.Ph != nil:
			b.WriteString(r.Ph.Data)
		case r.PcOpen != nil:
			b.WriteString(r.PcOpen.Data)
		case r.PcClose != nil:
			b.WriteString(r.PcClose.Data)
		case r.Sub != nil:
			b.WriteString(r.Sub.Ref)
		}
	}
	return b.String()
}

// encodeMosesText escapes plain text to the Moses InlineText surface,
// mirroring MosesTextEncoder.encode(String,…). Note `>` is NOT escaped
// (the skeleton-writer encoder leaves it literal, unlike the standalone
// MosesTextFilterWriter which conditionally escapes `>` after `]`).
func encodeMosesText(b *strings.Builder, text string) {
	for _, ch := range text {
		switch ch {
		case '<':
			b.WriteString("&lt;")
		case '&':
			b.WriteString("&amp;")
		case '\r':
			b.WriteString("&#13;")
		case '\n':
			b.WriteString("<lb/>")
		default:
			b.WriteRune(ch)
		}
	}
}

// decodeInlineText converts a Moses InlineText entry body into a Run
// sequence. Plain entries (no markup) yield a single TextRun; entries
// with markup are decoded into TextRun + paired/placeholder code runs,
// matching MosesTextFilter.fromPseudoXLIFF.
func decodeInlineText(text string) []model.Run {
	if text == "" {
		return []model.Run{{Text: &model.TextRun{Text: ""}}}
	}
	if !hasInlineMarkup(text) {
		return []model.Run{{Text: &model.TextRun{Text: text}}}
	}

	// Decode entities first — the upstream filter does these literal
	// replacements before any code matching so e.g. `&lt;g …&gt;` would
	// decode prior to (failing to) match the code regexes. Order and
	// targets mirror fromPseudoXLIFF exactly.
	text = crEntity.ReplaceAllString(text, "\r")
	text = strings.ReplaceAll(text, "&apos;", "'")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&amp;", "&")

	// Decode `<lb/>` hard line breaks to literal newlines.
	text = lineBreak.ReplaceAllString(text, "\n")

	// Parse paired and isolated codes into runs.
	return parseCodes(text)
}

// codeToken is an intermediate placeholder for a parsed inline code,
// remembered while the surrounding text is sliced apart.
type codeToken struct {
	kind model.RunKind // pcOpen | pcClose | ph
	id   string        // shared id for paired open/close, or the code id
	data string        // the original tag text (for round-trip)
	typ  string        // run Type tag
}

// parseCodes walks an entity-decoded entry and splits it into TextRun
// and inline-code runs. Paired `<g>` tags become PcOpen / PcClose with a
// shared id; `<x>` becomes a placeholder; `<bx>` / `<ex>` become paired
// open / close keyed on their id (mirroring MosesTextFilter's Xpt
// pairing). Tags are matched on the entity-decoded text, identical to
// the order in fromPseudoXLIFF.
func parseCodes(text string) []model.Run {
	type match struct {
		start, end int
		tok        codeToken
	}
	var matches []match

	// Opening / closing `<g>` markers. The upstream filter generates a
	// stable integer id from the opening tag's id attribute and pairs the
	// closing tag to the most recent unclosed opening via a stack.
	var stack []string
	for _, loc := range openClose.FindAllStringSubmatchIndex(text, -1) {
		full := text[loc[0]:loc[1]]
		// loc indices: group 1 = [2:3], group 3 (id) = [6:7],
		// group 4 (closing) = [8:9].
		if loc[2] >= 0 { // opening <g id="…">
			id := text[loc[6]:loc[7]]
			matches = append(matches, match{loc[0], loc[1], codeToken{
				kind: model.RunKindPcOpen, id: id, data: full, typ: "g",
			}})
			stack = append(stack, id)
		} else { // closing </g>
			id := ""
			if n := len(stack); n > 0 {
				id = stack[n-1]
				stack = stack[:n-1]
			}
			matches = append(matches, match{loc[0], loc[1], codeToken{
				kind: model.RunKindPcClose, id: id, data: full, typ: "g",
			}})
		}
	}

	// Isolated `<x|bx|ex id="…"/>` codes.
	for _, loc := range isolated.FindAllStringSubmatchIndex(text, -1) {
		full := text[loc[0]:loc[1]]
		name := text[loc[2]:loc[3]]
		id := text[loc[6]:loc[7]]
		var tok codeToken
		switch name {
		case "bx":
			tok = codeToken{kind: model.RunKindPcOpen, id: "Xpt" + id, data: full, typ: "Xpt" + id}
		case "ex":
			tok = codeToken{kind: model.RunKindPcClose, id: "Xpt" + id, data: full, typ: "Xpt" + id}
		default: // x
			tok = codeToken{kind: model.RunKindPh, id: id, data: full, typ: "x"}
		}
		matches = append(matches, match{loc[0], loc[1], tok})
	}

	if len(matches) == 0 {
		return []model.Run{{Text: &model.TextRun{Text: text}}}
	}

	// Sort matches by start offset (insertion sort keeps it dependency
	// free and the slices are tiny).
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	var runs []model.Run
	last := 0
	for _, m := range matches {
		if m.start < last {
			continue // overlapping match — skip
		}
		if m.start > last {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: text[last:m.start]}})
		}
		switch m.tok.kind {
		case model.RunKindPcOpen:
			runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{
				ID: m.tok.id, Type: m.tok.typ, Data: m.tok.data,
			}})
		case model.RunKindPcClose:
			runs = append(runs, model.Run{PcClose: &model.PcCloseRun{
				ID: m.tok.id, Type: m.tok.typ, Data: m.tok.data,
			}})
		case model.RunKindPh:
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID: m.tok.id, Type: m.tok.typ, Data: m.tok.data,
			}})
		}
		last = m.end
	}
	if last < len(text) {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: text[last:]}})
	}
	return runs
}
