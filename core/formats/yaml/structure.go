package yaml

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	yamlv3 "gopkg.in/yaml.v3"
)

// A YAML block is a scalar value of a block mapping, named by its key path,
// and its shell is the line that holds the key and the value (with the lines
// a multi-line value continues on). The writer adds a line beside another key
// of the same mapping, at its indentation, and removes a key's lines. Comment
// lines are never added or removed: a comment above a key stays where it is,
// and only a comment on the removed key's own line goes with it.
var _ format.StructureEditor = (*Writer)(nil)

// Structural declares insert_block and delete_block.
func (w *Writer) Structural() []string {
	return []string{format.StructuralDeleteBlock, format.StructuralInsertBlock}
}

// EditStructure adds and removes the keys edits name, in order.
func (w *Writer) EditStructure(doc []byte, edits []format.StructuralEdit) ([]byte, error) {
	for i, e := range edits {
		bom, content := format.SplitBOM(doc)
		var out []byte
		var err error
		switch e.Op {
		case format.StructuralDeleteBlock:
			out, err = yamlDelete(content, i, keyPath(e.Block, e.Key))
		case format.StructuralInsertBlock:
			out, err = yamlInsert(content, i, e)
		default:
			err = format.StructureErrorf(i, format.StructureUnsupported, "YAML has no structural operation %q", e.Op)
		}
		if err != nil {
			return nil, err
		}
		doc = append(append([]byte{}, bom...), out...)
	}
	return doc, nil
}

// keyPath is the key path a block is found by: its name, else key.
func keyPath(b *model.Block, key string) string {
	if b != nil && b.Name != "" {
		return b.Name
	}
	return key
}

// yamlEntry is one key of a mapping and its value.
type yamlEntry struct {
	key, val *yamlv3.Node
	// mapping is the mapping the key is in, and owner the entry whose value
	// that mapping is; nil for a document's top-level mapping.
	mapping *yamlv3.Node
	owner   *yamlEntry
	// path is the key path, as the reader names blocks; parent is the path
	// of the mapping.
	path, parent string
	// viaAlias says the entry is reached through an alias, so its bytes are
	// somewhere else.
	viaAlias bool
}

// yamlDoc is a YAML stream read into its keys.
type yamlDoc struct {
	content []byte
	lines   []int
	eol     string
	roots   []*yamlv3.Node
	entries []*yamlEntry
}

func readYAML(content []byte, i int) (*yamlDoc, error) {
	d := &yamlDoc{content: content, lines: buildLineOffsets(content), eol: detectDominantEOL(content)}
	dec := yamlv3.NewDecoder(bytes.NewReader(content))
	for {
		var n yamlv3.Node
		if err := dec.Decode(&n); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, format.StructureErrorf(i, format.StructureUnsupported, "the document does not read as YAML: %v", err)
		}
		root := &n
		if n.Kind == yamlv3.DocumentNode && len(n.Content) > 0 {
			root = n.Content[0]
		}
		d.roots = append(d.roots, root)
		d.walk(root, nil, "", false, map[*yamlv3.Node]bool{})
	}
	return d, nil
}

func (d *yamlDoc) walk(n *yamlv3.Node, owner *yamlEntry, path string, viaAlias bool, visiting map[*yamlv3.Node]bool) {
	switch n.Kind {
	case yamlv3.MappingNode:
		for j := 0; j+1 < len(n.Content); j += 2 {
			k, v := n.Content[j], n.Content[j+1]
			e := &yamlEntry{key: k, val: v, mapping: n, owner: owner, path: joinPath(path, k.Value), parent: path, viaAlias: viaAlias}
			d.entries = append(d.entries, e)
			d.walk(v, e, e.path, viaAlias, visiting)
		}
	case yamlv3.SequenceNode:
		for j, c := range n.Content {
			d.walk(c, nil, path+"["+strconv.Itoa(j)+"]", viaAlias, visiting)
		}
	case yamlv3.AliasNode:
		if n.Alias == nil || visiting[n.Alias] {
			return
		}
		visiting[n.Alias] = true
		d.walk(n.Alias, owner, path, true, visiting)
		delete(visiting, n.Alias)
	}
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// find returns the one entry at path whose bytes are its own.
func (d *yamlDoc) find(i int, path string) (*yamlEntry, error) {
	var found []*yamlEntry
	for _, e := range d.entries {
		if e.path == path {
			found = append(found, e)
		}
	}
	switch {
	case len(found) > 1:
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "two keys of the document have the key path %s", path)
	case len(found) == 0 && strings.HasSuffix(path, "]"):
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s is an item of a sequence, which is named by its position; a block is added or removed only as a key of a mapping", path)
	case len(found) == 0:
		return nil, format.StructureErrorf(i, format.StructureNotFound, "the document has no key at %s", path)
	}
	e := found[0]
	switch {
	case e.viaAlias:
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s is reached through an alias, so its key is written elsewhere", path)
	case e.mapping.Style&yamlv3.FlowStyle != 0:
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s is in a flow mapping ({…}), where keys are not lines of their own", path)
	case e.val.Kind != yamlv3.ScalarNode:
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s holds no text value, so no block is there", path)
	}
	if _, ok := d.ownLine(e.key); !ok {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "the key %s shares its line with something before it, such as a sequence item's dash", path)
	}
	return e, nil
}

// offset is the byte offset of a node's line and column.
func (d *yamlDoc) offset(n *yamlv3.Node) int { return lineColToOffset(d.lines, n.Line, n.Column) }

// ownLine returns the offset of the line a key starts, when nothing but
// indentation precedes the key on it.
func (d *yamlDoc) ownLine(key *yamlv3.Node) (int, bool) {
	if key.Line < 1 || key.Line > len(d.lines) {
		return 0, false
	}
	start := d.lines[key.Line-1]
	at := d.offset(key)
	if at < start || at > len(d.content) {
		return 0, false
	}
	return start, strings.Trim(string(d.content[start:at]), " ") == ""
}

// valueEnd is the offset just past the line where e's value ends, line break
// included.
func (d *yamlDoc) valueEnd(e *yamlEntry) int {
	start := d.offset(e.val)
	start += scanTagPrefix(d.content, start)
	if e.val.Anchor != "" && start < len(d.content) && d.content[start] == '&' {
		for start < len(d.content) && d.content[start] != ' ' && d.content[start] != '\n' {
			start++
		}
		for start < len(d.content) && d.content[start] == ' ' {
			start++
		}
	}
	end := scanScalarEnd(d.content, start, e.val.Style, e.val.Value)
	if end > 0 && end <= len(d.content) && d.content[end-1] == '\n' {
		return end
	}
	if nl := bytes.IndexByte(d.content[end:], '\n'); nl >= 0 {
		return end + nl + 1
	}
	return len(d.content)
}

// yamlDelete removes the key at path and the lines of its value.
func yamlDelete(content []byte, i int, path string) ([]byte, error) {
	d, err := readYAML(content, i)
	if err != nil {
		return nil, err
	}
	e, err := d.find(i, path)
	if err != nil {
		return nil, err
	}
	if e.val.Anchor != "" {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "the value of %s is an anchor (&%s) other keys may alias", path, e.val.Anchor)
	}
	start, _ := d.ownLine(e.key)
	end := d.valueEnd(e)
	out := splice(content, start, end, "")
	if len(e.mapping.Content) == 2 && e.owner != nil {
		// The mapping loses its last key. An empty value would read as
		// null, so the key that held the mapping is given an empty one.
		at, ok := d.afterColon(e.owner.key)
		if !ok {
			return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s is the last key under %s, and the line of %s has no place for an empty mapping", path, e.parent, e.parent)
		}
		out = splice(out, at, at, " {}")
	}
	return out, nil
}

// afterColon is the offset just past the colon after a block mapping's key.
func (d *yamlDoc) afterColon(key *yamlv3.Node) (int, bool) {
	at := d.offset(key)
	if at < 0 || at >= len(d.content) {
		return 0, false
	}
	switch key.Style {
	case yamlv3.DoubleQuotedStyle:
		at = scanQuotedEnd(d.content, at, '"')
	case yamlv3.SingleQuotedStyle:
		at = scanQuotedEnd(d.content, at, '\'')
	case 0:
		at += len(key.Value)
	default:
		return 0, false
	}
	for at < len(d.content) && d.content[at] == ' ' {
		at++
	}
	if at >= len(d.content) || d.content[at] != ':' {
		return 0, false
	}
	return at + 1, true
}

// yamlInsert adds e's key beside its anchor, or last in the top-level
// mapping.
func yamlInsert(content []byte, i int, e format.StructuralEdit) ([]byte, error) {
	d, err := readYAML(content, i)
	if err != nil {
		return nil, err
	}
	for _, x := range d.entries {
		if x.path == e.Key {
			return nil, format.StructureErrorf(i, format.StructureExists, "the document already has a key at %s", e.Key)
		}
	}
	value := newScalar(e.Value)
	var out []byte
	if e.Anchor == "" {
		if out, err = d.append(i, e.Key, value); err != nil {
			return nil, err
		}
	} else {
		anchor, ferr := d.find(i, keyPath(e.AnchorBlock, e.Anchor))
		if ferr != nil {
			return nil, ferr
		}
		key := e.Key
		if p := anchor.parent; p != "" {
			rest, ok := strings.CutPrefix(e.Key, p+".")
			if !ok || rest == "" {
				return nil, format.StructureErrorf(i, format.StructureUnsupported,
					"%s goes in the mapping at %s beside %s, so its key path starts with %s.", e.Key, p, anchor.path, p)
			}
			key = rest
		}
		for j := 0; j+1 < len(anchor.mapping.Content); j += 2 {
			if anchor.mapping.Content[j].Value == key {
				return nil, format.StructureErrorf(i, format.StructureExists, "the mapping at %s already has a key %q", anchor.parent, key)
			}
		}
		lineStart, _ := d.ownLine(anchor.key)
		indent := string(content[lineStart:d.offset(anchor.key)])
		line := indent + newScalar(key) + ": " + value
		if e.Before {
			out = splice(content, lineStart, lineStart, line+d.eol)
		} else {
			at := d.valueEnd(anchor)
			if at == len(content) && (at == 0 || content[at-1] != '\n') {
				out = splice(content, at, at, d.eol+line)
			} else {
				out = splice(content, at, at, line+d.eol)
			}
		}
	}
	return out, checkInserted(out, i, e)
}

// append adds a key last in the document's one top-level mapping.
func (d *yamlDoc) append(i int, key, value string) ([]byte, error) {
	var root *yamlv3.Node
	switch len(d.roots) {
	case 0:
	case 1:
		root = d.roots[0]
	default:
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "the file holds %d documents; name the key the new one goes beside", len(d.roots))
	}
	indent := ""
	if root != nil && root.Kind != yamlv3.ScalarNode {
		if root.Kind != yamlv3.MappingNode || root.Style&yamlv3.FlowStyle != 0 {
			return nil, format.StructureErrorf(i, format.StructureUnsupported, "the document's top level is not a block mapping, so it has no place for %s", key)
		}
		if len(root.Content) > 0 {
			indent = strings.Repeat(" ", max(root.Content[0].Column-1, 0))
		}
	} else if root != nil && root.Value != "" {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "the document is a single value, so it has no place for %s", key)
	}
	line := indent + newScalar(key) + ": " + value + d.eol
	content := d.content
	if len(content) > 0 && content[len(content)-1] != '\n' {
		line = d.eol + line
	}
	return splice(content, len(content), len(content), line), nil
}

// checkInserted reads the result back: the new key must be there, holding
// the value as given.
func checkInserted(out []byte, i int, e format.StructuralEdit) error {
	d, err := readYAML(out, i)
	if err != nil {
		return format.StructureErrorf(i, format.StructureUnsupported, "the document would not read as YAML after adding %s: %v", e.Key, err)
	}
	x, err := d.find(i, e.Key)
	if err != nil {
		return err
	}
	if x.val.Value != e.Value || x.val.ShortTag() != "!!str" {
		return format.StructureErrorf(i, format.StructureUnsupported, "%s would not read back as the text given", e.Key)
	}
	return nil
}

// newScalar spells s as a scalar that reads back as the string s: plain when
// that is safe, double-quoted otherwise.
func newScalar(s string) string {
	if needsDoubleQuoting(s) || !plainScalarSafe(s) || !plainIsString(s) {
		return encodeDoubleQuoted(s)
	}
	return s
}

// plainIsString reports whether s written plain reads as a string, not as a
// number, a boolean or null.
func plainIsString(s string) bool {
	var n yamlv3.Node
	if err := yamlv3.Unmarshal([]byte("k: "+s), &n); err != nil {
		return false
	}
	if len(n.Content) == 0 || len(n.Content[0].Content) < 2 {
		return false
	}
	v := n.Content[0].Content[1]
	return v.Kind == yamlv3.ScalarNode && v.ShortTag() == "!!str" && v.Value == s
}

// splice returns content with [from, to) replaced by text.
func splice(content []byte, from, to int, text string) []byte {
	out := make([]byte, 0, len(content)-(to-from)+len(text))
	out = append(out, content[:from]...)
	out = append(out, text...)
	return append(out, content[to:]...)
}
