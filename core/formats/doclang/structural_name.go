package doclang

import (
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// A block's Name is its structural address in the document tree, not the order
// the reader reached it in. reconcile.Identify folds the name into the block's
// context hash and grades content and context as independent signals, so a name
// that counts blocks as they go past makes the context signal worthless: delete
// one paragraph and every name below it moves. See core/model/structural.go.
//
// A DocLang address is the chain of enclosing containers — group, list, picture,
// table — down to the element itself. Each step is the element name plus an
// occurrence ordinal counted among the same-named children of its own parent, so
// the first such child stays unsuffixed and an edit in one container cannot
// shift anything in another. A table cell takes the grid address OTSL already
// gives it: `table/row#2/cell#3` beats any count.

// docFrame is one open container.
type docFrame struct {
	name string
	step string
	// ordinals counts children by name. Living on the parent is what scopes an
	// ordinal to it.
	ordinals map[string]int
}

func (f *docFrame) reserve(name string) int {
	if f.ordinals == nil {
		f.ordinals = map[string]int{}
	}
	f.ordinals[name]++
	return f.ordinals[name]
}

// docPath tracks the open containers of one document. One per read: the
// ordinals it hands out are scoped to that document.
type docPath struct {
	frames []docFrame
	root   map[string]int
}

// ordinalStep renders a step, suffixing the ordinal only past the first
// occurrence so appending a sibling never renames the one already there.
func ordinalStep(name string, ordinal int) string {
	if ordinal > 1 {
		return name + model.NameOrdinalSeparator + strconv.Itoa(ordinal)
	}
	return name
}

// reserve counts one more child named name under the innermost open container
// (or at document level) and returns its step.
func (p *docPath) reserve(name string) string {
	if n := len(p.frames); n > 0 {
		return ordinalStep(name, p.frames[n-1].reserve(name))
	}
	if p.root == nil {
		p.root = map[string]int{}
	}
	p.root[name]++
	return ordinalStep(name, p.root[name])
}

// push opens a container that was already given its step by reserve.
func (p *docPath) push(name, step string) {
	p.frames = append(p.frames, docFrame{name: name, step: step})
}

// pop closes the innermost open container.
func (p *docPath) pop() {
	if n := len(p.frames); n > 0 {
		p.frames = p.frames[:n-1]
	}
}

// name composes the open containers with any extra steps into a block name.
func (p *docPath) name(extra ...string) string {
	steps := make([]string, 0, len(p.frames)+len(extra))
	for i := range p.frames {
		steps = append(steps, p.frames[i].step)
	}
	steps = append(steps, extra...)
	return model.StructuralPath(steps...)
}
