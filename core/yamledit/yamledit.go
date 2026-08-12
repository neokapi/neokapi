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
//     supplies the comments, the key order and each scalar's style (quoting,
//     block scalars). A key the value no longer carries is removed, and one it
//     adds is appended — an edit, not a re-issue.
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
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultIndent is the nesting width used for a document that has none to copy
// (a new file, or one with no nested block).
const DefaultIndent = 2

// Marshal renders value as YAML, carrying over the comments, key order, scalar
// styles and indentation of original. An empty original is a plain marshal at
// DefaultIndent.
func Marshal(original []byte, value any) ([]byte, error) {
	fresh, err := encodeNode(value)
	if err != nil {
		return nil, err
	}

	indent := DefaultIndent
	var base yaml.Node
	if len(bytes.TrimSpace(original)) > 0 {
		if err := yaml.Unmarshal(original, &base); err == nil && base.Kind == yaml.DocumentNode {
			merge(&base, fresh)
			fresh = &base
			indent = detectIndent(original)
		}
		// A document that will not parse carries nothing to preserve: the fresh
		// marshal stands, which is what the caller would have written anyway.
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(indent)
	if err := enc.Encode(fresh); err != nil {
		return nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close yaml encoder: %w", err)
	}

	// A document that already says what the value says is returned untouched,
	// whatever the emitter would have made of it. Re-emitting is not lossless in
	// the small — a folded scalar is re-wrapped at the emitter's width, not the
	// author's — and byte-stability has to survive presentation the writer cannot
	// reproduce, or the first write after any such file churns for no decision.
	if len(original) > 0 && sameContent(original, buf.Bytes()) {
		return original, nil
	}
	return buf.Bytes(), nil
}

// sameContent reports whether two documents carry the same data, ignoring
// comments, key order and every presentation choice. Both are decoded to plain
// Go values and re-marshalled canonically, so the comparison is of what the
// documents say, not of how they say it.
func sameContent(a, b []byte) bool {
	canonical := func(doc []byte) ([]byte, bool) {
		var v any
		if err := yaml.Unmarshal(doc, &v); err != nil {
			return nil, false
		}
		out, err := yaml.Marshal(v)
		if err != nil {
			return nil, false
		}
		return out, true
	}
	ca, ok := canonical(a)
	if !ok {
		return false
	}
	cb, ok := canonical(b)
	return ok && bytes.Equal(ca, cb)
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
//
// Nodes of different kinds do not merge — the value's shape wins outright, since
// a mapping that became a list is a different answer, not an edited one.
func merge(base, fresh *yaml.Node) {
	if base == nil || fresh == nil || base.Kind != fresh.Kind {
		if base != nil && fresh != nil {
			carryComments(fresh, base)
			*base = *fresh
		}
		return
	}
	switch base.Kind {
	case yaml.DocumentNode:
		if len(base.Content) == 1 && len(fresh.Content) == 1 {
			merge(base.Content[0], fresh.Content[0])
			return
		}
		base.Content = fresh.Content
	case yaml.MappingNode:
		mergeMapping(base, fresh)
	case yaml.SequenceNode:
		mergeSequence(base, fresh)
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
func mergeMapping(base, fresh *yaml.Node) {
	freshAt := make(map[string]int, len(fresh.Content)/2)
	for i := 0; i+1 < len(fresh.Content); i += 2 {
		freshAt[fresh.Content[i].Value] = i
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
		merge(base.Content[i+1], fresh.Content[j+1])
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
func mergeSequence(base, fresh *yaml.Node) {
	n := min(len(base.Content), len(fresh.Content))
	content := make([]*yaml.Node, 0, len(fresh.Content))
	for i := range n {
		merge(base.Content[i], fresh.Content[i])
		content = append(content, base.Content[i])
	}
	content = append(content, fresh.Content[n:]...)
	base.Content = content
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
