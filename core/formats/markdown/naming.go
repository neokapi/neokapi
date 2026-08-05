package markdown

import (
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/model"
)

// Block names in markdown are STRUCTURAL addresses, not counters.
//
// A markdown document's structure is its heading trail plus the containers a
// block sits inside — a list, a table row, an HTML block. A block is addressed
// by that path and its kind:
//
//	getting-started/install/p        a paragraph under two headings
//	getting-started/install/p#2      the next paragraph in the same section
//	getting-started/install/list/item
//	getting-started/install/table/row#2/cell
//	getting-started/h                the "Install" heading ITSELF — see Heading
//
// A heading is the one block whose name is deliberately NOT its own text: it is
// addressed by its parent trail and its ordinal there, so rewording it is an
// edit rather than a replacement. Heading explains why at length.
//
// The kind is part of the path so a paragraph and a list item under one heading
// cannot collide, and the ordinal that separates genuine repeats is scoped to
// the tightest enclosing structure — a section, a list, a row. An edit in one
// section therefore cannot shift a name in another.
//
// What this does NOT fix, and cannot: two paragraphs under one heading are
// indistinguishable by structure, so deleting the first still re-addresses the
// second. See core/model/structural.go for why content is not an option (the
// name IS the context hash, so deriving it from content collapses the two
// signals reconcile grades independently), and
// TestMarkdownNames_ShiftWithinTheSameSection for the pinned behaviour.
//
// The name is also read by people. `kapi extract` drives the source file's own
// reader and hands the blocks to the XLIFF 2 writer, which writes a block's Name
// as the unit's `name` attribute for any format that is not itself XLIFF (see
// xliff2.(*Writer).unitNameAttr). A translator opening the file therefore sees
// `name="getting-started/install/p"` where the counter gave them "para3".
//
// Block.ID is unaffected. It is the skeleton join — the reader writes a ref, the
// writer matches on it — lives inside one read/write pass, and stays a counter.

// Block-name kinds. One per translatable atom the reader emits, so that two
// atoms at the same structural position stay distinguishable by kind.
const (
	kindHeading      = "h"
	kindParagraph    = "p"
	kindListItem     = "item"
	kindCell         = "cell"
	kindCode         = "code"
	kindMath         = "math"
	kindHTML         = "html"
	kindHTMLText     = "text"
	kindHTMLAttr     = "attr"
	kindAdmonTitle   = "admon-title"
	kindAdmonKeyword = "admon-keyword"
	kindAdmonBody    = "admon-body"
	kindDocuAdmon    = "docu-admon-title"
	kindRefLabel     = "ref-label"
	kindRefTitle     = "ref-title"
)

// Structural scopes. A scope is a container that owns the ordinals of what it
// holds, so an edit inside one cannot re-address the contents of another.
const (
	scopeList  = "list"
	scopeTable = "table"
	scopeRow   = "row"
	scopeQuote = "quote"
	scopeHTML  = "html"
)

// fallbackSegment names a heading that yields no usable characters at all (a
// rule of asterisks, an emoji). It still has to address the blocks beneath it.
const fallbackSegment = "section"

// maxSegmentRunes bounds one path segment. A heading can be a whole sentence;
// the address is meant to be read, and NamingState disambiguates any collision
// truncation introduces.
const maxSegmentRunes = 60

// namingSection is one entry in the heading trail: the level that opened it and
// the full segment path it addresses.
type namingSection struct {
	level int
	segs  []string
}

// NamingState composes structural block names for one document read.
//
// It is exported because MDX drives several markdown spans through separate
// markdown.Readers to read ONE document (see mdx.Reader). Sharing this state
// across those spans is what stops names from restarting — and colliding — at
// every JSX element. Within a plain .md read the reader owns its own.
//
// The zero value is ready to use. One state per document: the ordinals it hands
// out are scoped to that document, so sharing one across files would make a name
// depend on which file was read first.
type NamingState struct {
	builder  model.NameBuilder
	sections []namingSection
	scope    []string
}

// Reset clears the state for a new document.
func (s *NamingState) Reset() {
	s.builder.Reset()
	s.sections = nil
	s.scope = nil
}

// prefix returns the current structural path — the innermost heading trail plus
// any open scopes — as a fresh slice the caller may append to.
func (s *NamingState) prefix() []string {
	var sectionSegs []string
	if n := len(s.sections); n > 0 {
		sectionSegs = s.sections[n-1].segs
	}
	segs := make([]string, 0, len(sectionSegs)+len(s.scope)+1)
	segs = append(segs, sectionSegs...)
	return append(segs, s.scope...)
}

// Name returns the name for a block of the given kind at the current position,
// with an ordinal appended when that position genuinely repeats.
func (s *NamingState) Name(kind string) string {
	return s.builder.Name(model.StructuralPath(append(s.prefix(), kind)...))
}

// Skip consumes the ordinal a block of this kind would have taken without
// emitting one, so that structural position keeps its meaning: an empty table
// cell still occupies its column, and an empty list item still occupies its
// slot, so what follows is addressed by where it is rather than by how many
// neighbours happened to carry text.
func (s *NamingState) Skip(kind string) { _ = s.Name(kind) }

// Push enters a structure that owns its contents' ordinals — a list, a table, a
// row — and returns the function that leaves it. Use it as `defer Push(...)()`.
func (s *NamingState) Push(kind string) func() {
	return s.pushSegment(lastSegment(s.Name(kind)))
}

// PushName enters a structure already addressed by a block name, so a list
// item's nested content hangs off the item itself (`.../item#2/list/item`)
// rather than off the list.
func (s *NamingState) PushName(name string) func() {
	return s.pushSegment(lastSegment(name))
}

func (s *NamingState) pushSegment(seg string) func() {
	s.scope = append(s.scope, seg)
	depth := len(s.scope)
	return func() {
		if len(s.scope) >= depth {
			s.scope = s.scope[:depth-1]
		}
	}
}

// Heading opens the section this heading introduces and returns the heading
// block's own name. Levels nest: a level-2 heading closes any open level-2 or
// deeper section and hangs off the nearest shallower one.
//
// A heading is named by its PARENT trail and its ordinal among the headings
// there — never by its own text. That distinction is the whole point:
//
//   - Its own name must not move when its words change. The name is the CONTEXT
//     hash and the words are the CONTENT hash; change both at once and reconcile
//     has nothing left to match on, so a reworded heading grades New and loses
//     its history and its translation. Naming a block after its own content is
//     precisely the collapse this scheme exists to prevent, and a heading is the
//     one place it looks tempting.
//   - Its text still addresses everything BENEATH it. That is what makes a
//     descendant's name readable and stable, and it means rewording a heading
//     MOVES its contents (content unchanged, trail changed) rather than
//     replacing them.
//
// The trail is built from headings only. A heading nested inside a list or a
// blockquote (legal, and rare) addresses off its parent SECTION, so an open
// scope cannot leak into every block that follows the list.
func (s *NamingState) Heading(level int, text string) string {
	for len(s.sections) > 0 && s.sections[len(s.sections)-1].level >= level {
		s.sections = s.sections[:len(s.sections)-1]
	}

	var parent []string
	if n := len(s.sections); n > 0 {
		parent = s.sections[n-1].segs
	}

	// The heading's own name, from the parent trail alone.
	own := s.builder.Name(model.StructuralPath(appendSegment(parent, kindHeading)...))

	// The section it opens, addressed by its text. Two headings with the same
	// text under one parent are disambiguated here, once, rather than leaving
	// every block beneath them to interleave.
	seg := lastSegment(s.builder.Name(model.StructuralPath(appendSegment(parent, headingSlug(text))...)))
	s.sections = append(s.sections, namingSection{level: level, segs: appendSegment(parent, seg)})

	return own
}

// appendSegment returns parent with seg appended, without aliasing parent's
// backing array — the section trail keeps a reference to it.
func appendSegment(parent []string, seg string) []string {
	segs := make([]string, 0, len(parent)+1)
	segs = append(segs, parent...)
	return append(segs, seg)
}

// lastSegment returns the final segment of a path — the part a nested scope
// carries forward. NameBuilder appends its ordinal to the end of the path, so
// `install/list#2` yields `list#2`.
func lastSegment(path string) string {
	if i := strings.LastIndex(path, model.PathSeparator); i >= 0 {
		return path[i+1:]
	}
	return path
}

// headingSlug turns heading text into one path segment: lower-cased, words
// joined by hyphens, everything else dropped. Letters and digits of any script
// are kept, so a heading in a script without case addresses by its own
// characters rather than by a placeholder.
func headingSlug(text string) string {
	var b strings.Builder
	runes := 0
	pendingHyphen := false
	for _, rn := range text {
		if unicode.IsLetter(rn) || unicode.IsDigit(rn) {
			if pendingHyphen && b.Len() > 0 {
				b.WriteByte('-')
				runes++
				pendingHyphen = false
			}
			if runes >= maxSegmentRunes {
				break
			}
			b.WriteRune(unicode.ToLower(rn))
			runes++
			continue
		}
		pendingHyphen = true
	}
	if b.Len() == 0 {
		return fallbackSegment
	}
	return b.String()
}
