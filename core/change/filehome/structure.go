package filehome

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// A change set that adds or removes blocks (insert_block, delete_block) is
// written in two steps. restructure reads the document and the files of its
// editions as they stand, lets the service's editor check the operations
// against them, and has the format's writer (format.StructureEditor) write
// the shells of the blocks added and removed: into the document's own file,
// and into the file of each edition that holds a removed block or that a new
// block names. The results stay in the session's overlay, and the stage's
// pass then reads them as the document, so a new block's content is what the
// format reads back and every other operation applies to the document the
// structural edits left.
//
// In an edition's own file a new block goes beside the block that pairs with
// its anchor there, under the key that file gives it: a German file whose
// keys begin with de where the document's begin with en takes the new key
// with that prefix.

// restructure writes the change set's structural edits into the overlay.
func (st *staged) restructure(ctx context.Context) error {
	s := st.s
	r, ok := st.e.(change.Restructurer)
	if !ok {
		return errors.New("filehome: the editor adds and removes no blocks")
	}
	editions, ix, err := s.joinEditions(ctx, st.want.Editions)
	if err != nil {
		return err
	}
	r.StartStructure()
	si := 0
	err = s.ownPass(func(b *model.Block) error {
		join(editions, si, b)
		r.Locate(b)
		unjoin(editions, b)
		si++
		return nil
	}).run(ctx)
	if err != nil {
		return err
	}
	edits, err := r.Structure()
	if err != nil {
		return err
	}

	overlay := map[string][]byte{}
	own := s.ownSource()
	data, err := readAll(own)
	if err != nil {
		return err
	}
	out, err := writeStructure(s.doc.Format, data, ownEdits(edits))
	if err != nil {
		return refuseEdit(r, edits, nil, s.doc.Ref, err)
	}
	overlay[overlayKey(own)] = out

	for _, je := range editions {
		if !je.exists || je.kept != nil {
			// A new block's edition in a file that does not exist yet is
			// written when the pass materializes the file, and a kept
			// edition takes it from the pass as any change.
			continue
		}
		fedits, from, rerr := editionEdits(edits, je, ix)
		if rerr != nil {
			r.Refuse(rerr.edit, rerr.err)
			return change.ErrRefused
		}
		if len(fedits) == 0 {
			continue
		}
		data, err := readAll(je.src)
		if err != nil {
			return err
		}
		out, err := writeStructure(je.file.Format, data, fedits)
		if err != nil {
			return refuseEdit(r, edits, from, je.file.Ref, err)
		}
		overlay[je.file.Path] = out
	}
	s.overlay = overlay
	return nil
}

// writeStructure has the writer of format f make edits in data.
func writeStructure(f Binding, data []byte, edits []format.StructuralEdit) ([]byte, error) {
	if len(edits) == 0 {
		return data, nil
	}
	w, err := f.NewWriter()
	if err != nil {
		return nil, err
	}
	se, ok := w.(format.StructureEditor)
	if !ok {
		return nil, &format.StructureError{Reason: format.StructureUnsupported,
			Message: fmt.Sprintf("the %s format adds and removes no blocks", f.Name)}
	}
	return se.EditStructure(data, edits)
}

// refuseEdit refuses the structural edit a writer could not make. from maps
// the index of an edit as the writer saw it to the index of the change set's
// edit; nil is the identity.
func refuseEdit(r change.Restructurer, edits []change.StructuralEdit, from []int, file string, err error) error {
	var se *format.StructureError
	if !errors.As(err, &se) {
		return err
	}
	i := se.Edit
	if from != nil && i >= 0 && i < len(from) {
		i = from[i]
	}
	if i < 0 || i >= len(edits) {
		i = 0
	}
	e := edits[i]
	cerr := &change.Error{Code: change.CodeUnsupported, Capability: string(e.Kind), Message: file + ": " + se.Message}
	switch se.Reason {
	case format.StructureExists:
		cerr = &change.Error{Code: change.CodeInvalid, Field: "name", Message: file + ": " + se.Message}
	case format.StructureAnchor:
		// The change set names a block the new one cannot go beside: the
		// sender's to fix, as a field of the operation.
		field := "after"
		if e.Before {
			field = "before"
		}
		cerr = &change.Error{Code: change.CodeInvalid, Field: field, Message: file + ": " + se.Message}
	}
	r.Refuse(e, cerr)
	return change.ErrRefused
}

// ownEdits are the edits as the document's own file takes them.
func ownEdits(edits []change.StructuralEdit) []format.StructuralEdit {
	out := make([]format.StructuralEdit, 0, len(edits))
	for _, e := range edits {
		switch e.Kind {
		case change.KindDeleteBlock:
			out = append(out, format.StructuralEdit{Op: format.StructuralDeleteBlock, Key: e.Key, Block: e.Block})
		case change.KindInsertBlock:
			runs := e.Editions[model.EditionKey{}]
			out = append(out, format.StructuralEdit{Op: format.StructuralInsertBlock, Key: e.Key, Anchor: e.Anchor,
				AnchorBlock: e.AnchorBlock, Before: e.Before, Value: model.RenderRunsWithData(runs), Runs: runs})
		}
	}
	return out
}

// editionRefusal is an edit an edition's file has no place for.
type editionRefusal struct {
	edit change.StructuralEdit
	err  *change.Error
}

// editionEdits are the edits as the file of edition je takes them: each block
// removed that the file holds a partner of, and each new block that names the
// edition, beside the partner of its anchor. from maps each to its index in
// edits.
func editionEdits(edits []change.StructuralEdit, je *joinedEdition, ix *blockIndex) (out []format.StructuralEdit, from []int, refusal *editionRefusal) {
	// added holds the key each new block of the edition has in this file.
	added := map[string]string{}
	for i, e := range edits {
		switch e.Kind {
		case change.KindDeleteBlock:
			ti, ok := je.match[e.Index]
			if !ok {
				continue
			}
			partner := je.blocks[ti]
			out = append(out, format.StructuralEdit{Op: format.StructuralDeleteBlock, Key: change.BlockKey(partner), Block: partner})
			from = append(from, i)
		case change.KindInsertBlock:
			runs, ok := e.Editions[je.key]
			if !ok {
				continue
			}
			fe := format.StructuralEdit{Op: format.StructuralInsertBlock, Before: e.Before, Value: model.RenderRunsWithData(runs), Runs: runs}
			docKey, fileKey := "", ""
			switch {
			case e.Anchor == "":
				docKey, fileKey = je.anyPair(ix)
			case e.AnchorIndex >= 0:
				ti, paired := je.match[e.AnchorIndex]
				if !paired {
					return nil, nil, je.noPlace(e)
				}
				fe.AnchorBlock = je.blocks[ti]
				fe.Anchor = change.BlockKey(fe.AnchorBlock)
				docKey, fileKey = change.BlockKey(e.AnchorBlock), fe.Anchor
			default:
				k, here := added[e.Anchor]
				if !here {
					return nil, nil, je.noPlace(e)
				}
				fe.Anchor = k
				docKey, fileKey = e.Anchor, k
			}
			fe.Key = translateKey(e.Key, docKey, fileKey)
			added[e.Key] = fe.Key
			out = append(out, fe)
			from = append(from, i)
		}
	}
	return out, from, nil
}

// noPlace refuses a new block's edition that has no place in the edition's
// file.
func (je *joinedEdition) noPlace(e change.StructuralEdit) *editionRefusal {
	return &editionRefusal{edit: e, err: &change.Error{Code: change.CodeUnsupported, Capability: string(change.KindInsertBlock),
		Field: "editions/" + keyText(je.key),
		Message: fmt.Sprintf("%s holds no block that pairs with %s, so edition %s of the new block %s has no place there; add the block without that edition, or beside a block %s holds",
			je.file.Ref, e.Anchor, keyText(je.key), e.Key, je.file.Ref)}}
}

// anyPair is the keys of the first block of the document the file holds a
// partner of: the document's key and the file's.
func (je *joinedEdition) anyPair(ix *blockIndex) (docKey, fileKey string) {
	first := -1
	for si := range je.match {
		if first < 0 || si < first {
			first = si
		}
	}
	if first < 0 || ix == nil || first >= len(ix.keys) {
		return "", ""
	}
	return ix.keys[first], change.BlockKey(je.blocks[je.match[first]])
}

// translateKey gives key, a key of the document, the form the edition's file
// gives keys: where docKey and fileKey, a block's keys in the two, differ
// only in their first segments, key's first segments are replaced the same
// way.
func translateKey(key, docKey, fileKey string) string {
	if docKey == fileKey || docKey == "" || fileKey == "" {
		return key
	}
	d, f := strings.Split(docKey, "."), strings.Split(fileKey, ".")
	n := 0
	for n < len(d) && n < len(f) && d[len(d)-1-n] == f[len(f)-1-n] {
		n++
	}
	docPrefix := strings.Join(d[:len(d)-n], ".")
	filePrefix := strings.Join(f[:len(f)-n], ".")
	if docPrefix == "" || n == 0 {
		return key
	}
	rest, ok := strings.CutPrefix(key, docPrefix+".")
	if !ok {
		return key
	}
	if filePrefix == "" {
		return rest
	}
	return filePrefix + "." + rest
}

func keyText(k model.EditionKey) string {
	b, _ := k.Canonical().MarshalText()
	return string(b)
}
