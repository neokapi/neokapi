package host

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projector"
)

// The record of an applied edit.
//
// The change service hands every change set it applied to a recorder
// (change.Recorder) once the homes committed. This one records it in the
// workspace's log as one content.edit operation per document
// (projector.RecordEdit), which the projector writes into the block history:
// who changed which edition, from which revision to which, through which
// surface, under which governance.
//
// What a record keeps depends on who made the change. A person's or an
// agent's edit keeps the runs around each change and the change set as sent,
// which review, a revert by session and "who wrote this" read back. A tool's
// edit, which a flow makes by the thousand, keeps the revisions and hashes
// only: the file holds the text.

// EditRecorder returns the recorder for the project rooted at root. It
// records through the project's projector, so an edit reaches the same log
// and the same context store as the project's decisions.
func (a *App) EditRecorder(ctx context.Context, root string) (change.Recorder, error) {
	p, err := a.Projector(ctx, root)
	if err != nil {
		return nil, err
	}
	return &editRecorder{app: a, root: root, p: p}, nil
}

type editRecorder struct {
	app  *App
	root string
	p    *projector.Projector
}

var _ change.Recorder = (*editRecorder)(nil)

// Record records rec, one operation per document it changed, in the order the
// record lists them, and returns the id of the first. The others name the
// same change set.
func (r *editRecorder) Record(ctx context.Context, rec change.Record) (string, error) {
	keepRuns := rec.Actor.Kind == change.ActorPerson || rec.Actor.Kind == change.ActorAgent
	var setJSON []byte
	if keepRuns && rec.Set != nil {
		var err error
		if setJSON, err = json.Marshal(rec.Set); err != nil {
			return "", fmt.Errorf("record edit: encode the change set: %w", err)
		}
	}
	note := ""
	if rec.Set != nil {
		note = rec.Set.Note
	}

	var order []string
	results := map[string]change.DocResult{}
	byDoc := map[string][]change.Transition{}
	for _, d := range rec.Docs {
		if _, seen := results[d.Doc]; !seen {
			order = append(order, d.Doc)
		}
		results[d.Doc] = d
	}
	for _, t := range rec.Transitions {
		if _, listed := results[t.Ref.Doc]; !listed {
			if _, seen := byDoc[t.Ref.Doc]; !seen {
				order = append(order, t.Ref.Doc)
			}
		}
		byDoc[t.Ref.Doc] = append(byDoc[t.Ref.Doc], t)
	}

	docs := r.app.documentIndexOrEmpty(ctx, r.root)
	var first string
	for _, doc := range order {
		transitions := byDoc[doc]
		if len(transitions) == 0 {
			continue
		}
		res := results[doc]
		e := projector.Edit{
			Doc:         projector.EditDoc{Key: docs.Key(doc), Path: doc},
			Home:        res.Home,
			Actor:       rec.Actor,
			Origin:      projector.Origin{By: rec.Origin},
			Fingerprint: rec.Fingerprint,
			Note:        note,
			DocBefore:   res.Before,
			Overridden:  overriddenIn(doc, rec.Overridden),
			SetJSON:     setJSON,
		}
		if res.After != nil {
			e.DocAfter = *res.After
		}
		for _, t := range transitions {
			e.Transitions = append(e.Transitions, editTransition(t, keepRuns))
		}
		id, err := r.p.RecordEdit(ctx, e)
		if err != nil {
			return first, fmt.Errorf("record edit of %s: %w", doc, err)
		}
		if first == "" {
			first = id
		}
	}
	return first, nil
}

// editTransition renders one recorded transition. Its identity evidence comes
// from the block after the change where the transition carries none, so every
// record can re-attach history after a reorder.
func editTransition(t change.Transition, keepRuns bool) projector.EditTransition {
	edition := t.Ref.EditionText()
	key, contentHash, contextHash := t.Key, t.ContentHash, t.ContextHash
	if b := t.Block; b != nil {
		if text, err := b.EditionKeyOf(t.Ref.Edition).MarshalText(); err == nil && len(text) > 0 {
			edition = string(text)
		}
		if key == "" {
			key = b.Unit
		}
		if contentHash == "" || contextHash == "" {
			id := model.ComputeIdentity(b)
			if contentHash == "" {
				contentHash = id.ContentHash
			}
			if contextHash == "" {
				contextHash = id.ContextHash
			}
		}
	}
	out := projector.EditTransition{
		Block: t.Ref.Block, Key: key, Edition: edition,
		Before: t.BeforeRev, After: t.AfterRev, Basis: t.Basis,
		ContentHash: contentHash, ContextHash: contextHash,
	}
	if keepRuns {
		out.BeforeRuns, out.AfterRuns = t.Before, t.After
	}
	return out
}

// overriddenIn returns the overridden findings that concern one document: the
// ones placed in it, and the ones placed nowhere.
func overriddenIn(doc string, findings []change.Finding) []change.Finding {
	var out []change.Finding
	for _, f := range findings {
		if f.At == nil || f.At.Doc == doc {
			out = append(out, f)
		}
	}
	return out
}
