package json

import (
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/internal/jsonobject"
	"github.com/neokapi/neokapi/core/model"
)

// A JSON block is a string value in an object, named by its key path, and its
// shell is the member that holds it: the key, the colon and the value. The
// writer adds a member to the object the new key path names, beside another
// member or last, building the objects the path names that the document lacks,
// and removes one with the comma that separated it, keeping the document's
// indentation and every comment but one on the removed member's line.
var _ format.StructureEditor = (*Writer)(nil)

// Structural declares insert_block and delete_block, under a configuration
// that reads each block from its own member.
func (w *Writer) Structural() []string {
	if !w.structural() {
		return nil
	}
	return []string{format.StructuralDeleteBlock, format.StructuralInsertBlock}
}

// EditStructure adds and removes the members edits name, in order. Each new
// member is checked once all edits are made: the document must read with a
// text value at its key path.
func (w *Writer) EditStructure(doc []byte, edits []format.StructuralEdit) ([]byte, error) {
	if !w.structural() {
		return nil, format.StructureErrorf(0, format.StructureUnsupported,
			"this JSON configuration reads notes, ids or metadata from the members beside a block, so a block's shell is more than its member")
	}
	inserted := map[string]int{}
	for i, e := range edits {
		d, err := parseObjects(doc)
		if err != nil {
			return nil, format.StructureErrorf(i, format.StructureUnsupported, "the document does not read as JSON: %v", err)
		}
		switch e.Op {
		case format.StructuralDeleteBlock:
			m, serr := findMember(d, i, keyPath(e.Block, w.keyPathOf(e.Key)))
			if serr != nil {
				return nil, serr
			}
			if !d.IsString(m) {
				return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s holds no text value, so no block is there to remove", m.Path)
			}
			doc = d.Delete(m)
			delete(inserted, m.Path)
		case format.StructuralInsertBlock:
			if doc, err = w.insert(d, i, e); err != nil {
				return nil, err
			}
			inserted[w.keyPathOf(e.Key)] = i
		default:
			return nil, format.StructureErrorf(i, format.StructureUnsupported, "JSON has no structural operation %q", e.Op)
		}
	}
	if len(inserted) > 0 {
		if err := checkInserted(doc, inserted); err != nil {
			return nil, err
		}
	}
	return doc, nil
}

// structural reports whether the writer's configuration lets a block's
// member stand alone: a configuration that reads a note, an id or metadata
// from the members beside a block makes those members part of the block's
// shell, and the writer adds and removes no blocks under it.
func (w *Writer) structural() bool {
	c := w.cfg
	return c == nil || (c.NoteRules == "" && c.IDRules == "" && !c.UseIDStack && c.GenericMetaRules == "" && c.MaxwidthRules == "")
}

// keyPathOf is the key path of the block a reader names name: the name itself,
// or, where the configuration names blocks by full key path (/nav/home), the
// dotted path that name spells.
func (w *Writer) keyPathOf(name string) string {
	if w.cfg == nil || !w.cfg.UseKeyAsName || !w.cfg.UseFullKeyPath {
		return name
	}
	return strings.ReplaceAll(strings.TrimPrefix(name, "/"), "/", ".")
}

// insert adds e's member to d: in the deepest object the start of its key
// path names, beside its anchor or last there.
func (w *Writer) insert(d *jsonobject.Doc, i int, e format.StructuralEdit) ([]byte, error) {
	path := w.keyPathOf(e.Key)
	value := escapeJSONStringQuoted(e.Value, w.cfg.EscapeForwardSlashes, '"')
	if len(d.Find(path)) > 0 {
		return nil, format.StructureErrorf(i, format.StructureExists, "the document already has a value at %s", path)
	}
	o, keys, err := place(d, i, path)
	if err != nil {
		return nil, err
	}
	keyRaw := escapeJSONString(keys[0], false)
	if len(keys) > 1 {
		inner := make([]string, 0, len(keys)-1)
		for _, k := range keys[1:] {
			inner = append(inner, escapeJSONString(k, false))
		}
		value = d.Nest(o, inner, value)
	}
	if e.Anchor == "" {
		return d.Append(o, keyRaw, value), nil
	}
	anchor, err := findMember(d, i, keyPath(e.AnchorBlock, w.keyPathOf(e.Anchor)))
	if err != nil {
		return nil, err
	}
	if anchor.Object != o {
		return nil, format.StructureErrorf(i, format.StructureAnchor,
			"%s goes in the object at %s and %s is outside it; name a key of that object to put it beside, or no key to put it last there", path, objectName(o), anchor.Path)
	}
	if e.Before {
		return d.InsertBefore(anchor, keyRaw, value), nil
	}
	return d.InsertAfter(anchor, keyRaw, value), nil
}

// place finds where a member with key path path goes: the deepest object of
// d whose own key path starts path, and the keys that lead from it to the new
// value. One key is a member of that object. Several build objects inside one
// another under the first, unless the object already names values by flat key
// paths ("nav.home"), where the rest of path is one key.
func place(d *jsonobject.Doc, i int, path string) (*jsonobject.Object, []string, error) {
	o, rest := d.Root, path
	for at := strings.LastIndexByte(path, '.'); at > 0; at = strings.LastIndexByte(path[:at], '.') {
		objs := d.ObjectsAt(path[:at])
		if len(objs) > 1 {
			return nil, nil, format.StructureErrorf(i, format.StructureUnsupported, "two objects of the document have the key path %s", path[:at])
		}
		if len(objs) == 1 {
			o, rest = objs[0], path[at+1:]
			break
		}
	}
	if o == nil {
		return nil, nil, format.StructureErrorf(i, format.StructureUnsupported, "the document's top level is not an object, so it has no place for %s", path)
	}
	keys := []string{rest}
	if strings.Contains(rest, ".") && !o.DottedKeys() {
		keys = strings.Split(rest, ".")
	}
	for _, k := range keys {
		if k == "" || strings.HasSuffix(k, "]") {
			return nil, nil, format.StructureErrorf(i, format.StructureUnsupported,
				"%s names a value in an array or an empty key; a block is added only as a member of an object", path)
		}
	}
	if m := o.Member(keys[0]); m != nil {
		return nil, nil, format.StructureErrorf(i, format.StructureExists, "the document already has a value at %s, with no keys inside it", m.Path)
	}
	return o, keys, nil
}

// objectName names an object in a message.
func objectName(o *jsonobject.Object) string {
	if o.Path == "" {
		return "the top level"
	}
	return o.Path
}

// keyPath is the key path a block is found by: the path the reader recorded
// when the block's name is something else, else its name, else key.
func keyPath(b *model.Block, key string) string {
	if b != nil {
		if kp := b.Properties["json.keypath"]; kp != "" {
			return kp
		}
		if b.Name != "" {
			return b.Name
		}
	}
	return key
}

// findMember finds the one member at path.
func findMember(d *jsonobject.Doc, i int, path string) (*jsonobject.Member, error) {
	ms := d.Find(path)
	switch {
	case len(ms) == 1:
		return ms[0], nil
	case len(ms) > 1:
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "two members of the document have the key path %s", path)
	case strings.HasSuffix(path, "]"):
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s is a value in an array, which is named by its position; a block is added or removed only as a member of an object", path)
	}
	return nil, format.StructureErrorf(i, format.StructureNotFound, "the document has no member at %s", path)
}

// checkInserted checks that doc reads with one text member at each key path
// an insert added; inserted maps each path to its edit.
func checkInserted(doc []byte, inserted map[string]int) error {
	paths := slices.SortedFunc(maps.Keys(inserted), func(a, b string) int { return inserted[a] - inserted[b] })
	d, err := parseObjects(doc)
	if err != nil {
		return format.StructureErrorf(inserted[paths[0]], format.StructureUnsupported, "the document would not read as JSON after adding %s: %v", paths[0], err)
	}
	for _, path := range paths {
		i := inserted[path]
		m, serr := findMember(d, i, path)
		if serr != nil {
			return serr
		}
		if !d.IsString(m) {
			return format.StructureErrorf(i, format.StructureUnsupported, "%s would not hold a text value", path)
		}
	}
	return nil
}

// parseObjects reads doc with the JSON scanner.
func parseObjects(doc []byte) (*jsonobject.Doc, error) {
	toks, err := newScanner(doc).scan()
	if err != nil {
		return nil, err
	}
	out := make([]jsonobject.Token, len(toks))
	for i, t := range toks {
		out[i] = jsonobject.Token{Kind: objectKind(t.typ), Prefix: t.prefix, Raw: t.raw, Text: t.value}
	}
	return jsonobject.Parse(doc, out)
}

func objectKind(t tokenType) jsonobject.Kind {
	switch t {
	case tokenObjectStart:
		return jsonobject.ObjectStart
	case tokenObjectEnd:
		return jsonobject.ObjectEnd
	case tokenArrayStart:
		return jsonobject.ArrayStart
	case tokenArrayEnd:
		return jsonobject.ArrayEnd
	case tokenColon:
		return jsonobject.Colon
	case tokenComma:
		return jsonobject.Comma
	case tokenString:
		return jsonobject.String
	case tokenEOF:
		return jsonobject.EOF
	}
	return jsonobject.Scalar
}
