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
	locks []*filelock.Lock
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

	editions, _, err := s.joinEditions(ctx, st.want.Editions)
	if err != nil {
		return err
	}
	digests := make([]string, len(editions))
	for i, je := range editions {
		if digests[i], err = hashFile(je.src.path); err != nil {
			return err
		}
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
	src := s.ownSource()
	if st.want.Own {
		own.tmp, own.after, own.diff, err = st.write(ctx, s.doc.Path, src, func(out io.Writer) error {
			return pass{src: src, format: s.doc.Format, locale: s.doc.SourceLocale, encoding: s.doc.Encoding, fn: edit, out: out}.run(ctx)
		})
	} else {
		err = s.readPass(src, s.doc.Format, edit).run(ctx)
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

	for i, je := range editions {
		f := &stagedFile{ref: je.file.Ref, edition: &je.key, path: je.file.Path, before: digests[i], after: digests[i]}
		st.files = append(st.files, f)
		if len(changed[i]) == 0 {
			continue
		}
		if err := st.writeEdition(ctx, f, je, changed[i]); err != nil {
			return err
		}
	}
	return nil
}

// writeEdition stages the file of a joined edition with the runs the editor
// gave it. A file that exists and holds a block for every changed one is
// edited through its own skeleton; otherwise it is materialized from the
// document's skeleton, every edition the file held kept as content, as kapi
// merge writes a target file.
func (st *staged) writeEdition(ctx context.Context, f *stagedFile, je *joinedEdition, changed map[int][]model.Run) error {
	s := st.s
	inPlace := je.exists
	byTarget := map[int][]model.Run{}
	for si, runs := range changed {
		ti, ok := je.match[si]
		if !ok {
			inPlace = false
			break
		}
		byTarget[ti] = runs
	}
	var err error
	if inPlace {
		src := je.src
		ti := 0
		f.tmp, f.after, f.diff, err = st.write(ctx, je.file.Path, src, func(out io.Writer) error {
			return pass{src: src, format: je.file.Format, locale: s.doc.SourceLocale, encoding: s.doc.Encoding, out: out,
				fn: func(b *model.Block) error {
					if runs, ok := byTarget[ti]; ok {
						ed, _ := b.Edition(model.EditionKey{})
						ed.Runs = runs
						b.SetEdition(model.EditionKey{}, ed)
					}
					ti++
					return nil
				}}.run(ctx)
		})
		return err
	}
	if err := os.MkdirAll(filepath.Dir(je.file.Path), 0o755); err != nil {
		return err
	}
	src := s.ownSource()
	si := 0
	f.tmp, f.after, f.diff, err = st.write(ctx, je.file.Path, source{path: je.file.Path}, func(out io.Writer) error {
		return pass{src: src, format: s.doc.Format, locale: s.doc.SourceLocale, encoding: s.doc.Encoding, out: out,
			writeLocale: je.key.Locale, writerSource: src,
			fn: func(b *model.Block) error {
				runs, ok := changed[si]
				if !ok {
					if ti, held := je.match[si]; held {
						ed, _ := je.blocks[ti].Edition(model.EditionKey{})
						runs, ok = ed.Runs, true
					}
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

// write stages what produce writes as the new content of the file at path:
// for a plain file, the bytes themselves; for an archive member, the member,
// spliced into a copy of the archive with every other member as it was. It
// returns the staged file, its digest, and a renderer of the change.
func (st *staged) write(ctx context.Context, path string, src source, produce func(io.Writer) error) (*atomicfile.Staged, string, func() string, error) {
	h := sha256.New()
	if src.entry == "" {
		tmp, err := atomicfile.Stage(path, func(w io.Writer) error {
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
		before, berr := src.entryBytes()
		if berr != nil {
			return ""
		}
		return textDiff(before, edited, st.label(path)+"!"+src.entry)
	}, nil
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
		out = append(out, change.StagedFile{File: f.ref, Edition: f.edition, Before: f.before, After: f.after})
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

func (st *staged) LockKey() string {
	p, err := st.s.h.lockPath(st.s.doc.Path)
	if err != nil {
		return st.s.doc.Path
	}
	return p
}

// Settle takes the lock of every file the stage changes, in path order, and
// checks each still has the digest the stage read. When one moved, the
// document and its editions are read again under the locks and the editor
// applied once more; a file that moves during that pass as well (an editor
// saving outside kapi, which takes no lock) is doc_changed.
func (st *staged) Settle(ctx context.Context) error {
	if st.s.h.beforeSettle != nil {
		st.s.h.beforeSettle(st.s.doc.Ref)
	}
	if err := st.lock(ctx); err != nil {
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

// lock takes the lock of every file the stage reads or changes, in the order
// of their lock files.
func (st *staged) lock(ctx context.Context) error {
	if len(st.locks) > 0 {
		return nil
	}
	var paths []string
	for _, f := range st.files {
		p, err := st.s.h.lockPath(f.path)
		if err != nil {
			return err
		}
		if !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	for _, p := range paths {
		l, err := filelock.Open(p)
		if err != nil {
			st.unlock()
			return err
		}
		if err := l.Lock(ctx); err != nil {
			_ = l.Close()
			st.unlock()
			return err
		}
		st.locks = append(st.locks, l)
	}
	return nil
}

func (st *staged) unlock() {
	for _, l := range st.locks {
		l.Unlock()
		_ = l.Close()
	}
	st.locks = nil
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

func (st *staged) Commit(context.Context) error {
	if !st.settled {
		return fmt.Errorf("commit %s: the change was not settled", st.s.doc.Ref)
	}
	for _, f := range st.files {
		if f.tmp == nil {
			continue
		}
		if err := f.tmp.Commit(); err != nil {
			return err
		}
		f.tmp = nil
	}
	return nil
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
