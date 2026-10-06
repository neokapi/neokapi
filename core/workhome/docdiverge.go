package workhome

import (
	"context"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/workspace"
)

// A document the home keeps whole diverges when two writers wrote it from one
// head and their logs met: the write that sorts later is listed beside the
// head (DocHead.Divergent) and a read reports it (change.Page.Divergent). A
// person or an agent settles it in one of two ways, each recorded as a write
// that names the divergent write as its cause:
//
//   - Rebase reads the document at the head again and applies the divergent
//     write's changes to it through the change service, block by block, each
//     guarded by the revision the block had where the write began. A block
//     the head changed too stays contested and is listed on the divergent
//     write, for a person to decide with an ordinary change set guarded by
//     the head's revision; keeping the head's wording decides it too
//     (change.ContestedSession).
//   - Discard drops the divergent write and keeps the head as it stands.

// ErrNotDivergent is what Rebase and Discard return for a write that is not
// listed beside its document's head: it advanced the head, a rebase or a
// discard settled it, or the log holds no such write.
var ErrNotDivergent = errors.New("workhome: the write is not divergent on its document")

// Divergences lists the head of every document the home keeps whole that
// holds a write that did not land, with the projection caught up first.
func (d *Documents) Divergences(ctx context.Context) ([]DocHead, error) {
	if err := d.Log.CatchUp(ctx); err != nil {
		return nil, err
	}
	heads, err := d.Store.Documents(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(heads, func(h DocHead) bool { return len(h.Divergent) == 0 }), nil
}

// DivergentWording is what a divergent write left in one edition of one
// block: its runs, or Absent when it left none.
type DivergentWording struct {
	Runs   []model.Run
	Absent bool
}

// DivergentWording reads, for each of blocks, what the divergent write op to
// document key left in it.
func (d *Documents) DivergentWording(ctx context.Context, key, op string, blocks []DocBlock) (map[DocBlock]DivergentWording, error) {
	w, _, err := d.writeOf(ctx, key, op)
	if err != nil {
		return nil, err
	}
	data, err := d.Log.Blob(ctx, w.Blob)
	if err != nil {
		return nil, fmt.Errorf("workhome: read %s: %w", op, err)
	}
	read, _, err := d.blocksOf(ctx, key, w.Format, data)
	if err != nil {
		return nil, err
	}
	out := make(map[DocBlock]DivergentWording, len(blocks))
	for _, db := range blocks {
		b := read[db.Block]
		if b == nil {
			out[db] = DivergentWording{Absent: true}
			continue
		}
		ed, ok := b.Edition(d.editionKey(db.Edition))
		if !ok {
			out[db] = DivergentWording{Absent: true}
			continue
		}
		out[db] = DivergentWording{Runs: ed.Runs}
	}
	return out, nil
}

// DocRebase is what a rebase of a divergent write did.
type DocRebase struct {
	// Carried counts the operations applied to the head, and Result is the
	// change service's result for them; nil when the head already held every
	// change the write made, or the head changed each block the write did.
	Carried int
	Result  *change.Result
	// Contested lists the blocks the head changed too, which a person
	// decides; empty when the rebase settled the write.
	Contested []DocBlock
}

// Rebase carries the divergent write op to document key over onto the head:
// the head is read again, and each change the write made, block by block and
// edition by edition, is sent through svc as an operation guarded by the
// revision the block had where the write began, as actor. A block the head
// changed too is left contested. Once the operations land, a write that
// names op as its cause records the blocks left contested, or settles the
// write when none is. A change set the service refuses (a stale block, a
// failing gate) is returned in the result and settles nothing.
//
// svc routes the document's reference (Prefix+key) to this home. A write a
// rebase has carried over already is not carried again; its contested blocks
// are returned.
func (d *Documents) Rebase(ctx context.Context, svc *change.Service, key, op string, actor change.Actor, origin string) (DocRebase, error) {
	w, div, err := d.writeOf(ctx, key, op)
	if err != nil {
		return DocRebase{}, err
	}
	if div.Rebased() {
		return DocRebase{Contested: div.Contested}, nil
	}
	head, headData, found, err := d.Head(ctx, key)
	if err != nil {
		return DocRebase{}, err
	}
	if !found {
		return DocRebase{}, &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: "no document " + d.Prefix + key}
	}
	afterData, err := d.Log.Blob(ctx, w.Blob)
	if err != nil {
		return DocRebase{}, fmt.Errorf("workhome: read %s: %w", op, err)
	}
	after, afterOrder, err := d.blocksOf(ctx, key, w.Format, afterData)
	if err != nil {
		return DocRebase{}, err
	}
	base, baseOrder, err := d.baseOf(ctx, key, w)
	if err != nil {
		return DocRebase{}, err
	}
	held, _, err := d.blocksOf(ctx, key, head.Format, headData)
	if err != nil {
		return DocRebase{}, err
	}

	ref := d.Prefix + key
	var ops []change.Op
	var contested []DocBlock
	contest := func(db DocBlock) {
		if !slices.Contains(contested, db) {
			contested = append(contested, db)
		}
	}
	for _, k := range afterOrder {
		a, b, h := after[k], base[k], held[k]
		if b == nil {
			// A block the write added, or a write whose base the log does not
			// hold: each edition the head does not hold as the write left it
			// is contested.
			for _, ek := range a.EditionKeys() {
				if h == nil || model.EditionRevision(h, ek) != model.EditionRevision(a, ek) {
					contest(DocBlock{Block: k, Edition: d.editionName(ek)})
				}
			}
			continue
		}
		for _, o := range change.Diff(b, a) {
			db := DocBlock{Block: k, Edition: d.editionName(o.At.Edition)}
			if h == nil {
				contest(db)
				continue
			}
			switch now := model.EditionRevision(h, o.At.Edition); now {
			case o.IfMatch:
				o.At.Doc = ref
				ops = append(ops, o)
			case model.EditionRevision(a, o.At.Edition):
				// The head holds what the write left.
			default:
				contest(db)
			}
		}
	}
	for _, k := range baseOrder {
		if after[k] == nil && held[k] != nil {
			// A block the write removed and the head still holds.
			contest(DocBlock{Block: k})
		}
	}

	out := DocRebase{Contested: contested}
	if len(ops) > 0 {
		res, err := svc.Apply(ctx, change.Set{Note: "Rebase the write " + op + " onto the document as it stands", Ops: ops}, actor)
		if err != nil {
			return DocRebase{}, err
		}
		out.Carried, out.Result = len(ops), res
		if res.Status != change.SetApplied {
			return out, nil
		}
	}
	if err := d.settle(ctx, key, op, contested, actor, origin); err != nil {
		return DocRebase{}, err
	}
	return out, nil
}

// Discard drops the divergent write op to document key, keeping the head as
// it stands, as actor through origin. It discards a rebased write's
// contested blocks the same way: the head keeps its wording in each.
func (d *Documents) Discard(ctx context.Context, key, op string, actor change.Actor, origin string) error {
	if _, _, err := d.writeOf(ctx, key, op); err != nil {
		return err
	}
	return d.settle(ctx, key, op, nil, actor, origin)
}

// settle records a write naming the divergent write op as its cause, with
// the blocks a rebase left contested: the head's bytes again, staged on the
// head, so the head stays where it is.
func (d *Documents) settle(ctx context.Context, key, op string, contested []DocBlock, actor change.Actor, origin string) error {
	for attempt := 0; ; attempt++ {
		seq, err := d.Log.DocumentSubjectHead(ctx, key)
		if err != nil {
			return err
		}
		head, data, found, err := d.Head(ctx, key)
		if err != nil {
			return err
		}
		if !found || !slices.ContainsFunc(head.Divergent, func(dv DocDivergence) bool { return dv.Op == op }) {
			return ErrNotDivergent
		}
		_, err = d.Log.CommitDocument(ctx, DocCommit{Key: key, Path: d.Prefix + key, Expect: seq, Base: head.Op,
			Format: head.Format, Data: data, Before: head.Rev, After: head.Rev, Actor: actor, Origin: origin,
			Cause: op, Contested: contested})
		if errors.Is(err, workspace.ErrHeadMoved) && attempt < 3 {
			continue
		}
		return err
	}
}

// writeOf finds the divergent write op to document key in the log, with its
// entry beside the head.
func (d *Documents) writeOf(ctx context.Context, key, op string) (DocWrite, DocDivergence, error) {
	if err := d.Log.CatchUp(ctx); err != nil {
		return DocWrite{}, DocDivergence{}, err
	}
	head, found, err := d.Store.Document(ctx, key)
	if err != nil {
		return DocWrite{}, DocDivergence{}, err
	}
	i := slices.IndexFunc(head.Divergent, func(dv DocDivergence) bool { return dv.Op == op })
	if !found || i < 0 {
		return DocWrite{}, DocDivergence{}, ErrNotDivergent
	}
	writes, err := d.Log.DocumentWrites(ctx, key)
	if err != nil {
		return DocWrite{}, DocDivergence{}, err
	}
	at := slices.IndexFunc(writes, func(w DocWrite) bool { return w.Op == op })
	if at < 0 {
		return DocWrite{}, DocDivergence{}, ErrNotDivergent
	}
	return writes[at], head.Divergent[i], nil
}

// baseOf reads the document as the divergent write w found it: the bytes of
// the write it was staged on. A write staged on no write, or on one the log
// does not hold, has no base, and every block it holds is compared with the
// head alone.
func (d *Documents) baseOf(ctx context.Context, key string, w DocWrite) (map[string]*model.Block, []string, error) {
	if w.Base == "" {
		return map[string]*model.Block{}, nil, nil
	}
	writes, err := d.Log.DocumentWrites(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	at := slices.IndexFunc(writes, func(b DocWrite) bool { return b.Op == w.Base })
	if at < 0 {
		return map[string]*model.Block{}, nil, nil
	}
	data, err := d.Log.Blob(ctx, writes[at].Blob)
	if err != nil {
		return nil, nil, fmt.Errorf("workhome: read %s: %w", w.Base, err)
	}
	return d.blocksOf(ctx, key, writes[at].Format, data)
}

// blocksOf reads data, a version of document key in format, as the change
// service reads the document: every block by the key a reference names it
// by, with the editions the document holds, and the keys in document order.
func (d *Documents) blocksOf(ctx context.Context, key, format string, data []byte) (map[string]*model.Block, []string, error) {
	sess, dir, err := d.workingCopy(ctx, key, format, data)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		_ = sess.Close()
		_ = os.RemoveAll(dir)
	}()
	out := map[string]*model.Block{}
	var order []string
	if _, err := sess.Read(ctx, change.Want{}, func(b *model.Block) error {
		k := change.BlockKey(b)
		if _, dup := out[k]; !dup {
			order = append(order, k)
		}
		out[k] = b.CopyEditionSet()
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return out, order, nil
}

// workingCopy writes data, a version of document key in format, to a working
// copy of its own and opens it through the file home, as Open does for the
// head. The caller closes the session and removes dir.
func (d *Documents) workingCopy(ctx context.Context, key, format string, data []byte) (change.Session, string, error) {
	if format == "" {
		return nil, "", &change.Error{Code: change.CodeUnsupported, Capability: "format", Message: "no format reads " + d.Prefix + key}
	}
	if err := os.MkdirAll(d.WorkDir, 0o700); err != nil {
		return nil, "", fmt.Errorf("workhome: %w", err)
	}
	dir, err := os.MkdirTemp(d.WorkDir, "doc-")
	if err != nil {
		return nil, "", fmt.Errorf("workhome: %w", err)
	}
	copyPath := filepath.Join(dir, pathpkg.Base(key))
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", fmt.Errorf("workhome: %w", err)
	}
	ref := d.Prefix + key
	located := filehome.Doc{Ref: ref, Path: copyPath, Format: filehome.RegistryBinding(d.Formats, format, ""),
		SourceLocale: d.SourceLocale, Editions: change.EditionsPerFile,
		NoEditionFile: "a document kept whole in the workspace holds its own edition and the translations its format holds in the document"}
	if info := d.Formats.FormatInfo(registry.FormatID(format)); info != nil && info.Interchange {
		located.Editions, located.TargetLocale = change.EditionsInFile, d.TargetLocale
	}
	inner := filehome.New(oneDoc{located}, filehome.Options{LockDir: filepath.Join(dir, "locks")})
	sess, err := inner.Open(ctx, ref)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", err
	}
	return sess, dir, nil
}

// editionName is the name a DocBlock gives edition k: empty for the
// document's own edition, the edition's text form for any other.
func (d *Documents) editionName(k model.EditionKey) string {
	if k.IsZero() || (d.SourceLocale != "" && k.Canonical() == model.Variant(d.SourceLocale).Canonical()) {
		return ""
	}
	return editionText(k)
}

// appendBlock appends b to blocks unless it is listed already.
func appendBlock(blocks []DocBlock, b DocBlock) []DocBlock {
	if slices.Contains(blocks, b) {
		return blocks
	}
	return append(blocks, b)
}

// editionKey is the edition a DocBlock's edition name names.
func (d *Documents) editionKey(name string) model.EditionKey {
	if name == "" {
		return model.EditionKey{}
	}
	k, err := model.ParseEditionKey(name)
	if err != nil {
		return model.EditionKey{}
	}
	return k
}
