package openxml

import (
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// A block's Name is its structural address inside the package, not the order the
// reader reached it in. reconcile.Identify folds the name into the block's
// context hash and grades content and context as independent signals, so a name
// that counts blocks as they go past makes the context signal worthless: delete
// one paragraph and every name below it moves. Blocks read out of an OOXML
// package used to carry no Name at all, which left convergence.BlockKey falling
// back to the `tuN` reader counter — reading order exactly. See
// core/model/structural.go.
//
// An OOXML address starts with the part the content lives in (`word/document.xml`,
// `xl/worksheets/sheet1.xml`) — parts are independent files, so an edit in one
// can never move anything in another. Inside a part it continues with the
// structure the format already has: table / row / cell for WordprocessingML, the
// sheet cell reference for SpreadsheetML, the shape for DrawingML. Each step
// carries an occurrence ordinal counted among the same-named siblings of its own
// parent, so the first stays unsuffixed and appending one never renames another.
// The ordinal is spelled with model.NameOrdinalSeparator, the marker anything
// aligning on names reads as "this side encodes position".

// oxmlScope is one open structural level of a part.
type oxmlScope struct {
	name string
	step string
	// ordinals counts children by name. Living on the parent is what scopes an
	// ordinal to it.
	ordinals map[string]int
}

func (s *oxmlScope) reserve(name string) int {
	if s.ordinals == nil {
		s.ordinals = map[string]int{}
	}
	s.ordinals[name]++
	return s.ordinals[name]
}

// oxmlPath addresses content within one package part. One per part: the
// ordinals it hands out are scoped to that part, so reading order across parts
// cannot leak into a name.
type oxmlPath struct {
	part      string
	partSteps []string
	scopes    []oxmlScope
	root      map[string]int
}

// ordinalStep renders a step, suffixing the ordinal only past the first
// occurrence so appending a sibling never renames the one already there.
func ordinalStep(name string, ordinal int) string {
	if ordinal > 1 {
		return name + model.NameOrdinalSeparator + strconv.Itoa(ordinal)
	}
	return name
}

// ensurePart roots the path at a package part the first time that part is
// named. Re-rooting mid-part would reset the ordinals and rename everything
// after it, so a repeat call for the same part is a no-op.
func (p *oxmlPath) ensurePart(partPath string) {
	if p.part == partPath && p.partSteps != nil {
		return
	}
	p.part = partPath
	p.partSteps = strings.Split(partPath, "/")
	p.scopes = nil
	p.root = nil
}

// reserve counts one more child named name under the innermost open scope (or
// at part level) and returns its step.
func (p *oxmlPath) reserve(name string) string {
	if n := len(p.scopes); n > 0 {
		return ordinalStep(name, p.scopes[n-1].reserve(name))
	}
	if p.root == nil {
		p.root = map[string]int{}
	}
	p.root[name]++
	return ordinalStep(name, p.root[name])
}

// push opens a scope, reserving its step under the scope enclosing it.
func (p *oxmlPath) push(name string) {
	p.scopes = append(p.scopes, oxmlScope{name: name, step: p.reserve(name)})
}

// pushStep opens a scope whose step is already known — a spreadsheet cell
// reference, say, which is a better address than any count.
func (p *oxmlPath) pushStep(name, step string) {
	p.scopes = append(p.scopes, oxmlScope{name: name, step: step})
}

// pop closes the innermost open scope when it is the one named. A mismatch
// means the part left an element unclosed; the scope stays rather than popping
// somebody else's.
func (p *oxmlPath) pop(name string) {
	if n := len(p.scopes); n > 0 && p.scopes[n-1].name == name {
		p.scopes = p.scopes[:n-1]
	}
}

// name composes the part, the open scopes and any extra steps into a block name.
func (p *oxmlPath) name(extra ...string) string {
	steps := make([]string, 0, len(p.partSteps)+len(p.scopes)+len(extra))
	steps = append(steps, p.partSteps...)
	for i := range p.scopes {
		steps = append(steps, p.scopes[i].step)
	}
	steps = append(steps, extra...)
	return model.StructuralPath(steps...)
}
