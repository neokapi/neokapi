package projector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/core/workspace"
)

// The workspace home keeps the editions of a project's documents that have
// no file yet (core/workhome). Each write to one is a content.edit operation
// that carries the result and names the edition as its subject, appended
// only while the edition's head is still the one its writer read. The
// projector folds those operations, beside the block history, into the
// workspace home's tables: in id order per edition, so every machine whose
// log has been merged reaches the same head.

var _ workhome.Log = (*Projector)(nil)

// errNoWorkspaceLog is what a write to the workspace home meets on a store
// with no log: the log is where the workspace home keeps its editions.
var errNoWorkspaceLog = errors.New("projector: the workspace home keeps its editions in the workspace's log, and this project store has none (the workspace is read-only, or the store is embedded)")

// CommitWorkspace records a write to an edition the workspace home keeps as
// one content.edit operation carrying the result, appended only while the
// edition's head is still at c.Expect, and folds it into the projection.
func (p *Projector) CommitWorkspace(ctx context.Context, c workhome.Commit) (string, error) {
	if p.log == nil {
		return "", errNoWorkspaceLog
	}
	if p.st.Heads == nil {
		return "", errNoSubsystem
	}
	if c.Doc == "" || c.Edition == "" {
		return "", errors.New("projector: a write to the workspace home names its document and edition")
	}
	if len(c.Blocks) == 0 {
		return "", errors.New("projector: a write to the workspace home with no block records nothing")
	}
	e := Edit{
		Doc: EditDoc{Key: c.Doc, Path: c.Path}, Home: workhome.Name, Edition: c.Edition, Base: c.Base, Cause: c.Cause,
		Release: c.Release, Actor: c.Actor, Origin: Origin{By: c.Origin}, Fingerprint: c.Fingerprint, Note: c.Note,
		DocBefore: c.DocBefore, DocAfter: c.DocAfter, Overridden: c.Overridden,
	}
	byWriter := c.Actor.Kind == change.ActorPerson || c.Actor.Kind == change.ActorAgent
	if byWriter && c.Set != nil {
		data, err := json.Marshal(c.Set)
		if err != nil {
			return "", fmt.Errorf("projector: encode the change set: %w", err)
		}
		e.SetJSON = data
	}
	for _, b := range c.Blocks {
		t := EditTransition{Block: b.Block, Key: b.Key, Edition: c.Edition, Before: b.Before, After: b.After, Basis: b.Basis,
			ContentHash: b.ContentHash, ContextHash: b.ContextHash, Stamp: b.Stamp}
		if byWriter {
			t.BeforeRuns = b.BeforeRuns
		}
		if b.Edition != nil {
			// The log is the edition's home, so the record keeps the result
			// whoever made it.
			t.AfterRuns = b.Edition.Runs
			if t.AfterRuns == nil {
				t.AfterRuns = []model.Run{}
			}
			t.Status = b.Edition.Status
			if o := b.Edition.Origin; o != (model.Origin{}) {
				t.Origin = &o
				if !byWriter {
					t.Producer = &o
				}
			}
		}
		e.Transitions = append(e.Transitions, t)
	}
	expect := []workspace.Expect{{Project: p.key, Subject: workhome.Subject(c.Doc, c.Edition), Head: c.Expect}}
	ids, err := p.recordEdits(ctx, []Edit{e}, expect)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// SubjectHead is the local position of the latest operation the log holds on
// one edition the workspace home keeps, zero for none.
func (p *Projector) SubjectHead(ctx context.Context, doc, edition string) (int64, error) {
	if p.log == nil {
		return 0, nil
	}
	return p.log.SubjectHead(ctx, p.key, workhome.Subject(doc, edition))
}

// Blob reads a blob from the log.
func (p *Projector) Blob(ctx context.Context, address string) ([]byte, error) {
	if p.log == nil {
		return nil, errNoWorkspaceLog
	}
	return p.log.Blob(ctx, address)
}

// workWrite is the workspace home's reading of a write to an edition it
// keeps: the head the write was staged on and each block's edition as the
// write left it, the runs inline where they are small enough to keep so.
func (p *Projector) workWrite(ctx context.Context, op workspace.Op, e Edit) (workhome.Write, error) {
	w := workhome.Write{Op: op.ID, Doc: e.Doc.Key, Edition: e.Edition, Path: e.Doc.Path, Base: e.Base, Cause: e.Cause,
		Writer:  e.Actor.Kind == change.ActorPerson || e.Actor.Kind == change.ActorAgent,
		Release: e.Release}
	for _, t := range e.Transitions {
		b := workhome.BlockWrite{Block: t.Block, Before: t.Before, After: t.After, Basis: t.Basis, Status: t.Status, Stamp: t.Stamp}
		if t.Origin != nil {
			b.Origin = *t.Origin
		}
		if t.After != model.AbsentRevision {
			b.Ref, _ = BlobAddress(t.RunsAfter)
			switch {
			case t.AfterRuns != nil:
				b.Runs = model.CanonicalRunsJSON(t.AfterRuns)
			case b.Ref != "":
				data, err := p.log.Blob(ctx, b.Ref)
				if err != nil {
					return workhome.Write{}, fmt.Errorf("projector: read the runs of %s in %s: %w", t.Block, workspace.ShortOpID(op.ID), err)
				}
				if len(data) <= workhome.InlineRuns {
					b.Runs = data
				}
			}
		}
		w.Blocks = append(w.Blocks, b)
	}
	return w, nil
}

// applyWorkWrites folds writes into the workspace home's projection, folding
// every edition a write arrived out of order for again from all the writes
// the log holds for it.
func (p *Projector) applyWorkWrites(ctx context.Context, writes []workhome.Write) error {
	if len(writes) == 0 || p.st.Heads == nil {
		return nil
	}
	refold, err := p.st.Heads.Apply(ctx, writes)
	if err != nil {
		return err
	}
	for _, key := range refold {
		if err := p.refold(ctx, key[0], key[1]); err != nil {
			return err
		}
	}
	return nil
}

// refold folds one edition again from every write the log holds for it.
func (p *Projector) refold(ctx context.Context, doc, edition string) error {
	writes, err := p.Writes(ctx, doc, edition)
	if err != nil {
		return err
	}
	h, rows := workhome.Fold(writes)
	h.Doc, h.Edition = doc, edition
	return p.st.Heads.Replace(ctx, h, rows)
}

// Writes reads every write the log holds to one edition the workspace home
// keeps, in the log's order, as the fold reads them.
func (p *Projector) Writes(ctx context.Context, doc, edition string) ([]workhome.Write, error) {
	if p.log == nil {
		return nil, nil
	}
	ops, err := p.log.Select(ctx, workspace.OpQuery{Project: p.key, Subject: workhome.Subject(doc, edition)})
	if err != nil {
		return nil, err
	}
	var writes []workhome.Write
	for _, op := range ops {
		if op.Kind != KindEdit {
			continue
		}
		e, err := p.decodeEdit(ctx, op)
		if err != nil {
			continue
		}
		if !e.kept() {
			continue
		}
		w, err := p.workWrite(ctx, op, e)
		if err != nil {
			return nil, err
		}
		writes = append(writes, w)
	}
	return writes, nil
}

// RebaseWorkspace carries over the writes to editions the workspace home
// keeps that a merge left divergent: a write two machines made from one head
// does not advance it on the machine whose write sorts later, and when every
// block that write changed still holds the revision it started from, which is
// the common case for writes to different blocks, a new content.edit applies
// it onto the head and names it as its cause. It returns how many writes it
// carried over. A write a block of which has moved since stays divergent and
// is listed by workhome.Home.Conflicts.
func (p *Projector) RebaseWorkspace(ctx context.Context) (int, error) {
	if p.log == nil || p.st.Heads == nil {
		return 0, nil
	}
	if err := p.CatchUp(ctx); err != nil {
		return 0, err
	}
	heads, err := p.st.Heads.Heads(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, head := range heads {
		for _, d := range head.Divergent {
			done, err := p.rebase(ctx, head.Doc, head.Edition, d)
			if err != nil {
				return n, err
			}
			if done {
				n++
			}
		}
	}
	return n, nil
}

// rebase carries one divergent write over onto its edition's head, when
// every block it changed still holds the revision it started from. Only the
// blocks the head still lists for it are carried: a person's or an agent's
// write since has decided the others. A release that did not land is never
// carried over; the delivery that made it handles what the head holds.
func (p *Projector) rebase(ctx context.Context, doc, edition string, d workhome.Divergence) (bool, error) {
	if d.Release {
		return false, nil
	}
	seq, err := p.SubjectHead(ctx, doc, edition)
	if err != nil {
		return false, err
	}
	if err := p.CatchUp(ctx); err != nil {
		return false, err
	}
	head, _, err := p.st.Heads.Head(ctx, doc, edition)
	if err != nil {
		return false, err
	}
	rows, err := p.st.Heads.Rows(ctx, doc, edition)
	if err != nil {
		return false, err
	}
	if workhome.Settled(d, rows) || !workhome.Rebaseable(d, rows) {
		return false, nil
	}
	ops, err := p.log.Select(ctx, workspace.OpQuery{Project: p.key, Subject: workhome.Subject(doc, edition)})
	if err != nil {
		return false, err
	}
	at := slices.IndexFunc(ops, func(op workspace.Op) bool { return op.ID == d.Op })
	if at < 0 {
		return false, nil
	}
	e, err := p.decodeEdit(ctx, ops[at])
	if err != nil {
		return false, err
	}
	listed := make(map[string]bool, len(d.Blocks))
	for _, m := range d.Blocks {
		listed[m.Block] = true
	}
	re := e
	re.Base, re.Cause = head.Op, d.Op
	re.Origin = Origin{By: "rebase"}
	re.Transitions = slices.DeleteFunc(slices.Clone(e.Transitions), func(t EditTransition) bool { return !listed[t.Block] })
	re.Blobs = slices.Clone(e.Blobs)
	re.SetJSON = nil
	if len(re.Transitions) == 0 {
		return false, nil
	}
	after := maps.Clone(rows)
	for _, t := range re.Transitions {
		if t.After == model.AbsentRevision {
			delete(after, t.Block)
			continue
		}
		r := workhome.Row{Rev: t.After, Status: t.Status}
		if t.Origin != nil {
			r.Origin = *t.Origin
		}
		after[t.Block] = r
	}
	re.DocBefore, re.DocAfter = workhome.Digest(rows), workhome.Digest(after)
	expect := []workspace.Expect{{Project: p.key, Subject: workhome.Subject(doc, edition), Head: seq}}
	if _, err := p.recordEdits(ctx, []Edit{re}, expect); err != nil {
		if errors.Is(err, workspace.ErrHeadMoved) {
			// Another writer moved the edition first; the next rebase reads
			// the head it left.
			return false, nil
		}
		return false, err
	}
	return true, nil
}
