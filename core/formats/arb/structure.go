package arb

import (
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/internal/jsonobject"
	"github.com/neokapi/neokapi/core/internal/jsonscan"
	"github.com/neokapi/neokapi/core/model"
)

// An ARB message is a member of the top-level object, and its metadata is the
// "@<id>" member beside it. The writer keeps the two together: a message added
// after another goes after that message's metadata, one added before another
// goes before metadata that precedes it, and a message removed takes its
// metadata with it.
var _ format.StructureEditor = (*Writer)(nil)

// Structural declares insert_block and delete_block.
func (w *Writer) Structural() []string {
	return []string{format.StructuralDeleteBlock, format.StructuralInsertBlock}
}

// EditStructure adds and removes the messages edits name, in order.
func (w *Writer) EditStructure(doc []byte, edits []format.StructuralEdit) ([]byte, error) {
	for i, e := range edits {
		var err error
		switch e.Op {
		case format.StructuralDeleteBlock:
			doc, err = deleteMessage(doc, i, messageID(e.Block, e.Key))
		case format.StructuralInsertBlock:
			doc, err = insertMessage(doc, i, e)
		default:
			err = format.StructureErrorf(i, format.StructureUnsupported, "ARB has no structural operation %q", e.Op)
		}
		if err != nil {
			return nil, err
		}
	}
	return doc, nil
}

// messageID is the id a block's message is found by.
func messageID(b *model.Block, key string) string {
	if b != nil {
		if id := b.Properties["arb.key"]; id != "" {
			return id
		}
	}
	return key
}

// deleteMessage removes message id and its metadata.
func deleteMessage(doc []byte, i int, id string) ([]byte, error) {
	d, err := parseTop(doc, i)
	if err != nil {
		return nil, err
	}
	m, serr := message(d, i, id)
	if serr != nil {
		return nil, serr
	}
	doc = d.Delete(m)
	if d, err = parseTop(doc, i); err != nil {
		return nil, err
	}
	if meta := d.Root.Member("@" + id); meta != nil {
		doc = d.Delete(meta)
	}
	return doc, nil
}

// insertMessage adds e's message beside its anchor, or last.
func insertMessage(doc []byte, i int, e format.StructuralEdit) ([]byte, error) {
	if strings.HasPrefix(e.Key, "@") || e.Key == "" {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "an ARB message id cannot start with @, which marks metadata; %q is not a message id", e.Key)
	}
	d, err := parseTop(doc, i)
	if err != nil {
		return nil, err
	}
	if d.Root.Member(e.Key) != nil {
		return nil, format.StructureErrorf(i, format.StructureExists, "the catalog already has a member %q", e.Key)
	}
	text := e.Value
	if e.Runs != nil {
		text = valueFromRuns(e.Runs, "")
	}
	key, value := encodeJSONString(e.Key), encodeJSONString(text)
	if e.Anchor == "" {
		return d.Append(d.Root, key, value), nil
	}
	id := messageID(e.AnchorBlock, e.Anchor)
	anchor, serr := message(d, i, id)
	if serr != nil {
		return nil, serr
	}
	if e.Before {
		if prev := anchor.Prev(); prev != nil && prev.Key == "@"+id {
			anchor = prev
		}
		return d.InsertBefore(anchor, key, value), nil
	}
	if next := anchor.Next(); next != nil && next.Key == "@"+id {
		anchor = next
	}
	return d.InsertAfter(anchor, key, value), nil
}

// message finds message id in the top-level object.
func message(d *jsonobject.Doc, i int, id string) (*jsonobject.Member, error) {
	m := d.Root.Member(id)
	if m == nil || strings.HasPrefix(id, "@") {
		return nil, format.StructureErrorf(i, format.StructureNotFound, "the catalog has no message %q", id)
	}
	if !d.IsString(m) {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "%q holds no message text", id)
	}
	return m, nil
}

// parseTop reads doc's tokens into its objects; an ARB catalog is one object.
func parseTop(doc []byte, i int) (*jsonobject.Doc, error) {
	toks, err := jsonscan.New(doc, scanPrefix).Scan()
	if err != nil {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "the catalog does not read as JSON: %v", err)
	}
	out := make([]jsonobject.Token, len(toks))
	for j, t := range toks {
		out[j] = jsonobject.Token{Kind: objectKind(t.Type), Prefix: t.Prefix, Raw: t.Raw, Text: t.Value}
	}
	d, err := jsonobject.Parse(doc, out)
	if err != nil {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "the catalog does not read as JSON: %v", err)
	}
	if d.Root == nil {
		return nil, format.StructureErrorf(i, format.StructureUnsupported, "the catalog's top level is not an object")
	}
	return d, nil
}

func objectKind(t jsonscan.TokenType) jsonobject.Kind {
	switch t {
	case jsonscan.ObjectStart:
		return jsonobject.ObjectStart
	case jsonscan.ObjectEnd:
		return jsonobject.ObjectEnd
	case jsonscan.ArrayStart:
		return jsonobject.ArrayStart
	case jsonscan.ArrayEnd:
		return jsonobject.ArrayEnd
	case jsonscan.Colon:
		return jsonobject.Colon
	case jsonscan.Comma:
		return jsonobject.Comma
	case jsonscan.String:
		return jsonobject.String
	case jsonscan.EOF:
		return jsonobject.EOF
	}
	return jsonobject.Scalar
}
