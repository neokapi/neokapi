package epub

import (
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// A block's Name is its structural address, not the order it was read in:
// reconcile.Identify folds the name into the block's context hash and grades
// content and context as independent signals, so a name that counts blocks as
// they go past makes the context signal worthless. See core/model/structural.go.
//
// An EPUB is a container of XHTML documents, so an address has two parts: the
// spine item the block came from, and where it sits inside that item's markup.
// Naming a block by its item alone — which is what this reader used to do — gave
// every paragraph of a chapter the same address.
//
// Inside a document the step is the element name plus a `[n]` ordinal counted
// among the same-named children of that element's own parent. The first such
// child stays unsuffixed, so appending a sibling never renames the one already
// there, and an edit in one chapter cannot move anything in another.

// xhtmlFrame is one open element of an XHTML document.
type xhtmlFrame struct {
	name string
	step string
	// ordinals counts children by name, and text runs flushed directly under
	// this element. Living on the parent is what scopes an ordinal to it.
	ordinals map[string]int
}

func (f *xhtmlFrame) reserve(name string) int {
	if f.ordinals == nil {
		f.ordinals = map[string]int{}
	}
	f.ordinals[name]++
	return f.ordinals[name]
}

// xhtmlPath tracks the open elements of one XHTML document. One per document:
// the ordinals it hands out are scoped to that document.
type xhtmlPath struct {
	item   string
	frames []xhtmlFrame
	root   map[string]int
}

// textStepName is the step for a text run, spelled the way XPath spells it.
const textStepName = "text()"

func newXHTMLPath(itemPath string) *xhtmlPath { return &xhtmlPath{item: itemPath} }

// ordinalStep renders a step, suffixing the ordinal only past the first
// occurrence so appending a sibling never renames the one already there.
func ordinalStep(name string, ordinal int) string {
	if ordinal > 1 {
		return name + "[" + strconv.Itoa(ordinal) + "]"
	}
	return name
}

// reserve counts one more child named name under the innermost open element (or
// at document level) and returns its step.
func (p *xhtmlPath) reserve(name string) string {
	if n := len(p.frames); n > 0 {
		return ordinalStep(name, p.frames[n-1].reserve(name))
	}
	if p.root == nil {
		p.root = map[string]int{}
	}
	p.root[name]++
	return ordinalStep(name, p.root[name])
}

// push opens an element that was already given its step by reserve.
func (p *xhtmlPath) push(name, step string) {
	p.frames = append(p.frames, xhtmlFrame{name: name, step: step})
}

// pop closes the innermost open element when it is the one ending. A mismatch
// means the markup left an element unclosed; the frame stays rather than
// popping somebody else's.
func (p *xhtmlPath) pop(name string) {
	if n := len(p.frames); n > 0 && p.frames[n-1].name == name {
		p.frames = p.frames[:n-1]
	}
}

// implicitStructureTags are the XHTML wrappers dropped from a path. Every
// document has exactly one of each, so they discriminate nothing — and dropping
// them lines these names up with the ones the HTML reader produces when the
// same chapter is routed through it as a subfilter.
var implicitStructureTags = map[string]bool{
	"html": true,
	"head": true,
	"body": true,
}

// name composes the item path, the open elements and any extra steps into a
// block name. The item path contributes its own segments so the result reads as
// one location rather than a file name glued to a tree address.
func (p *xhtmlPath) name(extra ...string) string {
	steps := make([]string, 0, len(p.frames)+len(extra)+4)
	steps = append(steps, strings.Split(p.item, "/")...)
	for i := range p.frames {
		if implicitStructureTags[p.frames[i].name] {
			continue
		}
		steps = append(steps, p.frames[i].step)
	}
	steps = append(steps, extra...)
	return model.StructuralPath(steps...)
}

// textRun names a text run flushed under the innermost open element. The first
// run under an element takes the element's own address; a second run — text
// resuming after a nested block, say — takes an ordinaled text() step, counted
// on that element so nothing outside it can shift it.
func (p *xhtmlPath) textRun() string {
	var ordinal int
	if n := len(p.frames); n > 0 {
		ordinal = p.frames[n-1].reserve(textStepName)
	} else {
		if p.root == nil {
			p.root = map[string]int{}
		}
		p.root[textStepName]++
		ordinal = p.root[textStepName]
	}
	if ordinal == 1 {
		return p.name()
	}
	return p.name(ordinalStep(textStepName, ordinal))
}
