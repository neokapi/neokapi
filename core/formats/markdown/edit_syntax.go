package markdown

import (
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// An edited Markdown block is written with format.RenderEditedRuns: markup the
// block held when it was read is the document's own, and markup the edit adds
// is backslash-escaped so CommonMark reads it as text. The reader keeps the
// document's backslash escapes, and the brackets and angle brackets that open
// nothing, in text runs; those are the markup an edit can keep.

// EditSyntax is the markup CommonMark can read from the text of an edited
// Markdown block. A '<' followed by a letter, '/', '!' or '?' can open raw
// HTML, an HTML block at the start of a line, an autolink or a comment, and
// runs to the next '>'. A '[' can open a link, an image, a reference or a
// link reference definition, and runs to its closing ']' with the destination,
// reference or definition after it. An '&' can begin a character reference.
// A character already preceded by a backslash is text. Text inside a code
// span is literal.
var EditSyntax = format.AddedSyntax{
	Find:   findEditSyntax,
	Escape: backslashEscape,
	InCode: codeSpanRun,
}

// maxSyntaxSpan bounds how far Find looks for the end of markup, so a block
// full of brackets costs a bounded scan per bracket.
const maxSyntaxSpan = 4096

// entityPrefixRE matches a character reference at the start of a string.
var entityPrefixRE = regexp.MustCompile(`^&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]{1,7}|#[xX][0-9A-Fa-f]{1,6});`)

func findEditSyntax(s string, i int) (start, end int, ok bool) {
	if backslashEscaped(s, i) {
		return 0, 0, false
	}
	switch s[i] {
	case '<':
		if i+1 < len(s) && opensTag(s[i+1]) {
			return i, tagEnd(s, i), true
		}
	case '[':
		return i, bracketEnd(s, i), true
	case '&':
		if m := entityPrefixRE.FindStringIndex(s[i:]); m != nil {
			return i, i + m[1], true
		}
	}
	return 0, 0, false
}

// backslashEscape writes a backslash before the character, which CommonMark
// reads as that character for any ASCII punctuation.
func backslashEscape(s string, i int) (int, string) {
	return 1, `\` + s[i:i+1]
}

// codeSpanRun reports whether a run opens or closes a code span.
func codeSpanRun(r model.Run) (opens, closes bool) {
	switch {
	case r.PcOpen != nil:
		return r.PcOpen.Type == "fmt:code", false
	case r.PcClose != nil:
		return false, r.PcClose.Type == "fmt:code"
	}
	return false, false
}

// opensTag reports whether a '<' followed by c can open markup.
func opensTag(c byte) bool {
	return c == '/' || c == '!' || c == '?' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// tagEnd returns the offset just past the first '>' after s[i], or the end of
// the bounded span when none follows.
func tagEnd(s string, i int) int {
	limit := min(len(s), i+maxSyntaxSpan)
	if j := strings.IndexByte(s[i+1:limit], '>'); j >= 0 {
		return i + 1 + j + 1
	}
	return limit
}

// bracketEnd returns the offset just past the bracketed text that opens at
// s[i], with what follows it as markup: an inline destination in parentheses,
// a reference in brackets, or a definition's ':' and the rest of its line. An
// unclosed bracket runs to the end of its line.
func bracketEnd(s string, i int) int {
	limit := min(len(s), i+maxSyntaxSpan)
	j := matchClose(s, i, limit, '[', ']')
	if j < 0 {
		return lineEnd(s, i, limit)
	}
	k := j + 1
	if k >= limit {
		return k
	}
	switch s[k] {
	case '(':
		if e := matchClose(s, k, limit, '(', ')'); e >= 0 {
			return e + 1
		}
		return lineEnd(s, k, limit)
	case '[':
		if e := matchClose(s, k, limit, '[', ']'); e >= 0 {
			return e + 1
		}
	case ':':
		return lineEnd(s, k, limit)
	}
	return k
}

// matchClose returns the offset of the delimiter that closes the one at
// s[open], counting nested pairs and skipping backslash-escaped delimiters,
// or -1 when none does before limit.
func matchClose(s string, open, limit int, opener, closer byte) int {
	depth := 0
	for j := open; j < limit; j++ {
		switch s[j] {
		case '\\':
			j++
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// lineEnd returns the offset of the first line break at or after i, or limit.
func lineEnd(s string, i, limit int) int {
	if j := strings.IndexByte(s[i:limit], '\n'); j >= 0 {
		return i + j
	}
	return limit
}

// htmlSourceSyntax is the markup in a raw HTML block the HTML subfilter left
// whole: a tag, comment or declaration from '<', and a character reference
// from '&'. An edit's wording is escaped as HTML text.
var htmlSourceSyntax = format.AddedSyntax{
	Find: func(s string, i int) (int, int, bool) {
		switch s[i] {
		case '<':
			if i+1 < len(s) && opensTag(s[i+1]) {
				return i, tagEnd(s, i), true
			}
		case '&':
			if m := entityPrefixRE.FindStringIndex(s[i:]); m != nil {
				return i, i + m[1], true
			}
		}
		return 0, 0, false
	},
	Escape: htmlCharEscape,
}

// htmlAttrSyntax is the markup in an attribute value the HTML subfilter read:
// a character reference, and the quotes and angle brackets that end the value
// or open a tag. A quote the value held as read is one its own quoting
// allows, so keeping it is safe; one the edit adds is escaped.
var htmlAttrSyntax = format.AddedSyntax{
	Find: func(s string, i int) (int, int, bool) {
		switch s[i] {
		case '"', '\'', '<', '>':
			return i, i + 1, true
		case '&':
			if m := entityPrefixRE.FindStringIndex(s[i:]); m != nil {
				return i, i + m[1], true
			}
		}
		return 0, 0, false
	},
	Escape: htmlCharEscape,
}

// htmlCharEscape writes the character reference for an HTML delimiter.
func htmlCharEscape(s string, i int) (int, string) {
	switch s[i] {
	case '<':
		return 1, "&lt;"
	case '>':
		return 1, "&gt;"
	case '"':
		return 1, "&quot;"
	case '\'':
		return 1, "&#39;"
	case '&':
		return 1, "&amp;"
	}
	return 1, s[i : i+1]
}

// htmlTextEscaper encodes decoded text for HTML element content.
var htmlTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;")

// renderEditedSource renders an edited Markdown block; see RenderEditedSource.
func renderEditedSource(block *model.Block, read []model.Run) string {
	return RenderEditedSource(block, read, EditSyntax)
}

// RenderEditedSource renders a block whose source an edit has rewritten
// (model.Block.SourceAsRead), encoding the edit's wording for the block's
// place in the document. Code blocks, math and front matter hold literal
// text. The HTML subfilter's text blocks hold decoded text, so an edit is
// encoded as HTML text; its raw HTML blocks and attribute values hold HTML
// source. Every other block holds inline markup, which inline describes:
// EditSyntax for Markdown, a wider set for MDX. The result still needs
// FinishBlockContent.
func RenderEditedSource(block *model.Block, read []model.Run, inline format.AddedSyntax) string {
	switch block.Type {
	case "front-matter", "code-block", "math":
		return model.RenderRunsWithData(block.Source)
	case "html-text":
		var b strings.Builder
		model.RenderRunsWith(&b, block.Source, &model.RunRenderer{
			Text: func(b *strings.Builder, text string) { _, _ = htmlTextEscaper.WriteString(b, text) },
		})
		return b.String()
	case "html-block":
		return format.RenderEditedRuns(block.Source, read, htmlSourceSyntax)
	case "html-attr":
		return format.RenderEditedRuns(block.Source, read, htmlAttrSyntax)
	}
	if block.SemanticRole() == model.RoleCode {
		return model.RenderRunsWithData(block.Source)
	}
	return format.RenderEditedRuns(block.Source, read, inline)
}

// FinishBlockContent applies the block-level spelling the skeleton splice
// needs (front matter quoting, a continuation prefix, table-cell pipes) to a
// block's rendered runs, as RenderBlockContent does for runs it renders
// itself.
func FinishBlockContent(block *model.Block, rendered string) string {
	return finishBlockContent(block, rendered)
}
