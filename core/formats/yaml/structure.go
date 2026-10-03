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

// A YAML block is a scalar value of a block mapping, named by its key path.
// Its shell is the line that holds the key and the value, the lines a value
// of several lines continues on, and the comment lines directly above the key,
// which the reader reads as the block's note. The writer adds a key to the
// mapping the new key path names, at the indentation of its keys, beside
// another key or last, and builds the mappings the path names that the
// document lacks. It removes a key's lines with the comment above it. Every
// other comment stays where it is.
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

// yamlMapping is one mapping of a document.
type yamlMapping struct {
	node *yamlv3.Node
	// path is the mapping's key path, as the reader names the blocks in it;
	// owner is the entry whose value it is, nil for a document's top level
	// (root) or an item of a sequence.
	path  string
	owner *yamlEntry
	root  bool
	// viaAlias says the mapping is reached through an alias, so its bytes are
	// somewhere else; anchored that it, or a node around it, holds an anchor
	// that an alias may repeat elsewhere.
	viaAlias, anchored bool
	entries            []*yamlEntry
}

// yamlEntry is one key of a mapping and its value.
type yamlEntry struct {
	key, val *yamlv3.Node
	mapping  *yamlMapping
	// path is the key path, as the reader names blocks.
	path string
}

// yamlDoc is a YAML stream read into its mappings and keys.
type yamlDoc struct {
	content  []byte
	lines    []int
	eol      string
	roots    []*yamlv3.Node
	mappings []*yamlMapping
	entries  []*yamlEntry
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
		d.walk(root, nil, "", true, false, false, map[*yamlv3.Node]bool{})
	}
	return d, nil
}

// walk records the mappings and keys under n, named as the reader names them:
// keys joined with dots, an item of a sequence as [i].
func (d *yamlDoc) walk(n *yamlv3.Node, owner *yamlEntry, path string, root, viaAlias, anchored bool, visiting map[*yamlv3.Node]bool) {
	anchored = anchored || n.Anchor != ""
	switch n.Kind {
	case yamlv3.MappingNode:
		m := &yamlMapping{node: n, path: path, owner: owner, root: root, viaAlias: viaAlias, anchored: anchored}
		d.mappings = append(d.mappings, m)
		for j := 0; j+1 < len(n.Content); j += 2 {
			k, v := n.Content[j], n.Content[j+1]
			e := &yamlEntry{key: k, val: v, mapping: m, path: joinPath(path, k.Value)}
			m.entries = append(m.entries, e)
			d.entries = append(d.entries, e)
			d.walk(v, e, e.path, false, viaAlias, anchored, visiting)
		}
	case yamlv3.SequenceNode:
		for j, c := range n.Content {
			d.walk(c, nil, joinPath(path, "["+strconv.Itoa(j)+"]"), false, viaAlias, anchored, visiting)
		}
	case yamlv3.AliasNode:
		if n.Alias == nil || visiting[n.Alias] {
			return
		}
		visiting[n.Alias] = true
		d.walk(n.Alias, owner, path, false, true, anchored, visiting)
		delete(visiting, n.Alias)
	}
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// editable refuses an edit inside m when its bytes are not m's own lines.
func (m *yamlMapping) editable(i int, path string) error {
	switch {
	case m.viaAlias:
		return format.StructureErrorf(i, format.StructureUnsupported, "%s is reached through an alias, so its key is written elsewhere", path)
	case m.anchored:
		return format.StructureErrorf(i, format.StructureUnsupported, "%s is inside a node with an anchor (&), which an alias may repeat elsewhere in the document", path)
	case m.node.Style&yamlv3.FlowStyle != 0:
		return format.StructureErrorf(i, format.StructureUnsupported, "%s is in a flow mapping ({…}), where keys are not lines of their own", path)
	}
	return nil
}

// dottedKeys reports whether a key of m holds a dot: the mapping names some
// values by flat key paths ("nav.home") rather than by mappings inside it.
func (m *yamlMapping) dottedKeys() bool {
	for _, e := range m.entries {
		if strings.Contains(e.key.Value, ".") {
			return true
		}
	}
	return false
}

// find returns the one entry at path whose bytes are its own, holding text.
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
	if err := e.mapping.editable(i, path); err != nil {
		return nil, err
	}
	if e.val.Kind != yamlv3.ScalarNode {
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

// lineText is line l (1-based) without its line break.
func (d *yamlDoc) lineText(l int) string {
	start := d.lines[l-1]
	end := len(d.content)
	if l < len(d.lines) {
		end = d.lines[l]
	}
	return strings.TrimRight(string(d.content[start:end]), "\r\n")
}

// headStart is the offset of the first line of the comment directly above a
// key (yaml.v3's head comment, which the reader reads as the note of the
// block the key holds), or of the key's own line when it has none.
func (d *yamlDoc) headStart(key *yamlv3.Node) int {
	start, _ := d.ownLine(key)
	if key.HeadComment == "" {
		return start
	}
	first := key.Line - (strings.Count(key.HeadComment, "\n") + 1)
	if first < 1 || !strings.HasPrefix(strings.TrimSpace(d.lineText(first)), "#") {
		return start
	}
	for l := first; l < key.Line; l++ {
		if t := strings.TrimSpace(d.lineText(l)); t != "" && !strings.HasPrefix(t, "#") {
			return start
		}
	}
	return d.lines[first-1]
}

// lineEnd is the offset just past the line break of the line at holds.
func (d *yamlDoc) lineEnd(at int) int {
	if at >= len(d.content) {
		return len(d.content)
	}
	if nl := bytes.IndexByte(d.content[at:], '\n'); nl >= 0 {
		return at + nl + 1
	}
	return len(d.content)
}

// nodeEnd is the offset just past the line where n ends, line break included.
func (d *yamlDoc) nodeEnd(n *yamlv3.Node) int {
	switch n.Kind {
	case yamlv3.MappingNode, yamlv3.SequenceNode:
		if n.Style&yamlv3.FlowStyle == 0 && len(n.Content) > 0 {
			return d.nodeEnd(n.Content[len(n.Content)-1])
		}
		return d.lineEnd(d.flowEnd(d.valueStart(n)))
	case yamlv3.AliasNode:
		return d.lineEnd(d.offset(n))
	}
	start := d.valueStart(n)
	end := scanScalarEnd(d.content, start, n.Style, n.Value)
	if end > 0 && end <= len(d.content) && d.content[end-1] == '\n' {
		return end
	}
	return d.lineEnd(end)
}

// valueStart is the offset of n's value, past its tag and anchor.
func (d *yamlDoc) valueStart(n *yamlv3.Node) int {
	at := d.offset(n)
	if at < 0 {
		return len(d.content)
	}
	for at < len(d.content) && (d.content[at] == '!' || d.content[at] == '&') {
		if d.content[at] == '!' {
			at += scanTagPrefix(d.content, at)
			continue
		}
		for at < len(d.content) && d.content[at] != ' ' && d.content[at] != '\n' {
			at++
		}
		for at < len(d.content) && d.content[at] == ' ' {
			at++
		}
	}
	return at
}

// flowEnd is the offset past the bracket that closes the flow collection
// opening at at, or at itself when none opens there.
func (d *yamlDoc) flowEnd(at int) int {
	c := d.content
	if at >= len(c) || (c[at] != '{' && c[at] != '[') {
		return at
	}
	depth := 0
	for i := at; i < len(c); i++ {
		switch c[i] {
		case '"', '\'':
			i = scanQuotedEnd(c, i, c[i]) - 1
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(c)
}

// yamlDelete removes the key at path, the lines of its value and the comment
// directly above it.
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
	out := splice(content, d.headStart(e.key), d.nodeEnd(e.val), "")
	if len(e.mapping.node.Content) == 2 && !e.mapping.root {
		// The mapping loses its last key. An empty value would read as
		// null, so the key that held the mapping is given an empty one.
		owner := e.mapping.owner
		if owner == nil {
			return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s is the last key of an item of a sequence, and removing it would leave the item empty", path)
		}
		at, ok := d.emptyAt(owner.key)
		if !ok {
			return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s is the last key under %s, and the line of %s has no place for an empty mapping", path, owner.path, owner.path)
		}
		out = splice(out, at, at, " {}")
	}
	if _, err := readYAML(out, i); err != nil {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "the document would not read as YAML after removing %s", path)
	}
	return out, nil
}

// emptyAt is where an empty mapping goes as the value of a block mapping's
// key: after the colon, and after the tag the value carries there.
func (d *yamlDoc) emptyAt(key *yamlv3.Node) (int, bool) {
	at, ok := d.afterColon(key)
	if !ok {
		return 0, false
	}
	for {
		j := at
		for j < len(d.content) && d.content[j] == ' ' {
			j++
		}
		if j >= len(d.content) || (d.content[j] != '!' && d.content[j] != '&') {
			return at, true
		}
		for j < len(d.content) && d.content[j] != ' ' && d.content[j] != '\t' && d.content[j] != '\n' && d.content[j] != '\r' {
			j++
		}
		at = j
	}
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

// yamlInsert adds e's key to the mapping its key path names: beside its
// anchor, or last there.
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
	var anchor *yamlEntry
	if e.Anchor != "" {
		if anchor, err = d.find(i, keyPath(e.AnchorBlock, e.Anchor)); err != nil {
			return nil, err
		}
	}
	m, keys, err := d.place(i, e.Key, anchor)
	if err != nil {
		return nil, err
	}
	text := d.keyLines(m, keys, newScalar(e.Value))
	var at int
	switch {
	case anchor != nil && e.Before:
		at = d.headStart(anchor.key)
	case anchor != nil:
		at = d.nodeEnd(anchor.val)
	case m != nil && len(m.entries) > 0:
		at = d.nodeEnd(m.entries[len(m.entries)-1].val)
	default:
		at = len(content)
	}
	if at == len(content) && at > 0 && content[at-1] != '\n' {
		text = d.eol + text
	}
	out := splice(content, at, at, text+d.eol)
	return out, checkInserted(out, i, e)
}

// place finds where a key with key path path goes: the deepest mapping of the
// document whose own key path starts path (nil for the top level of an empty
// document), and the keys that lead from it to the new value. One key is a
// key of that mapping. Several build mappings inside one another under the
// first, unless the mapping already names values by flat key paths
// ("nav.home"), where the rest of path is one key. A new key with an anchor
// goes in the anchor's mapping.
func (d *yamlDoc) place(i int, path string, anchor *yamlEntry) (*yamlMapping, []string, error) {
	var m *yamlMapping
	rest := path
	for at := strings.LastIndexByte(path, '.'); at > 0 && m == nil; at = strings.LastIndexByte(path[:at], '.') {
		var ms []*yamlMapping
		for _, x := range d.mappings {
			if x.path == path[:at] && !x.root {
				ms = append(ms, x)
			}
		}
		switch {
		case len(ms) == 1:
			m, rest = ms[0], path[at+1:]
		case len(ms) > 1 && anchor != nil:
			for _, x := range ms {
				if x == anchor.mapping {
					m, rest = x, path[at+1:]
				}
			}
			if m == nil {
				m, rest = ms[0], path[at+1:]
			}
		case len(ms) > 1:
			return nil, nil, format.StructureErrorf(i, format.StructureUnsupported, "two mappings of the document have the key path %s; name the key the new one goes beside", path[:at])
		}
	}
	if m == nil {
		top, err := d.top(i, path, anchor)
		if err != nil {
			return nil, nil, err
		}
		m = top
	}
	where := "the top level"
	if m != nil && m.path != "" {
		where = m.path
	}
	if anchor != nil && anchor.mapping != m {
		return nil, nil, format.StructureErrorf(i, format.StructureUnsupported,
			"%s goes in the mapping at %s and %s is outside it; name a key of that mapping to put it beside, or no key to put it last there", path, where, anchor.path)
	}
	if m != nil {
		if err := m.editable(i, path); err != nil {
			return nil, nil, err
		}
	}
	keys := []string{rest}
	if strings.Contains(rest, ".") && (m == nil || !m.dottedKeys()) {
		keys = strings.Split(rest, ".")
	}
	for _, k := range keys {
		if k == "" || strings.HasPrefix(k, "[") {
			return nil, nil, format.StructureErrorf(i, format.StructureUnsupported,
				"%s names an item of a sequence or an empty key; a block is added only as a key of a mapping", path)
		}
	}
	if m != nil {
		for _, x := range m.entries {
			if x.key.Value == keys[0] {
				return nil, nil, format.StructureErrorf(i, format.StructureExists, "the document already has a value at %s, with no keys inside it", x.path)
			}
		}
	}
	return m, keys, nil
}

// top is the top-level mapping a new key goes in: the anchor's document's, or
// the document's one; nil for a document that holds nothing yet.
func (d *yamlDoc) top(i int, path string, anchor *yamlEntry) (*yamlMapping, error) {
	if anchor != nil && anchor.mapping.root {
		return anchor.mapping, nil
	}
	switch len(d.roots) {
	case 0:
		return nil, nil
	case 1:
	default:
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "the file holds %d documents; name the key the new one goes beside", len(d.roots))
	}
	root := d.roots[0]
	if (root.Kind == yamlv3.ScalarNode && root.Value == "") || (root.Kind == yamlv3.DocumentNode && len(root.Content) == 0) {
		return nil, nil
	}
	for _, x := range d.mappings {
		if x.node == root {
			return x, nil
		}
	}
	return nil, format.StructureErrorf(i, format.StructureUnsupported, "the document's top level is not a mapping, so it has no place for %s", path)
}

// keyLines spells the lines of new keys in m, one inside the other, the last
// holding value: the first at the indentation of m's keys, each further one
// step deeper. The text has no line break at its end.
func (d *yamlDoc) keyLines(m *yamlMapping, keys []string, value string) string {
	indent, step := d.indentation(m)
	var b strings.Builder
	for j, k := range keys {
		if j > 0 {
			b.WriteString(d.eol)
		}
		b.WriteString(indent + strings.Repeat(step, j) + newScalar(k) + ":")
		if j == len(keys)-1 {
			b.WriteString(" " + value)
		}
	}
	return b.String()
}

// indentation is the indentation of m's keys and the step a mapping inside
// another adds, read from the document: two spaces where it shows none.
func (d *yamlDoc) indentation(m *yamlMapping) (indent, step string) {
	step = "  "
	for _, x := range d.mappings {
		if x.owner != nil && len(x.entries) > 0 && !x.viaAlias {
			if n := x.entries[0].key.Column - x.owner.key.Column; n > 0 {
				step = strings.Repeat(" ", n)
				break
			}
		}
	}
	if m != nil && len(m.entries) > 0 {
		indent = strings.Repeat(" ", max(m.entries[0].key.Column-1, 0))
	}
	return indent, step
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
// that is safe under YAML 1.2 and YAML 1.1 alike, double-quoted otherwise.
func newScalar(s string) string {
	if needsDoubleQuoting(s) || !plainScalarSafe(s) || !plainIsString(s) || yaml11NonString(s) {
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

// yaml11NonString reports whether a YAML 1.1 parser (Ruby's Psych, PyYAML)
// may read s written plain as something other than a string: a boolean such
// as yes, no, on or off in any case, null, or a number, which YAML 1.1 also
// spells with underscores, colons (sexagesimal) and a leading dot.
func yaml11NonString(s string) bool {
	switch strings.ToLower(s) {
	case "y", "n", "yes", "no", "on", "off", "true", "false", "null", "~":
		return true
	}
	t := strings.TrimLeft(s, "+-")
	if t == "" {
		return false
	}
	t = strings.TrimPrefix(t, ".")
	return t != "" && (t[0] >= '0' && t[0] <= '9' || strings.HasPrefix(strings.ToLower(t), "inf") || strings.HasPrefix(strings.ToLower(t), "nan"))
}

// splice returns content with [from, to) replaced by text.
func splice(content []byte, from, to int, text string) []byte {
	out := make([]byte, 0, len(content)-(to-from)+len(text))
	out = append(out, content[:from]...)
	out = append(out, text...)
	return append(out, content[to:]...)
}
