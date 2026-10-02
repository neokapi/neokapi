package json

import (
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/internal/jsonobject"
	"github.com/neokapi/neokapi/core/model"
)

// A JSON block is a string value in an object, named by its key path, and its
// shell is the member that holds it: the key, the colon and the value. The
// writer adds a member beside another in the same object and removes one with
// the comma that separated it, keeping the document's indentation and every
// comment.
var _ format.StructureEditor = (*Writer)(nil)

// Structural declares insert_block and delete_block.
func (w *Writer) Structural() []string {
	return []string{format.StructuralDeleteBlock, format.StructuralInsertBlock}
}

// EditStructure adds and removes the members edits name, in order.
func (w *Writer) EditStructure(doc []byte, edits []format.StructuralEdit) ([]byte, error) {
	for i, e := range edits {
		d, err := parseObjects(doc)
		if err != nil {
			return nil, format.StructureErrorf(i, format.StructureUnsupported, "the document does not read as JSON: %v", err)
		}
		switch e.Op {
		case format.StructuralDeleteBlock:
			m, serr := findMember(d, i, keyPath(e.Block, e.Key))
			if serr != nil {
				return nil, serr
			}
			if !d.IsString(m) {
				return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s holds no text value, so no block is there to remove", m.Path)
			}
			doc = d.Delete(m)
		case format.StructuralInsertBlock:
			if doc, err = w.insert(d, i, e); err != nil {
				return nil, err
			}
			if _, err := findInserted(doc, i, e.Key); err != nil {
				return nil, err
			}
		default:
			return nil, format.StructureErrorf(i, format.StructureUnsupported, "JSON has no structural operation %q", e.Op)
		}
	}
	return doc, nil
}

// insert adds e's member to d.
func (w *Writer) insert(d *jsonobject.Doc, i int, e format.StructuralEdit) ([]byte, error) {
	value := escapeJSONStringQuoted(e.Value, w.cfg.EscapeForwardSlashes, '"')
	if len(d.Find(e.Key)) > 0 {
		return nil, format.StructureErrorf(i, format.StructureExists, "the document already has a value at %s", e.Key)
	}
	if e.Anchor == "" {
		if d.Root == nil {
			return nil, format.StructureErrorf(i, format.StructureUnsupported, "the document's top level is not an object, so it has no place for %s", e.Key)
		}
		if d.Root.Member(e.Key) != nil {
			return nil, format.StructureErrorf(i, format.StructureExists, "the document already has a member %q", e.Key)
		}
		return d.Append(d.Root, escapeJSONString(e.Key, false), value), nil
	}
	anchor, err := findMember(d, i, keyPath(e.AnchorBlock, e.Anchor))
	if err != nil {
		return nil, err
	}
	key := e.Key
	if p := anchor.Object.Path; p != "" {
		rest, ok := strings.CutPrefix(e.Key, p+".")
		if !ok || rest == "" {
			return nil, format.StructureErrorf(i, format.StructureUnsupported,
				"%s goes in the object at %s beside %s, so its key path starts with %s.", e.Key, p, anchor.Path, p)
		}
		key = rest
	}
	if anchor.Object.Member(key) != nil {
		return nil, format.StructureErrorf(i, format.StructureExists, "the object at %s already has a member %q", anchor.Object.Path, key)
	}
	if e.Before {
		return d.InsertBefore(anchor, escapeJSONString(key, false), value), nil
	}
	return d.InsertAfter(anchor, escapeJSONString(key, false), value), nil
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

// findInserted checks that the document doc reads with one string member at
// path, the member an insert added.
func findInserted(doc []byte, i int, path string) (*jsonobject.Member, error) {
	d, err := parseObjects(doc)
	if err != nil {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "the document would not read as JSON after adding %s: %v", path, err)
	}
	m, serr := findMember(d, i, path)
	if serr != nil {
		return nil, serr
	}
	if !d.IsString(m) {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "%s would not hold a text value", path)
	}
	return m, nil
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
