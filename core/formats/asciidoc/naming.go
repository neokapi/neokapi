package asciidoc

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// How an AsciiDoc block is named — its structural address in the document.
//
// AsciiDoc carries two things a counter cannot match, and both are used here:
//
//   - Section headings. A block's address is the trail of headings enclosing
//     it plus what kind of block it is: `Install/Prerequisites/p`. Delete a
//     paragraph and the ones below it keep their names, because a heading trail
//     does not renumber. Two paragraphs under one heading are genuinely
//     indistinguishable by structure, so the ordinal that separates them is
//     scoped to that heading (model.NameBuilder) and an edit in a different
//     section cannot move it.
//   - Block anchors — `[[install]]` and `[#install]`. An anchor is an address
//     the author wrote down, document-unique by definition (it is what a
//     cross-reference resolves against), so it stands alone as the name and no
//     trail is prefixed. A block with an anchor keeps its name even when it
//     moves to a different section, which is exactly what the author meant by
//     giving it one.
//
// The document title is deliberately NOT part of the trail. It would sit in
// every name in the file and buy no discrimination — core/reconcile already
// scopes a name to its document — while making an edit to the title rename
// every block under it.
//
// Table cells are addressed by the table they sit in plus their row and column.
// That is geometry, but a cell's position in its table IS its structure; what a
// counter would do instead is renumber every cell in the file when a paragraph
// three sections up is deleted.
//
// A heading trail is written in the document's own language, so the same
// paragraph is named `Install/Prerequisites/p` in the source and something else
// entirely in its translation. Each name therefore has a translation-invariant
// twin, carried on the block as model.StructureAnnotation.Address: the same path
// with each section written as the identity of the heading that opened it
// (`h2`, `h2#2`) rather than as its title. The name stays what everything else
// keys on — reconcile's context hash, the state store, the XLIFF `name`
// attribute — and the address answers the one question it cannot, which is
// whether two blocks in two languages are the same unit.
//
// Block.ID is untouched: it stays the `tuN` skeleton join the writer matches on.

var (
	// reAnchorLine matches a standalone block anchor: `[[install]]`, optionally
	// with a cross-reference label (`[[install,Installing]]`).
	reAnchorLine = regexp.MustCompile(`^\[\[([A-Za-z_:][\w:.-]*)(?:,[^\]]*)?\]\]$`)
	// reIDAttr matches the `#id` shorthand inside a block-attribute line —
	// `[#install]`, `[#install.lead]`, `[quote#install]`. The id stops at the
	// role (`.`) and option (`%`) separators that may follow it.
	reIDAttr = regexp.MustCompile(`^\[[^\]]*#([A-Za-z_:][\w:-]*)[^\]]*\]$`)
)

// blockAnchor returns the anchor a block-attribute line assigns, or "".
func blockAnchor(line string) string {
	if m := reAnchorLine.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	if m := reIDAttr.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

// trail returns the heading segments enclosing the current position, outermost
// first.
func (r *Reader) trail() []string {
	out := make([]string, 0, len(r.groupStack))
	for _, f := range r.groupStack {
		if f.kind == "section" && f.title != "" {
			out = append(out, f.title)
		}
	}
	return out
}

// addrTrail is trail over the section IDENTITIES rather than the section
// titles: what a section is called varies by language, what it is does not.
func (r *Reader) addrTrail() []string {
	out := make([]string, 0, len(r.groupStack))
	for _, f := range r.groupStack {
		if f.kind == "section" && f.addr != "" {
			out = append(out, f.addr)
		}
	}
	return out
}

// sectionPath builds a structural path for a block at the current position:
// the enclosing heading trail followed by segs.
func (r *Reader) sectionPath(segs ...string) string {
	return model.StructuralPath(append(r.trail(), segs...)...)
}

// blockAddress issues the translation-invariant address for a block whose
// structural path is the given one (model.StructureAnnotation.Address): the
// same path with the enclosing heading titles replaced by the identities of the
// headings that opened them, so a source document and its translation address
// the block identically. It returns "" when there is nothing to rewrite —
// outside any section, or for a path that was not built from the current trail.
//
// A pending address set by emitHeading wins: a heading's own address is issued
// before its section opens, because the section is named by it.
func (r *Reader) blockAddress(path string) string {
	if addr := r.pendingAddress; addr != "" {
		r.pendingAddress = ""
		return addr
	}
	prefix := model.StructuralPath(r.trail()...)
	if prefix == "" {
		return ""
	}
	rest, ok := strings.CutPrefix(path, prefix+model.PathSeparator)
	if !ok {
		return ""
	}
	return r.addrNames.Name(model.StructuralPath(append(r.addrTrail(), rest)...))
}

// headingAddress issues the address of a heading block, which is composed from
// its PARENT trail — the heading is what opens the section beneath it, so it
// cannot be addressed inside it. openSection reads the section's own segment
// off the result.
func (r *Reader) headingAddress(level int) string {
	return r.addrNames.Name(model.StructuralPath(append(r.addrTrail(), fmt.Sprintf("h%d", level))...))
}

// lastSegment returns the final segment of a path — what a nested section
// carries forward as its own identity.
func lastSegment(path string) string {
	if i := strings.LastIndex(path, model.PathSeparator); i >= 0 {
		return path[i+1:]
	}
	return path
}

// blockName resolves the name a block is emitted under: a pending author anchor
// when one precedes it, otherwise the structural path the caller built. Either
// way it goes through the builder, so a path that genuinely repeats within the
// document is disambiguated rather than silently shared.
func (r *Reader) blockName(path string) string {
	if anchor := r.pendingAnchor; anchor != "" {
		r.pendingAnchor = ""
		return r.names.Name(anchor)
	}
	return r.names.Name(path)
}

// openSection opens a section group and records the segment that names
// everything inside it, plus the heading identity that addresses it.
func (r *Reader) openSection(ctx context.Context, ch chan<- model.PartResult, level int, title, addr string) bool {
	if !r.openGroup(ctx, ch, "section", "section", level, nil) {
		return false
	}
	top := &r.groupStack[len(r.groupStack)-1]
	top.title = title
	top.addr = addr
	return true
}
