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
}

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
		if f, ok := s.editionFile(k); ok {
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

	// What the editor changed in each joined edition, by document block index.
	changed := make([]map[int][]model.Run, len(editions))
	for i := range changed {
		changed[i] = map[int][]model.Run{}
	}
	ownChanged := false
	si := 0
	edit := func(b *model.Block) error {
		join(editions, si, b)
		keys, err := st.e.Edit(b)
		for _, k := range keys {
			at := slices.IndexFunc(editions, func(je *joinedEdition) bool { return je.key == k.Canonical() })
			if at < 0 {
				ownChanged = true
				continue
			}
			ed, _ := b.Edition(editions[at].key)
			changed[at][si] = ed.Runs
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
	if data, ok := s.overlay[overlayKey(source{path: s.doc.Path, entry: s.doc.Entry})]; ok && own.tmp == nil {
		// Blocks added or removed are the whole change to the file: the
		// bytes the format wrote for them are what lands.
		if own.tmp, own.after, own.diff, err = st.writeBytes(ctx, s.doc.Path, s.ownSource(), data); err != nil {
			return err
		}
	}

	for i, je := range editions {
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
		if err := st.writeEdition(ctx, f, je, ix, changed[i]); err != nil {
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
// gave it. A file that exists is edited through its own skeleton, so every
// byte of it outside the changed blocks stays, and each changed block must
// have a partner there; a write that would need a block the file does not
// hold is refused, because adding one means rewriting the file. A file that
// does not exist yet is materialized from the document's skeleton, as kapi
// merge writes a target file, and so is every file under
// Options.Materialize, except a bilingual file that still holds every block
// of the document and nothing else: it keeps its own skeleton, so its header
// (its language, its plural rule) and its comments stay.
func (st *staged) writeEdition(ctx context.Context, f *stagedFile, je *joinedEdition, ix *blockIndex, changed map[int][]model.Run) error {
	s := st.s
	inPlace := je.exists && !s.h.materialize
	if je.exists && s.h.materialize && je.file.Bilingual && !je.drifted(ix) {
		inPlace = true
	}
	if inPlace {
		return st.writeEditionInPlace(ctx, f, je, ix, changed)
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
				if ok {
					b.SetEdition(je.key, model.Edition{Runs: runs})
				}
				si++
				return nil
			}}.run(ctx)
	})
	return err
}

// writeEditionInPlace stages the existing file of a joined edition through
// its own skeleton, with each changed block written into its partner there.
// A changed block with no partner refuses the write.
func (st *staged) writeEditionInPlace(ctx context.Context, f *stagedFile, je *joinedEdition, ix *blockIndex, changed map[int][]model.Run) error {
	s := st.s
	byTarget := map[int][]model.Run{}
	var unpaired []string
	for si, runs := range changed {
		ti, ok := je.match[si]
		if !ok {
			unpaired = append(unpaired, ix.keys[si])
			continue
		}
		byTarget[ti] = runs
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
				if runs, ok := byTarget[ti]; ok {
					ed, _ := b.Edition(key)
					ed.Runs = runs
					b.SetEdition(key, ed)
				}
				ti++
				return nil
			}}.run(ctx)
	})
	return err
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
		out = append(out, change.StagedFile{File: f.ref, Edition: f.edition, Before: f.before, After: f.after, Written: f.written})
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
	moved, err := st.moved()
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
	if moved, err = st.moved(); err != nil {
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
// read, or nil.
func (st *staged) moved() (*stagedFile, error) {
	for _, f := range st.files {
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
func (st *staged) Commit(context.Context) error {
	if !st.settled {
		return fmt.Errorf("commit %s: the change was not settled", st.s.doc.Ref)
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
