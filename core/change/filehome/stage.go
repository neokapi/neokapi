package filehome

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/neokapi/neokapi/core/atomicfile"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage/filelock"
)

// staged is a change held ready to commit: a temporary file beside each file
// it changes.
type staged struct {
	s     *session
	want  change.Want
	e     change.Editor
	files []*stagedFile
	// keys are the lock files of the files the stage reads or changes,
	// sorted; held are the locks taken on them.
	keys []string
	held map[string]*filelock.Lock
	// settled says the commit locks are held and every staged file applies
	// to the file as it stands.
	settled bool
	// rec is the record of the change the service hands a commit that
	// records a kept edition, and recordID the id that commit appended.
	rec      *change.Record
	recordID string
}

// Recording keeps the record of the change for the commit of a kept edition.
func (st *staged) Recording(rec change.Record) { st.rec = &rec }

// RecordID is the id of the record the commit of a kept edition appended.
func (st *staged) RecordID() string { return st.recordID }

var _ change.RecordingStaged = (*staged)(nil)

// stagedFile is one file a stage changes, or reads and leaves.
type stagedFile struct {
	ref     string
	edition *model.EditionKey
	// path is the file on disk.
	path string
	// before is the file's digest when the stage read it; empty when the
	// stage creates it.
	before string
	after  string
	tmp    *atomicfile.Staged
	// written says Commit renamed the staged file into place.
	written bool
	// diff renders the change for a preview.
	diff func() string
	// keeper keeps the edition the file will hold once delivered, for an
	// edition with no file yet (EditionFile.Kept), and token names the head
	// the stage read there. kept is the change the commit hands the keeper;
	// nil when the stage leaves the edition as it was.
	keeper Keeper
	token  string
	kept   *keptPart
}

// run reads every file the change needs, applies the editor, and stages the
// files it changes. It starts afresh: whatever an earlier run staged is
// discarded.
func (st *staged) run(ctx context.Context) error {
	st.discard()
	s := st.s
	own := &stagedFile{ref: s.doc.Ref, path: s.doc.Path}
	var err error
	if own.before, err = hashFile(s.doc.Path); err != nil {
		return err
	}
	if own.before == "" {
		return &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: "no document " + s.doc.Ref}
	}
	own.after = own.before
	st.files = []*stagedFile{own}

	// Each edition's file is hashed before the join reads it, as the
	// document's own file is: a writer that commits between the hash and the
	// read leaves the file at another digest than the one recorded, and
	// Settle reads it again under the lock.
	before := map[string]string{}
	for _, k := range st.want.Editions {
		if f, ok := s.editionFile(k); ok && f.Kept == nil {
			if before[f.Path], err = hashFile(f.Path); err != nil {
				return err
			}
		}
	}
	// A change set that adds or removes blocks has the format write those
	// first (structure.go); the pass below then reads the result.
	s.overlay = nil
	defer func() { s.overlay = nil }()
	if st.want.Structural {
		if err := st.restructure(ctx); err != nil {
			return err
		}
	}
	editions, ix, err := s.joinEditions(ctx, st.want.Editions)
	if err != nil {
		return err
	}

	// What the editor changed in each joined edition, by document block index:
	// the runs for a file, and the whole edition, or nil for one removed, for
	// a kept edition; and the blocks it removed the edition from.
	changed := make([]map[int][]model.Run, len(editions))
	changedKept := make([]map[int]*model.Edition, len(editions))
	gone := make([]map[int]bool, len(editions))
	for i := range changed {
		changed[i] = map[int][]model.Run{}
		changedKept[i] = map[int]*model.Edition{}
		gone[i] = map[int]bool{}
	}
	ownChanged := false
	// The editions the editor removed from blocks the document holds them in,
	// by block index, which the write is read back for (verifyRemoved).
	removed := map[int][]model.EditionKey{}
	si := 0
	edit := func(b *model.Block) error {
		join(editions, si, b)
		keys, err := st.e.Edit(b)
		for _, k := range keys {
			at := slices.IndexFunc(editions, func(je *joinedEdition) bool { return je.key == k.Canonical() })
			if at < 0 {
				ownChanged = true
				if _, held := b.Edition(k); !held {
					removed[si] = append(removed[si], k)
				}
				continue
			}
			ed, held := b.Edition(editions[at].key)
			changed[at][si] = ed.Runs
			if held {
				changedKept[at][si] = &ed
			} else {
				changedKept[at][si] = nil
				gone[at][si] = true
			}
		}
		unjoin(editions, b)
		si++
		return err
	}

	st.e.Begin()
	if st.want.Own {
		own.tmp, own.after, own.diff, err = st.write(ctx, s.doc.Path, s.ownSource(), func(out io.Writer) error {
			p := s.ownPass(edit)
			p.out = out
			return p.run(ctx)
		})
	} else {
		err = s.ownPass(edit).run(ctx)
	}
	if err != nil {
		return err
	}
	if err := st.e.End(); err != nil {
		if errors.Is(err, change.ErrRefused) {
			// The refusal names the files the pass read, the editions' own
			// files with the document's.
			for _, je := range editions {
				if je.kept == nil {
					digest := before[je.file.Path]
					st.files = append(st.files, &stagedFile{ref: je.file.Ref, edition: &je.key, path: je.file.Path, before: digest, after: digest})
				}
			}
		}
		return err
	}
	if !ownChanged && own.tmp != nil {
		_ = own.tmp.Discard()
		own.tmp, own.after, own.diff = nil, own.before, nil
	}
	if len(removed) > 0 && own.tmp != nil && s.doc.Entry == "" {
		if err := st.verifyRemoved(ctx, own.tmp.Name(), removed); err != nil {
			return err
		}
	}
	if data, ok := s.overlay[overlayKey(source{path: s.doc.Path, entry: s.doc.Entry})]; ok && own.tmp == nil {
		// Blocks added or removed are the whole change to the file: the
		// bytes the format wrote for them are what lands.
		if own.tmp, own.after, own.diff, err = st.writeBytes(ctx, s.doc.Path, s.ownSource(), data); err != nil {
			return err
		}
	}

	for i, je := range editions {
		if je.kept != nil {
			st.stageKept(je, ix, changedKept[i])
			continue
		}
		digest := before[je.file.Path]
		f := &stagedFile{ref: je.file.Ref, edition: &je.key, path: je.file.Path, before: digest, after: digest}
		st.files = append(st.files, f)
		if len(changed[i]) == 0 {
			if data, ok := s.overlay[je.file.Path]; ok {
				if f.tmp, f.after, f.diff, err = st.writeBytes(ctx, je.file.Path, je.src, data); err != nil {
					return err
				}
				continue
			}
			if !s.h.materialize || je.file.Bilingual || !je.drifted(ix) {
				continue
			}
			// The translation no longer holds the document's blocks: it
			// follows the document again even though no content changed.
		}
		if err := st.writeEdition(ctx, f, je, ix, changed[i], gone[i]); err != nil {
			return err
		}
		if f.after == f.before && f.tmp != nil {
			// The write gave the bytes the file already holds.
			_ = f.tmp.Discard()
			f.tmp, f.diff = nil, nil
		}
	}
	return st.lockKeys()
}

// verifyRemoved reads back the document as the stage wrote it to the file at
// staged and refuses the change when a translation the editor removed from a
// block is still there. Some bilingual writers write a translation of every
// unit and fill one the block does not hold from its source, as the Okapi
// filters they follow do: such a write would replace the translation with the
// source rather than remove it.
func (st *staged) verifyRemoved(ctx context.Context, staged string, removed map[int][]model.EditionKey) error {
	data, err := os.ReadFile(staged)
	if err != nil {
		return err
	}
	s := st.s
	var kept *change.Error
	var keptKey string
	var keptEdition model.EditionKey
	si := 0
	p := s.ownPass(func(b *model.Block) error {
		for _, k := range removed[si] {
			if _, held := b.Edition(k); held && kept == nil {
				text, _ := k.Canonical().MarshalText()
				keptKey, keptEdition = change.BlockKey(b), k
				kept = &change.Error{Code: change.CodeUnsupported, Capability: string(change.KindRemoveEdition), Field: "at/edition",
					Message: fmt.Sprintf("the %s writer writes a translation of every unit, taking the source where a block holds none, so removing translation %s of block %s from %s would write its source in its place; give the translation new content with set_content instead",
						s.doc.Format.Name, text, keptKey, s.doc.Ref)}
			}
		}
		si++
		return nil
	})
	p.src = p.src.with(data)
	if err := p.run(ctx); err != nil {
		return err
	}
	if kept != nil {
		return st.refuseEdition(keptKey, keptEdition, kept)
	}
	return nil
}

// refuseEdition refuses with err the operations that changed edition k of
// the block the document keys key (change.EditionRefuser) and returns
// change.ErrRefused, so the change set is refused at that operation and every
// other is not applied. An editor that refuses no operation there has err
// refuse the document instead.
func (st *staged) refuseEdition(key string, k model.EditionKey, err *change.Error) error {
	if r, ok := st.e.(change.EditionRefuser); ok && r.RefuseEdition(key, k, err) {
		return change.ErrRefused
	}
	return err
}

// lockKeys names the lock file of every file the stage reads or changes.
func (st *staged) lockKeys() error {
	st.keys = st.keys[:0]
	for _, f := range st.files {
		p, err := st.s.h.lockPath(f.path)
		if err != nil {
			return err
		}
		if !slices.Contains(st.keys, p) {
			st.keys = append(st.keys, p)
		}
	}
	slices.Sort(st.keys)
	return nil
}

// writeEdition stages the file of a joined edition with the runs the editor
// gave it, and without the edition in the blocks gone names. A file that
// exists is edited through its own skeleton, so every byte of it outside the
// changed blocks stays, and each changed block must have a partner there; a
// write that would need a block the file does not hold is refused, because
// adding one means rewriting the file. A file that does not exist yet is
// materialized from the document's skeleton, as kapi merge writes a target
// file, and so is every file under Options.Materialize, except a bilingual
// file that still holds every block of the document and nothing else: it
// keeps its own skeleton, so its header (its language, its plural rule) and
// its comments stay. A bilingual file a translation was removed from is read
// back (verifyEditionRemoved); any other file loses the block that held the
// translation (restructureEditionFile).
func (st *staged) writeEdition(ctx context.Context, f *stagedFile, je *joinedEdition, ix *blockIndex, changed map[int][]model.Run, gone map[int]bool) error {
	s := st.s
	inPlace := je.exists && !s.h.materialize
	if je.exists && s.h.materialize && je.file.Bilingual && !je.drifted(ix) {
		inPlace = true
	}
	if inPlace {
		return st.writeEditionInPlace(ctx, f, je, ix, changed, gone)
	}
	src := s.ownSource()
	si := 0
	var err error
	f.tmp, f.after, f.diff, err = st.write(ctx, je.file.Path, source{path: je.file.Path}, func(out io.Writer) error {
		return pass{src: src, format: s.doc.Format, locale: s.doc.SourceLocale, encoding: s.doc.Encoding, out: out,
			writeLocale: je.key.Locale, writerSource: src,
			fn: func(b *model.Block) error {
				runs, ok := changed[si]
				if ti, paired := je.match[si]; !ok && paired {
					// A block the change leaves keeps what the file held.
					runs, ok = je.held(ti)
				}
				if ok && !gone[si] {
					b.SetEdition(je.key, model.Edition{Runs: runs})
				}
				si++
				return nil
			}}.run(ctx)
	})
	if err != nil || len(gone) == 0 {
		return err
	}
	// The file is written from the document's skeleton, so its blocks have
	// the document's keys.
	removed := make(map[string]string, len(gone))
	for si := range gone {
		removed[ix.keys[si]] = ix.keys[si]
	}
	if !je.file.Bilingual {
		return st.restructureEditionFile(ctx, f, je, removed, nil)
	}
	return st.verifyEditionRemoved(ctx, f, je, removed)
}

// stageKept stages the change to an edition a keeper holds: the blocks the
// editor changed, each with the revision it had, and the edition's digest
// around the change. Nothing is written until Commit hands it to the keeper.
func (st *staged) stageKept(je *joinedEdition, ix *blockIndex, changed map[int]*model.Edition) {
	k := je.kept
	f := &stagedFile{ref: je.file.Ref, edition: &je.key, path: je.file.Path, before: k.Digest, after: k.Digest,
		keeper: je.file.Kept, token: k.Token}
	st.files = append(st.files, f)
	if len(changed) == 0 {
		return
	}
	now := maps.Clone(k.Blocks)
	if now == nil {
		now = map[string]model.Edition{}
	}
	var changes []KeptChange
	for _, si := range slices.Sorted(maps.Keys(changed)) {
		key := ix.keys[si]
		was, held := now[key]
		before := model.AbsentRevision
		if held {
			before = model.RunsRevision(je.key, was.Runs)
		}
		ed := changed[si]
		switch {
		case ed == nil && !held:
			continue
		case ed == nil:
			delete(now, key)
		case held && KeptEntry(je.key, was) == KeptEntry(je.key, *ed):
			continue
		default:
			now[key] = *ed
		}
		changes = append(changes, KeptChange{Block: key, Before: before, Edition: ed})
	}
	if len(changes) == 0 {
		return
	}
	f.after = KeptDigest(je.key, now)
	f.kept = &keptPart{keeper: je.file.Kept, write: KeptWrite{Doc: st.s.doc.Ref, File: je.file.Ref, Edition: je.key,
		Token: k.Token, Before: f.before, After: f.after, Changes: changes}}
	held := k.Blocks
	f.diff = func() string { return keptDiff(je.file.Ref, je.key, held, changes) }
}

// writeEditionInPlace stages the existing file of a joined edition through
// its own skeleton, with each changed block written into its partner there.
// A translation the change creates for a block the file holds no partner of
// is added to a file in the edition's own language as a block of its own,
// through the format's writer (restructureEditionFile); a file whose format
// adds no block, or a bilingual file, refuses the write.
func (st *staged) writeEditionInPlace(ctx context.Context, f *stagedFile, je *joinedEdition, ix *blockIndex, changed map[int][]model.Run, gone map[int]bool) error {
	s := st.s
	byTarget := map[int][]model.Run{}
	goneAt := map[int]bool{}
	// removed maps the file's key of each block whose translation leaves it
	// to the document's key of the block it translates.
	removed := map[string]string{}
	var created, unpaired []int
	for si, runs := range changed {
		ti, ok := je.match[si]
		switch {
		case !ok && !gone[si]:
			created = append(created, si)
			continue
		case !ok:
			unpaired = append(unpaired, si)
			continue
		}
		byTarget[ti] = runs
		if gone[si] {
			goneAt[ti] = true
			removed[change.BlockKey(je.blocks[ti])] = ix.keys[si]
		}
	}
	if len(created) > 0 && (je.file.Bilingual || !addsBlocks(je.file.Format)) {
		unpaired, created = append(unpaired, created...), nil
	}
	if len(unpaired) > 0 {
		refusal := func(keys []string) *change.Error {
			return &change.Error{Code: change.CodeUnsupported, Capability: "edition", Field: "at/block",
				Message: fmt.Sprintf("%s holds no block that pairs with %s of %s, and an edition is written only into a block its file already holds; add the block to %s first",
					je.file.Ref, blockList(keys), s.doc.Ref, je.file.Ref)}
		}
		keys := make([]string, 0, len(unpaired))
		for _, si := range unpaired {
			keys = append(keys, ix.keys[si])
		}
		slices.Sort(keys)
		if r, ok := st.e.(change.EditionRefuser); ok {
			each := true
			for _, k := range keys {
				each = r.RefuseEdition(k, je.key, refusal([]string{k})) && each
			}
			if each {
				return change.ErrRefused
			}
		}
		return refusal(keys)
	}
	// A bilingual file holds the edition as its translation, read and
	// written in the edition's language; any other file holds it as its own
	// content.
	key, lang := model.EditionKey{}, model.LocaleID("")
	if je.file.Bilingual {
		key, lang = je.key, je.key.Locale
	}
	src := je.src
	ti := 0
	var err error
	f.tmp, f.after, f.diff, err = st.write(ctx, je.file.Path, src, func(out io.Writer) error {
		return pass{src: src, format: je.file.Format, locale: s.doc.SourceLocale, target: lang, encoding: s.doc.Encoding, out: out,
			writeLocale: lang,
			fn: func(b *model.Block) error {
				switch runs, ok := byTarget[ti]; {
				case ok && goneAt[ti] && je.file.Bilingual:
					b.RemoveEdition(key)
				case ok && goneAt[ti]:
					// A file of the edition's own holds the translation as
					// the block itself, which leaves the file below
					// (restructureEditionFile).
				case ok:
					ed, _ := b.Edition(key)
					ed.Runs = runs
					b.SetEdition(key, ed)
				}
				ti++
				return nil
			}}.run(ctx)
	})
	if err != nil {
		return err
	}
	if je.file.Bilingual {
		if len(removed) == 0 {
			return nil
		}
		return st.verifyEditionRemoved(ctx, f, je, removed)
	}
	if len(removed) == 0 && len(created) == 0 {
		return nil
	}
	return st.restructureEditionFile(ctx, f, je, removed, creations(je, ix, created, changed, goneAt))
}

// addsBlocks reports whether the writer of format b adds a block to a
// document (format.StructureEditor).
func addsBlocks(b Binding) bool {
	if b.NewWriter == nil {
		return false
	}
	w, err := b.NewWriter()
	if err != nil {
		return false
	}
	return slices.Contains(format.StructuralOps(w), format.StructuralInsertBlock)
}

// creation is a translation a change creates for a block of the document
// that the edition's file, in the edition's own language, holds no partner
// of: the block the file's writer adds there to hold it.
type creation struct {
	// docKey is the document's key of the block translated, and edit the
	// block the writer adds, under the key the file gives it.
	docKey string
	edit   format.StructuralEdit
}

// creations are the blocks of the translations the change creates in a file
// in the edition's own language that holds no partner of them, at the
// document indexes created. Each goes under the key the file gives keys
// (translateKey, as a new block's edition is keyed there), after the partner
// of the document's nearest block before it that the file holds, or the
// block added for that one, when its key there shares the parent of the new
// key; else before the nearest such block after it, on the same terms; else
// last in the mapping its key names.
func creations(je *joinedEdition, ix *blockIndex, created []int, changed map[int][]model.Run, goneAt map[int]bool) []creation {
	slices.Sort(created)
	docKey, fileKey := je.anyPair(ix)
	added := map[int]string{}
	// in is the file's key of the document's block sj and its block there,
	// when the file holds it once the change is written.
	in := func(sj int) (string, *model.Block, bool) {
		if k, ok := added[sj]; ok {
			return k, nil, true
		}
		ti, ok := je.match[sj]
		if !ok || goneAt[ti] {
			return "", nil, false
		}
		return change.BlockKey(je.blocks[ti]), je.blocks[ti], true
	}
	out := make([]creation, 0, len(created))
	for _, si := range created {
		runs := changed[si]
		e := format.StructuralEdit{Op: format.StructuralInsertBlock, Key: translateKey(ix.keys[si], docKey, fileKey),
			Value: model.RenderRunsWithData(runs), Runs: runs}
		parent := parentKey(e.Key)
		for sj := si - 1; sj >= 0; sj-- {
			if k, b, ok := in(sj); ok {
				if parentKey(k) == parent {
					e.Anchor, e.AnchorBlock = k, b
				}
				break
			}
		}
		for sj := si + 1; e.Anchor == "" && sj < len(ix.keys); sj++ {
			if k, b, ok := in(sj); ok {
				if parentKey(k) == parent {
					e.Anchor, e.AnchorBlock, e.Before = k, b, true
				}
				break
			}
		}
		added[si] = e.Key
		out = append(out, creation{docKey: ix.keys[si], edit: e})
	}
	return out
}

// parentKey is the key path of the mapping a block keyed key sits in, as
// JSON and YAML key paths name it.
func parentKey(key string) string {
	if parent, _, ok := strings.CutLast(key, "."); ok {
		return parent
	}
	return ""
}

// restructureEditionFile takes the blocks removed names out of the file the
// stage wrote for a joined edition in the edition's own language, and adds
// the blocks of adds to it. Such a file holds a translation as a block of its
// own, so a translation leaves it with its block, as delete_block removes a
// block from an edition's file, and a translation created there arrives with
// one; the format's writer makes both (format.StructureEditor). removed maps
// the file's key of each block to the document's key of the block it
// translates. A block the writer cannot remove or add refuses the operations
// on it, and the file is never deleted: it keeps every other block, and
// whatever else the format writes there when none is left.
func (st *staged) restructureEditionFile(ctx context.Context, f *stagedFile, je *joinedEdition, removed map[string]string, adds []creation) error {
	s := st.s
	var data []byte
	var err error
	if f.tmp != nil {
		data, err = os.ReadFile(f.tmp.Name())
	} else {
		data, err = readAll(je.src)
	}
	if err != nil {
		return err
	}
	// The blocks are found by key: an edit of the pass can change what the
	// reader reads ahead of them (a YAML value written as a number reads as
	// no block), and with it the place of every block after.
	var edits []format.StructuralEdit
	var docKeys []string
	seen := map[string]int{}
	err = s.readPass(source{path: je.file.Path}.with(data), je.file.Format, func(b *model.Block) error {
		k := change.BlockKey(b)
		if docKey, ok := removed[k]; ok {
			if seen[k]++; seen[k] == 1 {
				edits = append(edits, format.StructuralEdit{Op: format.StructuralDeleteBlock, Key: k, Block: b})
				docKeys = append(docKeys, docKey)
			}
		}
		return nil
	}).run(ctx)
	if err != nil {
		return err
	}
	for _, k := range slices.Sorted(maps.Keys(removed)) {
		switch seen[k] {
		case 0:
			return fmt.Errorf("remove a translation from %s: the file the stage wrote holds no block keyed %s", je.file.Ref, k)
		case 1:
		default:
			return st.refuseEdition(removed[k], je.key, &change.Error{Code: change.CodeUnsupported, Capability: string(change.KindRemoveEdition), Field: "at/edition",
				Message: fmt.Sprintf("%s holds more than one block keyed %s, so translation %s of block %s of %s cannot leave it with its block; give the translation new content with set_content instead",
					je.file.Ref, k, keyText(je.key), removed[k], s.doc.Ref)})
		}
	}
	removals := len(edits)
	for _, a := range adds {
		edits = append(edits, a.edit)
		docKeys = append(docKeys, a.docKey)
	}
	out, err := writeStructure(je.file.Format, data, edits)
	var se *format.StructureError
	for err != nil && errors.As(err, &se) && se.Edit >= removals && se.Edit < len(edits) && edits[se.Edit].Anchor != "" {
		// The writer has no place for the new block beside the one chosen
		// (another mapping, a sequence), so it goes last in the mapping its
		// key names.
		edits[se.Edit].Anchor, edits[se.Edit].AnchorBlock, edits[se.Edit].Before = "", nil, false
		out, err = writeStructure(je.file.Format, data, edits)
	}
	if err != nil {
		if !errors.As(err, &se) {
			return err
		}
		i := se.Edit
		if i < 0 || i >= len(edits) {
			i = 0
		}
		if i < removals {
			return st.refuseEdition(docKeys[i], je.key, &change.Error{Code: change.CodeUnsupported, Capability: string(change.KindRemoveEdition), Field: "at/edition",
				Message: fmt.Sprintf("translation %s of block %s of %s lives in %s, which it leaves only with its block, and %s; give the translation new content with set_content instead",
					keyText(je.key), docKeys[i], s.doc.Ref, je.file.Ref, se.Message)})
		}
		advice := "add the block to " + je.file.Ref + " first"
		if se.Reason == format.StructureExists {
			advice = "give that entry of " + je.file.Ref + " a text value, or remove it, first"
		}
		return st.refuseEdition(docKeys[i], je.key, &change.Error{Code: change.CodeUnsupported, Capability: "edition", Field: "at/block",
			Message: fmt.Sprintf("%s holds no block that pairs with block %s of %s, so translation %s arrives there with a block of its own, and %s; %s",
				je.file.Ref, docKeys[i], s.doc.Ref, keyText(je.key), se.Message, advice)})
	}
	if len(adds) > 0 {
		if err := st.verifyCreated(ctx, je, out, adds); err != nil {
			return err
		}
	}
	if f.tmp != nil {
		_ = f.tmp.Discard()
		f.tmp = nil
	}
	f.tmp, f.after, f.diff, err = st.writeBytes(ctx, je.file.Path, je.src, out)
	return err
}

// verifyCreated reads back data, a file in the edition's own language with
// the blocks of adds added, and refuses a translation it reads otherwise than
// it was given: no block under its key, or a value the format reads as other
// text.
func (st *staged) verifyCreated(ctx context.Context, je *joinedEdition, data []byte, adds []creation) error {
	w, err := je.file.Format.NewWriter()
	if err != nil {
		return err
	}
	want := make(map[string]bool, len(adds))
	for _, a := range adds {
		want[a.edit.Key] = true
	}
	read := map[string][]*model.Block{}
	err = st.s.readPass(source{path: je.file.Path}.with(data), je.file.Format, func(b *model.Block) error {
		if k := change.BlockKey(b); want[k] {
			read[k] = append(read[k], b)
		}
		return nil
	}).run(ctx)
	if err != nil {
		return err
	}
	for _, a := range adds {
		blocks := read[a.edit.Key]
		reads := fmt.Sprintf("reads %d blocks keyed %s there", len(blocks), a.edit.Key)
		if len(blocks) == 1 {
			ed, _ := blocks[0].Edition(model.EditionKey{})
			got := format.SpellValue(w, blocks[0], ed.Runs)
			if got == format.SpellValue(w, nil, a.edit.Runs) {
				continue
			}
			reads = fmt.Sprintf("reads it as %q", got)
		}
		return st.refuseEdition(a.docKey, je.key, &change.Error{Code: change.CodeUnsupported, Capability: "edition", Field: "at/block",
			Message: fmt.Sprintf("translation %s of block %s of %s arrives in %s as a block keyed %s, and the %s format %s, not as given",
				keyText(je.key), a.docKey, st.s.doc.Ref, je.file.Ref, a.edit.Key, je.file.Format.Name, reads)})
	}
	return nil
}

// verifyEditionRemoved reads back the bilingual file of a joined edition as
// the stage wrote it, in the edition's language, and refuses the change when
// a block removed names (by the file's key, mapped to the document's) still
// holds the translation the editor removed: a writer that fills a unit with
// no translation from its source, as the XLIFF and TMX writers do, would
// replace the translation with the source rather than remove it.
func (st *staged) verifyEditionRemoved(ctx context.Context, f *stagedFile, je *joinedEdition, removed map[string]string) error {
	if f.tmp == nil {
		return nil
	}
	data, err := os.ReadFile(f.tmp.Name())
	if err != nil {
		return err
	}
	s := st.s
	var kept *change.Error
	var keptKey string
	p := pass{src: source{path: je.file.Path}.with(data), format: je.file.Format, locale: s.doc.SourceLocale, target: je.key.Locale,
		encoding: s.doc.Encoding, fn: func(b *model.Block) error {
			docKey, ok := removed[change.BlockKey(b)]
			if _, held := b.Edition(je.key); held && ok && kept == nil {
				text, _ := je.key.MarshalText()
				keptKey = docKey
				kept = &change.Error{Code: change.CodeUnsupported, Capability: string(change.KindRemoveEdition), Field: "at/edition",
					Message: fmt.Sprintf("the %s writer writes a translation of every unit, taking the source where a block holds none, so removing translation %s of block %s from %s would write its source in its place; give the translation new content with set_content instead",
						je.file.Format.Name, text, change.BlockKey(b), je.file.Ref)}
			}
			return nil
		}}
	if err := p.run(ctx); err != nil {
		return err
	}
	if kept != nil {
		return st.refuseEdition(keptKey, je.key, kept)
	}
	return nil
}

// blockList names up to three blocks in a message.
func blockList(keys []string) string {
	const shown = 3
	if len(keys) == 1 {
		return "block " + keys[0]
	}
	if len(keys) <= shown {
		return "blocks " + strings.Join(keys, ", ")
	}
	return fmt.Sprintf("blocks %s and %d more", strings.Join(keys[:shown], ", "), len(keys)-shown)
}

// write stages what produce writes as the new content of the file at path:
// for a plain file, the bytes themselves; for an archive member, the member,
// spliced into a copy of the archive with every other member as it was. It
// returns the staged file, its digest, and a renderer of the change. A file
// whose directory does not exist yet, such as a translation written for the
// first time, has the directory created when it commits, never before.
func (st *staged) write(ctx context.Context, path string, src source, produce func(io.Writer) error) (*atomicfile.Staged, string, func() string, error) {
	h := sha256.New()
	if src.entry == "" {
		tmp, err := atomicfile.StageWithParents(path, func(w io.Writer) error {
			bw := bufio.NewWriterSize(io.MultiWriter(w, h), 64*1024)
			if err := produce(bw); err != nil {
				return err
			}
			return bw.Flush()
		})
		if err != nil {
			return nil, "", nil, err
		}
		after := digestOf(h)
		return tmp, after, func() string { return fileDiff(src.path, tmp.Name(), st.label(path)) }, nil
	}
	var member bytes.Buffer
	if err := produce(&member); err != nil {
		return nil, "", nil, err
	}
	tmp, err := atomicfile.Stage(path, func(w io.Writer) error {
		return container.Transform(path, io.MultiWriter(w, h), func(name string, _ func() ([]byte, error)) ([]byte, bool, error) {
			if sameEntry(name, src.entry) {
				return member.Bytes(), true, nil
			}
			return nil, false, nil
		})
	})
	if err != nil {
		return nil, "", nil, err
	}
	after := digestOf(h)
	edited := member.Bytes()
	return tmp, after, func() string {
		before, berr := source{path: src.path, entry: src.entry}.entryBytes()
		if berr != nil {
			return ""
		}
		return textDiff(before, edited, st.label(path)+"!"+src.entry)
	}, nil
}

// writeBytes stages data as the new content of the file at path, src being
// the file or archive member data replaces.
func (st *staged) writeBytes(ctx context.Context, path string, src source, data []byte) (*atomicfile.Staged, string, func() string, error) {
	return st.write(ctx, path, src, func(out io.Writer) error {
		_, err := out.Write(data)
		return err
	})
}

// label names a file in a diff: its reference.
func (st *staged) label(path string) string {
	for _, f := range st.files {
		if f.path == path {
			return f.ref
		}
	}
	if path == st.s.doc.Path {
		return st.s.doc.Ref
	}
	return filepath.Base(path)
}

func digestOf(h hash.Hash) string { return "sha256:" + hex.EncodeToString(h.Sum(nil)) }

// sameEntry compares two archive member names up to slashes and a leading ./.
func sameEntry(a, b string) bool {
	norm := func(s string) string { return strings.TrimPrefix(filepath.ToSlash(s), "./") }
	return norm(a) == norm(b)
}

func (st *staged) Files() []change.StagedFile {
	out := make([]change.StagedFile, 0, len(st.files))
	for _, f := range st.files {
		sf := change.StagedFile{File: f.ref, Edition: f.edition, Before: f.before, After: f.after, Written: f.written}
		if f.keeper != nil {
			sf.Home, sf.Recorded = f.keeper.Name(), true
		}
		out = append(out, sf)
	}
	return out
}

func (st *staged) Diff() string {
	var b strings.Builder
	for _, f := range st.files {
		if f.diff != nil {
			b.WriteString(f.diff())
		}
	}
	return b.String()
}

// LockKeys are the lock files of the files the stage reads or changes.
func (st *staged) LockKeys() []string { return slices.Clone(st.keys) }

// Lock takes the lock on the lock file key. The first lock a stage takes is
// preceded by Options.BeforeSettle.
func (st *staged) Lock(ctx context.Context, key string) error {
	if _, ok := st.held[key]; ok {
		return nil
	}
	if !slices.Contains(st.keys, key) {
		return fmt.Errorf("lock %s: the change to %s takes no such lock", key, st.s.doc.Ref)
	}
	if len(st.held) == 0 && st.s.h.beforeSettle != nil {
		st.s.h.beforeSettle(st.s.doc.Ref)
	}
	l, err := st.s.h.openLock(key)
	if err != nil {
		return err
	}
	if err := l.Lock(ctx); err != nil {
		_ = l.Close()
		return err
	}
	if st.held == nil {
		st.held = map[string]*filelock.Lock{}
	}
	st.held[key] = l
	return nil
}

// lockAll takes every lock the stage needs that it does not hold, in key
// order.
func (st *staged) lockAll(ctx context.Context) error {
	for _, k := range st.keys {
		if err := st.Lock(ctx, k); err != nil {
			return err
		}
	}
	return nil
}

// Settle checks, with the lock of every file the stage touches held, that
// each file still has the digest the stage read. When one moved, the
// document and its editions are read again under the locks and the editor
// applied once more; a file that moves during that pass as well (an editor
// saving outside kapi, which takes no lock) is doc_changed.
func (st *staged) Settle(ctx context.Context) error {
	if err := st.lockAll(ctx); err != nil {
		return err
	}
	moved, err := st.moved(ctx)
	if err != nil {
		return err
	}
	if moved == nil {
		st.settled = true
		return nil
	}
	if err := st.run(ctx); err != nil {
		return err
	}
	if err := st.lockAll(ctx); err != nil {
		return err
	}
	if moved, err = st.moved(ctx); err != nil {
		return err
	}
	if moved != nil {
		return &change.Error{Code: change.CodeDocChanged,
			Message: moved.ref + " changed while the edit was applied again; read it and send the change again"}
	}
	st.settled = true
	return nil
}

func (st *staged) unlock() {
	for _, l := range st.held {
		l.Unlock()
		_ = l.Close()
	}
	st.held = nil
}

// moved returns the first file whose digest is no longer the one the stage
// read, or nil. A kept edition has moved when its head has.
func (st *staged) moved(ctx context.Context) (*stagedFile, error) {
	for _, f := range st.files {
		if f.keeper != nil {
			now, err := f.keeper.Edition(ctx, st.s.doc.Ref, *f.edition)
			if err != nil {
				return nil, err
			}
			if now.Token != f.token {
				return f, nil
			}
			continue
		}
		now, err := hashFile(f.path)
		if err != nil {
			return nil, err
		}
		if now != f.before {
			return f, nil
		}
	}
	return nil, nil
}

// Commit renames each staged file onto its target. An error leaves the files
// renamed before it written, which Files reports.
func (st *staged) Commit(ctx context.Context) error {
	if !st.settled {
		return fmt.Errorf("commit %s: the change was not settled", st.s.doc.Ref)
	}
	// A kept edition commits first: its keeper checks the head again inside
	// the log's transaction, and a refusal there leaves every file as it was.
	for _, f := range st.files {
		if f.kept == nil {
			continue
		}
		w := f.kept.write
		w.Record = st.rec
		id, err := f.kept.keeper.Commit(ctx, w)
		if err != nil {
			return err
		}
		f.kept = nil
		f.written = true
		if st.recordID == "" {
			st.recordID = id
		}
	}
	for _, f := range st.files {
		if f.tmp == nil {
			continue
		}
		if st.s.h.backup != "" && f.before != "" {
			if err := backUp(f.path, st.s.h.backup); err != nil {
				return err
			}
		}
		err := f.tmp.Commit()
		f.tmp = nil
		if err != nil {
			return err
		}
		f.written = true
	}
	return nil
}

// backUp copies the file at path beside it, with suffix appended to its name
// and its mode kept. It is called under the commit lock, once the file is
// known to hold what the stage read.
func backUp(path, suffix string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(path+suffix, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("write backup: %w", err)
	}
	return out.Close()
}

func (st *staged) Release() error {
	st.discard()
	st.unlock()
	st.settled = false
	return nil
}

// discard removes every staged temporary file.
func (st *staged) discard() {
	for _, f := range st.files {
		if f.tmp != nil {
			_ = f.tmp.Discard()
			f.tmp = nil
		}
	}
}

// diffLimit bounds the size of a file a preview renders as a diff.
const diffLimit = 1 << 20

// fileDiff renders the change from the file at before to the file at after.
func fileDiff(before, after, label string) string {
	a, err := readLimited(before)
	if errors.Is(err, fs.ErrNotExist) {
		// The change creates the file.
		a, err = nil, nil
	}
	if err != nil {
		return ""
	}
	b, err := readLimited(after)
	if err != nil {
		return ""
	}
	return textDiff(a, b, label)
}

func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, diffLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > diffLimit {
		return nil, fmt.Errorf("%s is larger than a preview renders", path)
	}
	return data, nil
}

// textDiff is a unified diff of two texts, or "" when either is not text.
func textDiff(a, b []byte, label string) string {
	if bytes.Equal(a, b) || !isText(a) || !isText(b) {
		return ""
	}
	d, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A: difflib.SplitLines(string(a)), B: difflib.SplitLines(string(b)),
		FromFile: "a/" + label, ToFile: "b/" + label, Context: 3,
	})
	if err != nil {
		return ""
	}
	return d
}

func isText(b []byte) bool {
	return len(b) <= diffLimit && utf8.Valid(b) && bytes.IndexByte(b, 0) < 0
}
