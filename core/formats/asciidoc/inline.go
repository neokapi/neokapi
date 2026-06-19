package asciidoc

import (
	"regexp"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
)

// This file linearizes AsciiDoc inline markup into the canonical Run
// vocabulary (Framework AD-002: inline markup lives in runs, never in text).
// The parser is LOSSLESS by construction: every input byte is accounted for
// either as TextRun content or inside an inline code run's Data, so
// model.RenderRunsWithData over the produced runs reproduces the input
// byte-for-byte. That is what lets the writer round-trip an untranslated
// document exactly.
//
// Canonical types emitted (grounded in core/formats/constructs.yaml and the
// core/model vocabulary packs):
//   *strong*   -> fmt:bold            (paired)
//   _emphasis_ -> fmt:italic          (paired)
//   `mono`     -> fmt:code            (paired)
//   ^super^    -> fmt:superscript     (paired, unconstrained)
//   ~sub~      -> fmt:subscript       (paired, unconstrained)
//   https://x[text] / link:t[text] -> link:hyperlink (paired)
//   {attr}     -> code:variable       (standalone placeholder)
//   <<id,text>> -> code:markup around the visible text (paired)
//   <<id>>     -> code:markup         (standalone placeholder)

var (
	reAttrRef   = regexp.MustCompile(`^\{[A-Za-z0-9_][A-Za-z0-9_-]*\}`)
	reXref      = regexp.MustCompile(`^<<[^<>\n]+>>`)
	reLinkMacro = regexp.MustCompile(`^(?:link:|mailto:|(?:https?|ftp|irc)://)[^\[\s\]]+\[[^\]\n]*\]`)
	reBold      = regexp.MustCompile(`^\*([^*\n]+)\*`)
	reItalic    = regexp.MustCompile(`^_([^_\n]+)_`)
	reMono      = regexp.MustCompile("^`([^`\\n]+)`")
	reSuper     = regexp.MustCompile(`^\^([^\^\s\n]+)\^`)
	reSub       = regexp.MustCompile(`^~([^~\s\n]+)~`)
)

// parseInline returns the canonical Run sequence for a span of AsciiDoc inline
// text. The concatenation of every run's literal content (TextRun text plus
// inline-code Data) equals the input string.
func parseInline(text string) []model.Run {
	b := newRunBuilder()
	id := 0
	i := 0
	n := len(text)
	for i < n {
		if adv := tryConstruct(b, text, i, &id); adv > 0 {
			i += adv
			continue
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		b.AddText(text[i : i+size])
		i += size
	}
	return b.Runs()
}

// tryConstruct attempts to match an inline construct anchored at position i in
// text. On success it appends the construct's runs to b and returns the number
// of bytes consumed; otherwise it returns 0.
func tryConstruct(b *runBuilder, text string, i int, id *int) int {
	rest := text[i:]

	// Attribute reference {name} -> standalone placeholder.
	if m := reAttrRef.FindString(rest); m != "" {
		*id++
		b.AddPh(strconv.Itoa(*id), "code:variable", "asciidoc:attribute-ref", m, m[1:len(m)-1])
		return len(m)
	}

	// Cross reference <<id[,text]>>.
	if m := reXref.FindString(rest); m != "" {
		inner := m[2 : len(m)-2] // between << and >>
		if comma := indexByteRune(inner, ','); comma >= 0 {
			label := inner[comma+1:]
			open := "<<" + inner[:comma+1] // "<<id,"
			*id++
			pairID := strconv.Itoa(*id)
			b.AddPcOpen(pairID, "code:markup", "asciidoc:xref", open, "")
			b.AddText(label)
			b.AddPcClose(pairID, "code:markup", "asciidoc:xref", ">>")
			return len(m)
		}
		// No visible text — opaque placeholder.
		*id++
		b.AddPh(strconv.Itoa(*id), "code:markup", "asciidoc:xref", m, "")
		return len(m)
	}

	// Link / URL macro target[text].
	if m := reLinkMacro.FindString(rest); m != "" {
		open := indexByteRune(m, '[')
		if open > 0 {
			*id++
			pairID := strconv.Itoa(*id)
			b.AddPcOpen(pairID, "link:hyperlink", "asciidoc:link", m[:open+1], "")
			b.AddText(m[open+1 : len(m)-1])
			b.AddPcClose(pairID, "link:hyperlink", "asciidoc:link", "]")
			return len(m)
		}
	}

	// Constrained formatting: *bold*, _italic_, `mono`.
	if adv := tryConstrainedPair(b, text, i, id, reBold, "*", "fmt:bold"); adv > 0 {
		return adv
	}
	if adv := tryConstrainedPair(b, text, i, id, reItalic, "_", "fmt:italic"); adv > 0 {
		return adv
	}
	if adv := tryConstrainedPair(b, text, i, id, reMono, "`", "fmt:code"); adv > 0 {
		return adv
	}

	// Unconstrained formatting: ^super^, ~sub~ (may sit mid-word, e.g. H~2~O).
	if adv := tryUnconstrainedPair(b, rest, id, reSuper, "^", "fmt:superscript"); adv > 0 {
		return adv
	}
	if adv := tryUnconstrainedPair(b, rest, id, reSub, "~", "fmt:subscript"); adv > 0 {
		return adv
	}

	return 0
}

// tryConstrainedPair matches an AsciiDoc constrained inline-format pair. The
// "constrained" rule (https://docs.asciidoctor.org/asciidoc/latest/text/) is
// that the marker may not abut a word character on the outside, and the
// content may not start or end with a space. Without this `2*3*4` would be read
// as bold.
func tryConstrainedPair(b *runBuilder, text string, i int, id *int, re *regexp.Regexp, marker, typ string) int {
	loc := re.FindStringSubmatchIndex(text[i:])
	if loc == nil {
		return 0
	}
	inner := text[i+loc[2] : i+loc[3]]
	if hasSpaceEdge(inner) {
		return 0
	}
	// Left boundary: preceding char must not be a word character.
	if i > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:i])
		if isWordRune(r) {
			return 0
		}
	}
	// Right boundary: following char must not be a word character.
	end := i + loc[1]
	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if isWordRune(r) {
			return 0
		}
	}
	emitPair(b, id, marker, typ, inner)
	return loc[1]
}

// tryUnconstrainedPair matches a ^super^ / ~sub~ pair with no word-boundary
// constraint (these are unconstrained formatting pairs in AsciiDoc).
func tryUnconstrainedPair(b *runBuilder, rest string, id *int, re *regexp.Regexp, marker, typ string) int {
	loc := re.FindStringSubmatchIndex(rest)
	if loc == nil {
		return 0
	}
	emitPair(b, id, marker, typ, rest[loc[2]:loc[3]])
	return loc[1]
}

func emitPair(b *runBuilder, id *int, marker, typ, inner string) {
	*id++
	pairID := strconv.Itoa(*id)
	b.AddPcOpen(pairID, typ, "", marker, "")
	b.AddText(inner)
	b.AddPcClose(pairID, typ, "", marker)
}

func hasSpaceEdge(s string) bool {
	if s == "" {
		return true
	}
	first, _ := utf8.DecodeRuneInString(s)
	last, _ := utf8.DecodeLastRuneInString(s)
	return unicode.IsSpace(first) || unicode.IsSpace(last)
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// indexByteRune returns the index of the first occurrence of b in s, or -1.
func indexByteRune(s string, b byte) int {
	for i := range len(s) {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// runBuilder accumulates a []model.Run, coalescing adjacent text runs so the
// run sequence stays minimal (and losslessly reconstructible).
type runBuilder struct {
	runs []model.Run
}

func newRunBuilder() *runBuilder { return &runBuilder{} }

// AddText appends plain text, merging with a trailing TextRun when present.
func (b *runBuilder) AddText(text string) {
	if text == "" {
		return
	}
	if n := len(b.runs); n > 0 && b.runs[n-1].Text != nil {
		b.runs[n-1].Text.Text += text
		return
	}
	b.runs = append(b.runs, model.Run{Text: &model.TextRun{Text: text}})
}

// AddPh appends a self-closing placeholder run.
func (b *runBuilder) AddPh(id, typ, subType, data, equiv string) {
	b.runs = append(b.runs, model.Run{Ph: &model.PlaceholderRun{
		ID: id, Type: typ, SubType: subType, Data: data, Equiv: equiv,
	}})
}

// AddPcOpen appends the opening half of a paired inline code.
func (b *runBuilder) AddPcOpen(id, typ, subType, data, equiv string) {
	b.runs = append(b.runs, model.Run{PcOpen: &model.PcOpenRun{
		ID: id, Type: typ, SubType: subType, Data: data, Equiv: equiv,
	}})
}

// AddPcClose appends the closing half of a paired inline code.
func (b *runBuilder) AddPcClose(id, typ, subType, data string) {
	b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
		ID: id, Type: typ, SubType: subType, Data: data,
	}})
}

// Runs returns the accumulated runs (always non-nil).
func (b *runBuilder) Runs() []model.Run {
	if b.runs == nil {
		return []model.Run{}
	}
	return b.runs
}
