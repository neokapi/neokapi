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
	si := 0
	p := s.ownPass(func(b *model.Block) error {
		for _, k := range removed[si] {
			if _, held := b.Edition(k); held && kept == nil {
				text, _ := k.Canonical().MarshalText()
				kept = &change.Error{Code: change.CodeUnsupported, Capability: string(change.KindRemoveEdition), Field: "at/edition",
					Message: fmt.Sprintf("the %s writer writes a translation of every unit, taking the source where a block holds none, so removing translation %s of block %s from %s would write its source in its place; give the translation new content with set_content instead",
						s.doc.Format.Name, text, change.BlockKey(b), s.doc.Ref)}
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
		return kept
	}
	return nil
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
// back (verifyEditionRemoved).
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
	if err != nil || len(gone) == 0 || !je.file.Bilingual {
		return err
	}
	// The file is written from the document's skeleton, so its blocks are in
	// the document's order.
	return st.verifyEditionRemoved(ctx, f, je, gone)
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
// A changed block with no partner refuses the write.
func (st *staged) writeEditionInPlace(ctx context.Context, f *stagedFile, je *joinedEdition, ix *blockIndex, changed map[int][]model.Run, gone map[int]bool) error {
	s := st.s
	byTarget := map[int][]model.Run{}
	goneAt := map[int]bool{}
	var unpaired []string
	for si, runs := range changed {
		ti, ok := je.match[si]
		if !ok {
			unpaired = append(unpaired, ix.keys[si])
			continue
		}
		byTarget[ti] = runs
		if gone[si] {
			goneAt[ti] = true
		}
	}
	if len(unpaired) > 0 {
		slices.Sort(unpaired)
		return &change.Error{Code: change.CodeUnsupported, Capability: "edition", Field: "at/block",
			Message: fmt.Sprintf("%s holds no block that pairs with %s of %s, and an edition is written only into a block its file already holds; add the block to %s first",
				je.file.Ref, blockList(unpaired), s.doc.Ref, je.file.Ref)}
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
				case ok:
					ed, _ := b.Edition(key)
					ed.Runs = runs
					b.SetEdition(key, ed)
				}
				ti++
				return nil
			}}.run(ctx)
	})
	if err != nil || len(goneAt) == 0 || !je.file.Bilingual {
		return err
	}
	return st.verifyEditionRemoved(ctx, f, je, goneAt)
}

// verifyEditionRemoved reads back the bilingual file of a joined edition as
// the stage wrote it, in the edition's language, and refuses the change when
// a block at one of the indexes at (in the file's block order) still holds the
// translation the editor removed: a writer that fills a unit with no
// translation from its source, as the XLIFF and TMX writers do, would replace
// the translation with the source rather than remove it.
func (st *staged) verifyEditionRemoved(ctx context.Context, f *stagedFile, je *joinedEdition, at map[int]bool) error {
	if f.tmp == nil {
		return nil
	}
	data, err := os.ReadFile(f.tmp.Name())
	if err != nil {
		return err
	}
	s := st.s
	var kept *change.Error
	i := 0
	p := pass{src: source{path: je.file.Path}.with(data), format: je.file.Format, locale: s.doc.SourceLocale, target: je.key.Locale,
		encoding: s.doc.Encoding, fn: func(b *model.Block) error {
			if _, held := b.Edition(je.key); held && at[i] && kept == nil {
				text, _ := je.key.MarshalText()
				kept = &change.Error{Code: change.CodeUnsupported, Capability: string(change.KindRemoveEdition), Field: "at/edition",
					Message: fmt.Sprintf("the %s writer writes a translation of every unit, taking the source where a block holds none, so removing translation %s of block %s from %s would write its source in its place; give the translation new content with set_content instead",
						je.file.Format.Name, text, change.BlockKey(b), je.file.Ref)}
			}
			i++
			return nil
		}}
	if err := p.run(ctx); err != nil {
		return err
	}
	if kept != nil {
		return kept
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
