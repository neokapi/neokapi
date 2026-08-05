package html

import (
	"strconv"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/neokapi/neokapi/core/model"
)

// A block's Name is its structural address in the DOM — the element path from
// the document root — not the order the reader reached it in. reconcile.Identify
// folds the name into the block's context hash and grades content and context as
// independent signals, so a name that counts blocks as they go past makes the
// context signal worthless: delete one paragraph and every name below it moves.
// See core/model/structural.go.
//
// Three rules shape an HTML path:
//
//   - An `id` is a natural key the author controls, so it IS the step, and the
//     path stops climbing there. `nav-id/a#2` survives the whole subtree being
//     moved somewhere else in the document.
//   - Otherwise the step is the tag name, plus an occurrence ordinal counted
//     among the SAME-TAGGED children of that element's own parent. The first
//     such child stays unsuffixed, so appending a sibling never renames the one
//     already there, and an edit in another subtree cannot shift it. The ordinal
//     is spelled with model.NameOrdinalSeparator because that is the marker
//     anything aligning on names reads as "this side encodes position" — kdiff
//     switches from name alignment to content alignment on it.
//   - `html`, `head` and `body` are dropped. Every document has exactly one of
//     each, so they discriminate nothing — and dropping them is what lets the
//     DOM path (which sees the parser's synthesized html/body) and the tokenizer
//     path (which sees only what the source wrote) agree on a name.
//
// Text nodes have no tag, so they take the XPath-flavoured `text()` step,
// ordinaled among their parent's text children.

// implicitStructureTags are the elements dropped from a path. The HTML parser
// synthesizes them when the source omits them, so including them would make a
// fragment and a full document name the same paragraph differently.
var implicitStructureTags = map[atom.Atom]bool{
	atom.Html: true,
	atom.Head: true,
	atom.Body: true,
}

// textStepName is the step for a text node — XPath's spelling, since a text node
// is exactly what it means.
const textStepName = "text()"

// ordinalStep renders one path step, suffixing the ordinal only past the first
// occurrence.
func ordinalStep(name string, ordinal int) string {
	if ordinal > 1 {
		return name + model.NameOrdinalSeparator + strconv.Itoa(ordinal)
	}
	return name
}

// idStep renders the step for an element carrying an id. The `-id` suffix marks
// the step as a natural key rather than a tag name, and keeps a page whose
// element is `<p id="p">` from naming it the same as a bare second `<p>`.
func idStep(id string) string { return id + "-id" }

// nodeStep returns the path step for one DOM node.
func nodeStep(n *html.Node) string {
	if n.Type == html.TextNode {
		ordinal := 1
		for sib := n.PrevSibling; sib != nil; sib = sib.PrevSibling {
			if sib.Type == html.TextNode {
				ordinal++
			}
		}
		return ordinalStep(textStepName, ordinal)
	}
	if id := getAttr(n, "id"); id != "" {
		return idStep(id)
	}
	ordinal := 1
	for sib := n.PrevSibling; sib != nil; sib = sib.PrevSibling {
		if sib.Type == html.ElementNode && sib.Data == n.Data {
			ordinal++
		}
	}
	return ordinalStep(n.Data, ordinal)
}

// nodePath returns the structural name for a DOM node: its step, prefixed by the
// steps of its ancestors up to the document root or the nearest id anchor.
func nodePath(n *html.Node) string {
	if n == nil {
		return ""
	}
	var steps []string
	for cur := n; cur != nil && cur.Type != html.DocumentNode; cur = cur.Parent {
		if cur != n && cur.Type == html.ElementNode && implicitStructureTags[cur.DataAtom] {
			continue
		}
		steps = append(steps, nodeStep(cur))
		if cur.Type == html.ElementNode && getAttr(cur, "id") != "" {
			break
		}
	}
	reverse(steps)
	return model.StructuralPath(steps...)
}

// attributePath names a block extracted from an element's attribute, so a
// translatable `alt` and the element's own text never share an address.
func attributePath(n *html.Node, attrKey string) string {
	return nodePath(n) + "@" + attrKey
}

// pathFrame is one open element on the tokenizer path. The tokenizer has no DOM
// to climb, so the path is kept as elements open and close.
type pathFrame struct {
	tag  string
	a    atom.Atom
	id   string
	step string
	// childOrdinals counts this element's children by name. The counts live
	// on the parent, which is exactly what scopes an ordinal to it.
	childOrdinals map[string]int
}

func (f *pathFrame) reserveOrdinal(name string) int {
	if f.childOrdinals == nil {
		f.childOrdinals = map[string]int{}
	}
	f.childOrdinals[name]++
	return f.childOrdinals[name]
}

// pushPath records an element that stays open — a container whose end tag the
// main loop will see. An element consumed whole (a leaf block, a script body) is
// never an ancestor of anything and must not be pushed.
func (s *tokenReaderState) pushPath(tag string, a atom.Atom, id, step string) {
	s.pathStack = append(s.pathStack, pathFrame{tag: tag, a: a, id: id, step: step})
}

// popPath closes the innermost open element when it is the one ending. A
// mismatch means the source left an element unclosed; the frame is left in
// place rather than popping somebody else's.
func (s *tokenReaderState) popPath(tag string) {
	if n := len(s.pathStack); n > 0 && s.pathStack[n-1].tag == tag {
		s.pathStack = s.pathStack[:n-1]
	}
}

// ancestorPath renders the open elements as path steps, dropping the implicit
// html/head/body wrappers and stopping at the nearest id anchor — the same
// composition nodePath performs on the DOM side.
func (s *tokenReaderState) ancestorPath() []string {
	var steps []string
	for i := len(s.pathStack) - 1; i >= 0; i-- {
		f := &s.pathStack[i]
		if f.id == "" && implicitStructureTags[f.a] {
			continue
		}
		steps = append(steps, f.step)
		if f.id != "" {
			break
		}
	}
	reverse(steps)
	return steps
}

// structuralName composes the open-element path with one more step.
func (s *tokenReaderState) structuralName(step ...string) string {
	return model.StructuralPath(append(s.ancestorPath(), step...)...)
}

// reserveStep assigns tag's sibling ordinal under the innermost open element
// (or at document level) and returns the step. It must be called once per start
// tag, in document order.
func (s *tokenReaderState) reserveStep(tag, id string) string {
	ordinal := s.reserveOrdinal(tag)
	if id != "" {
		return idStep(id)
	}
	return ordinalStep(tag, ordinal)
}

// reserveOrdinal counts one more child named name under the innermost open
// element, or at document level when nothing is open.
func (s *tokenReaderState) reserveOrdinal(name string) int {
	if n := len(s.pathStack); n > 0 {
		return s.pathStack[n-1].reserveOrdinal(name)
	}
	if s.rootOrdinals == nil {
		s.rootOrdinals = map[string]int{}
	}
	s.rootOrdinals[name]++
	return s.rootOrdinals[name]
}

// textStep names a text run by its position among its parent's text runs, so
// text elsewhere in the document cannot move it.
func (s *tokenReaderState) textStep() string {
	return ordinalStep(textStepName, s.reserveOrdinal(textStepName))
}

// inlineChildName names an inline element consumed INSIDE a leaf block — an
// `<img alt>` within a `<p>`. Such an element never becomes an open ancestor,
// so the enclosing leaf block is the tightest structure available to scope it
// to, and its ordinal is counted there and reset with each leaf block.
func (s *tokenReaderState) inlineChildName(tag string, attrs []html.Attribute) string {
	step := ""
	if id := getTokenAttr(attrs, "id"); id != "" {
		step = idStep(id)
	} else {
		if s.leafOrdinals == nil {
			s.leafOrdinals = map[string]int{}
		}
		s.leafOrdinals[tag]++
		step = ordinalStep(tag, s.leafOrdinals[tag])
	}
	return s.structuralName(s.currentStep, step)
}

func reverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
