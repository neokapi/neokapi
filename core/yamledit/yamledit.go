// Package yamledit writes a Go value back into an existing YAML document
// without disturbing what the document says about itself.
//
// Governance files — a recipe, a voice profile — are committed, human-authored
// source. Their comments carry the reasoning a reviewer needs, and their key
// order is the order someone chose to explain themselves in. A write-back that
// marshals the struct over the file keeps every field and loses all of that: the
// change lands correctly and deletes the file's explanation of itself, silently.
//
// Two properties, and the second matters as much as the first:
//
//   - COMMENT-PRESERVING. The value supplies the data; the document on disk
//     supplies the comments, the blank lines between its sections, the key
//     order, and how each value is spelled — a scalar's quoting or block style,
//     and the mapping-or-scalar form a key whose type accepts both was authored
//     in. A key the value no longer carries is removed, and one it adds is
//     appended — an edit, not a re-issue.
//
//   - BYTE-STABLE. A write whose result is identical to what is already there
//     does not happen at all. This is not tidiness: `scripts/check-sync-backed.sh`
//     reads any change under `.kapi/` as the decision backing a run's derived
//     artifacts, and asks for backing over the run as a whole. A file that
//     churns on its own therefore manufactures backing for artifacts nothing
//     decided — the failure the gate exists to prevent, arriving through a path
//     no human chose. It follows the terms projection, which compares the merged
//     document against the bytes on disk for the same reason.
//
// What it does not preserve is a key the value's type does not model. The
// loaders are non-strict, so such a key is already dropped on the way in; the
// document written back is the one the type can describe.
package yamledit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultIndent is the nesting width used for a document that has none to copy
// (a new file, or one with no nested block).
const DefaultIndent = 2

// Marshal renders value as YAML, carrying over the comments, blank lines, key
// order, scalar styles and indentation of original. An empty original is a plain
// marshal at DefaultIndent.
func Marshal(original []byte, value any) ([]byte, error) {
	fresh, err := encodeNode(value)
	if err != nil {
		return nil, err
	}

	indent := DefaultIndent
	var base *yaml.Node
	var authored map[*yaml.Node]int
	if len(bytes.TrimSpace(original)) > 0 {
		var doc yaml.Node
		if err := yaml.Unmarshal(original, &doc); err == nil && doc.Kind == yaml.DocumentNode {
			base = &doc
			authored = lineIndex(base)
			merge(base, fresh, baselineFor(original, value))
			fresh = base
			indent = detectIndent(original)
		}
		// A document that will not parse carries nothing to preserve: the fresh
		// marshal stands, which is what the caller would have written anyway.
	}

	rendered, err := encode(fresh, indent)
	if err != nil {
		return nil, err
	}
	if base != nil {
		rendered = restoreSpacing(original, rendered, base, authored)
	}

	// A document that already says what the value says is returned untouched,
	// whatever the emitter would have made of it. Re-emitting is not lossless in
	// the small — a folded scalar is re-wrapped at the emitter's width, not the
	// author's — and byte-stability has to survive presentation the writer cannot
	// reproduce, or the first write after any such file churns for no decision.
	if len(original) > 0 && sameContent(original, rendered) {
		return original, nil
	}
	return rendered, nil
}

// encode renders a node tree at the given nesting width.
func encode(node *yaml.Node, indent int) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(indent)
	if err := enc.Encode(node); err != nil {
		return nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close yaml encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// baselineFor renders what the marshaller would write for the document exactly
// as it stands: the document loaded into the value's own type and marshalled
// straight back. It is the reference a merge needs to tell a value that changed
// from one the type merely spells differently — a `voice:` mapping the type
// collapses to its scalar short form is not an edit anybody made, and the merge
// can only know that by seeing the same collapse happen to the untouched
// document. nil when the document does not load into the type, which leaves the
// merge with the fresh marshal alone.
func baselineFor(original []byte, value any) *yaml.Node {
	t := reflect.TypeOf(value)
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	loaded := reflect.New(t).Interface()
	if err := yaml.Unmarshal(original, loaded); err != nil {
		return nil
	}
	node, err := encodeNode(loaded)
	if err != nil {
		return nil
	}
	return node
}

// sameContent reports whether two documents carry the same data, ignoring
// comments, key order and every presentation choice. Both are decoded to plain
// Go values and re-marshalled canonically, so the comparison is of what the
// documents say, not of how they say it.
func sameContent(a, b []byte) bool {
	ca, ok := canonicalDoc(a)
	if !ok {
		return false
	}
	cb, ok := canonicalDoc(b)
	return ok && bytes.Equal(ca, cb)
}

func canonicalDoc(doc []byte) ([]byte, bool) {
	var v any
	if err := yaml.Unmarshal(doc, &v); err != nil {
		return nil, false
	}
	return canonicalValue(v)
}

func canonicalValue(v any) ([]byte, bool) {
	out, err := yaml.Marshal(v)
	if err != nil {
		return nil, false
	}
	return out, true
}

// WriteFile renders value into path, preserving whatever document is already
// there, and reports whether the bytes moved. An unchanged serialization is not
// written — the file keeps its mtime as well as its content.
//
// The write is atomic (temp file + rename), so a crash mid-write cannot leave a
// governance file half-rewritten.
func WriteFile(path string, value any, perm os.FileMode) (changed bool, err error) {
	original, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, fmt.Errorf("read %s: %w", path, readErr)
	}
	next, err := Marshal(original, value)
	if err != nil {
		return false, err
	}
	if readErr == nil && bytes.Equal(original, next) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, next, perm); err != nil {
		return false, fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return false, fmt.Errorf("rename %s: %w", tmp, err)
	}
	return true, nil
}

// encodeNode marshals a value into the node tree the merge works over.
func encodeNode(value any) (*yaml.Node, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal yaml: %w", err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("parse marshalled yaml: %w", err)
	}
	return &node, nil
}

// merge folds the fresh node's data into base in place: base keeps its comments,
// its key order and its scalar styles; fresh decides what the document says.
// baseline is the same position in the untouched document's own marshal, and may
// be nil where the document does not reach that far.
//
// Nodes of different kinds do not merge — the value's shape wins outright, since
// a mapping that became a list is a different answer, not an edited one. Unless
// the value at that position did not change: a type that accepts both a scalar
// and a mapping for one key marshals to whichever form its data implies, so the
// shape the fresh marshal arrives in is the type's habit rather than a decision,
// and the document keeps the form it was authored in.
func merge(base, fresh, baseline *yaml.Node) {
	if base == nil || fresh == nil || base.Kind != fresh.Kind {
		if base != nil && fresh != nil {
			if sameData(baseline, fresh) {
				return
			}
			carryComments(fresh, base)
			*base = *fresh
		}
		return
	}
	switch base.Kind {
	case yaml.DocumentNode:
		if len(base.Content) == 1 && len(fresh.Content) == 1 {
			merge(base.Content[0], fresh.Content[0], childAt(baseline, 0))
			return
		}
		base.Content = fresh.Content
	case yaml.MappingNode:
		mergeMapping(base, fresh, baseline)
	case yaml.SequenceNode:
		mergeSequence(base, fresh, baseline)
	default:
		// Scalar (and alias): the value is the fresh one, the presentation is the
		// document's — a quoted string stays quoted, a block scalar stays a block.
		style := base.Style
		tag := base.Tag
		carryComments(fresh, base)
		base.Value = fresh.Value
		base.Tag = fresh.Tag
		base.Style = fresh.Style
		if fresh.Tag == tag {
			base.Style = style
		}
	}
}

// mergeMapping rebuilds base's pairs in base's order, keeping only the keys
// fresh still carries and appending the ones it adds. A key present in both
// keeps its key node — and therefore the comment written above it.
func mergeMapping(base, fresh, baseline *yaml.Node) {
	freshAt := make(map[string]int, len(fresh.Content)/2)
	for i := 0; i+1 < len(fresh.Content); i += 2 {
		freshAt[fresh.Content[i].Value] = i
	}
	baselineAt := make(map[string]int)
	if baseline != nil && baseline.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(baseline.Content); i += 2 {
			baselineAt[baseline.Content[i].Value] = i
		}
	}

	content := make([]*yaml.Node, 0, len(fresh.Content))
	kept := make(map[string]bool, len(freshAt))
	for i := 0; i+1 < len(base.Content); i += 2 {
		key := base.Content[i].Value
		j, ok := freshAt[key]
		if !ok {
			// The value no longer carries this key. An `omitempty` field the
			// author wrote out at its zero (`replacement: ""`) is not a removal
			// though — the marshal simply does not emit it — so the line stays,
			// and only a key whose value actually said something is dropped.
			if isZeroNode(base.Content[i+1]) {
				content = append(content, base.Content[i], base.Content[i+1])
			}
			continue
		}
		kept[key] = true
		var was *yaml.Node
		if b, ok := baselineAt[key]; ok {
			was = baseline.Content[b+1]
		}
		merge(base.Content[i+1], fresh.Content[j+1], was)
		content = append(content, base.Content[i], base.Content[i+1])
	}
	for i := 0; i+1 < len(fresh.Content); i += 2 {
		if kept[fresh.Content[i].Value] {
			continue
		}
		// A key the document omits and the value fills with its type's zero is
		// not an addition: the omission already said that. Writing it in would
		// stamp `replacement: ""` over a rule the author wrote one line for, and
		// every field without omitempty would grow into every governance file the
		// first time anything was applied to it.
		if isZeroNode(fresh.Content[i+1]) {
			continue
		}
		content = append(content, fresh.Content[i], fresh.Content[i+1])
	}
	base.Content = content
}

// isZeroNode reports a node carrying its type's zero: an empty or null scalar,
// false, 0, an empty collection — or a mapping whose every value is itself zero,
// which is how a struct without omitempty marshals a field nobody has set. A
// non-empty sequence is never zero: a list with items in it is a list someone
// wrote.
func isZeroNode(n *yaml.Node) bool {
	switch n.Kind {
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!null":
			return true
		case "!!bool":
			return n.Value == "false"
		case "!!int", "!!float":
			return n.Value == "0"
		default:
			return n.Value == ""
		}
	case yaml.MappingNode:
		for i := 1; i < len(n.Content); i += 2 {
			if !isZeroNode(n.Content[i]) {
				return false
			}
		}
		return true
	case yaml.SequenceNode:
		return len(n.Content) == 0
	default:
		return false
	}
}

// mergeSequence merges item by item as far as both go, then takes the fresh
// node's remainder. Sequence items have no key to match on, so position is the
// only correspondence available: an appended rule leaves the items above it —
// and their comments — exactly as they were, which is the shape nearly every
// applied change takes.
func mergeSequence(base, fresh, baseline *yaml.Node) {
	n := min(len(base.Content), len(fresh.Content))
	content := make([]*yaml.Node, 0, len(fresh.Content))
	for i := range n {
		merge(base.Content[i], fresh.Content[i], childAt(baseline, i))
		content = append(content, base.Content[i])
	}
	content = append(content, fresh.Content[n:]...)
	base.Content = content
}

// childAt is the i-th child of a node that may be nil or shorter than the
// position asked for — the shape a baseline takes wherever the document and the
// value have drifted apart.
func childAt(n *yaml.Node, i int) *yaml.Node {
	if n == nil || i >= len(n.Content) {
		return nil
	}
	return n.Content[i]
}

// sameData reports whether two nodes carry the same data, ignoring how either
// says it. Both are decoded to plain Go values and re-marshalled canonically, so
// a scalar and the mapping a type expands it into compare as what they mean.
func sameData(a, b *yaml.Node) bool {
	if a == nil || b == nil {
		return false
	}
	ca, ok := canonicalNode(a)
	if !ok {
		return false
	}
	cb, ok := canonicalNode(b)
	return ok && bytes.Equal(ca, cb)
}

func canonicalNode(n *yaml.Node) ([]byte, bool) {
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, false
	}
	return canonicalValue(v)
}

// carryComments moves the document's commentary onto a node taken from the
// fresh marshal, so a value whose shape changed keeps the prose written about
// it. A struct marshal produces no comments of its own, so there is nothing to
// overwrite.
func carryComments(dst, src *yaml.Node) {
	if src.HeadComment != "" {
		dst.HeadComment = src.HeadComment
	}
	if src.LineComment != "" {
		dst.LineComment = src.LineComment
	}
	if src.FootComment != "" {
		dst.FootComment = src.FootComment
	}
}

// restoreSpacing puts the document's blank lines back into the re-emitted text.
// yaml.v3 keeps a blank line only inside a comment block; every other one — the
// gap that separates one collection from the next, the space above a section —
// is dropped by the parser and never reaches the emitter. That spacing is
// authored punctuation, so it is measured on both sides and restored.
//
// merged is the tree that was emitted and authored maps its nodes to the lines
// they stood on in the original, which is the correspondence between the two
// texts: for each node that survived, the run of blank and comment lines above
// it is compared with the run above the line it now occupies, and the run is
// rewritten to the authored one whenever the two agree on which comments stand
// there. A comment the emitter moved or the merge dropped leaves its region
// alone rather than guessing.
func restoreSpacing(original, rendered []byte, merged *yaml.Node, authored map[*yaml.Node]int) []byte {
	var doc yaml.Node
	if err := yaml.Unmarshal(rendered, &doc); err != nil {
		return rendered
	}
	pairs := make(map[int]int)
	pairLines(merged, &doc, authored, pairs)

	authoredLines := strings.Split(string(original), "\n")
	renderedLines := strings.Split(string(rendered), "\n")

	splices := make(map[int]gap, len(pairs))
	for renderedLine, authoredLine := range pairs {
		now, ok := gapAbove(renderedLines, renderedLine)
		if !ok {
			continue
		}
		then, ok := gapAbove(authoredLines, authoredLine)
		if !ok {
			continue
		}
		want, ok := reflowGap(then.lines, now.lines)
		if !ok || slices.Equal(want, now.lines) {
			continue
		}
		now.lines = want
		splices[now.start] = now
	}

	out := make([]string, 0, len(renderedLines)+len(splices))
	for i := 0; i < len(renderedLines); {
		if s, ok := splices[i]; ok {
			delete(splices, i)
			out = append(out, s.lines...)
			i = s.end
			continue
		}
		out = append(out, renderedLines[i])
		i++
	}
	restored := setTrailingBlanks(strings.Join(out, "\n"), trailingBlanks(authoredLines))

	// The spacing pass edits text the emitter produced, so it answers to the
	// emitter: a document it changed the meaning of is not spacing.
	if !sameContent(rendered, []byte(restored)) {
		return rendered
	}
	return []byte(restored)
}

// gap is a run of blank and comment lines standing above a node, as a half-open
// range of line indices.
type gap struct {
	start, end int
	lines      []string
}

// gapAbove reads the run of blank and comment lines immediately above a 1-based
// line number.
func gapAbove(lines []string, line int) (gap, bool) {
	end := line - 1
	if end < 0 || end > len(lines) {
		return gap{}, false
	}
	start := end
	for start > 0 && isSpacingLine(lines[start-1]) {
		start--
	}
	return gap{start: start, end: end, lines: lines[start:end]}, true
}

func isSpacingLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

// reflowGap lays the authored run's blank lines around the comment lines as they
// were emitted, so a restored gap carries the emitter's indentation and the
// author's spacing. It reports false unless both runs carry the same comments in
// the same order, which is the only case where each blank line has an
// unambiguous place to go.
func reflowGap(authored, emitted []string) ([]string, bool) {
	comments := make([]string, 0, len(emitted))
	for _, line := range emitted {
		if strings.TrimSpace(line) != "" {
			comments = append(comments, line)
		}
	}
	out := make([]string, 0, len(authored))
	next := 0
	for _, line := range authored {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		if next >= len(comments) || strings.TrimSpace(comments[next]) != strings.TrimSpace(line) {
			return nil, false
		}
		out = append(out, comments[next])
		next++
	}
	if next != len(comments) {
		return nil, false
	}
	return out, true
}

// pairLines walks the emitted tree alongside the tree it was emitted from,
// mapping each rendered line back to the line the same node stood on in the
// original. A node the merge added is not in authored and contributes nothing; a
// branch whose shape the two trees disagree on is abandoned rather than paired
// off by position.
func pairLines(merged, rendered *yaml.Node, authored map[*yaml.Node]int, into map[int]int) {
	if merged == nil || rendered == nil {
		return
	}
	if merged.Kind != rendered.Kind || len(merged.Content) != len(rendered.Content) {
		return
	}
	if line, ok := authored[merged]; ok && line > 0 && rendered.Line > 0 {
		if _, seen := into[rendered.Line]; !seen {
			into[rendered.Line] = line
		}
	}
	for i := range merged.Content {
		pairLines(merged.Content[i], rendered.Content[i], authored, into)
	}
}

// lineIndex records where every node stood before the merge moved its data
// around, which is what makes the spacing pass possible: a merged node keeps its
// identity even when its value was replaced outright.
func lineIndex(node *yaml.Node) map[*yaml.Node]int {
	index := make(map[*yaml.Node]int)
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		index[n] = n.Line
		for _, child := range n.Content {
			walk(child)
		}
	}
	walk(node)
	return index
}

// trailingBlanks counts the blank lines a document ends on, discounting the
// empty element that splitting on the final newline produces.
func trailingBlanks(lines []string) int {
	n := 0
	for i := len(lines) - 1; i >= 0 && strings.TrimSpace(lines[i]) == ""; i-- {
		n++
	}
	if n > 0 {
		n--
	}
	return n
}

func setTrailingBlanks(text string, want int) string {
	return strings.TrimRight(text, "\n") + strings.Repeat("\n", want+1)
}

// detectIndent reads the nesting width from the document itself, so a file
// written with two spaces is not silently reflowed to the encoder's four. The
// first line indented under a mapping key decides it; a document with no nested
// block has none to copy and takes DefaultIndent.
func detectIndent(doc []byte) int {
	for line := range strings.SplitSeq(string(doc), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if n := len(line) - len(trimmed); n > 0 {
			return n
		}
	}
	return DefaultIndent
}
