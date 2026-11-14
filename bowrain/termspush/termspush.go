// Package termspush applies the terminology part of a push.
//
// A push carries the project's terminology edits beside its content
// (host/venue/client.TermsEdits): ordinary edits that apply directly, and
// governed ones bundled into one change-set for review. The commit handler
// checks them before it accepts the push (Validate, AssertRef), and the push
// worker applies them (Apply) on the transaction that stores the content, so
// a push lands whole or not at all.
//
// The rules are the ones the concept and change-set routes enforce: a direct
// edit may not make a governed transition (a term set to forbidden or
// preferred, a forbidden term removed, the do-not-translate flag changed, a
// REPLACED_BY relation), and a change-set from an origin supersedes the open
// one of that origin unless it holds the same operations.
package termspush

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/knowledge"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/terms"
)

// ErrGoverned refuses a direct edit that makes a governed transition. Such an
// edit travels in the proposal instead.
var ErrGoverned = errors.New("governed change requires a change-set")

// Validate checks what can be checked of edits without reading a store: no
// direct edit makes a governed transition the request itself shows, every
// relation names its target, and every proposed operation is well formed.
func Validate(edits *apiclient.TermsEdits) error {
	if edits.Empty() {
		return nil
	}
	for i, c := range edits.Creates {
		if c.DoNotTranslate {
			return fmt.Errorf("%w: concept %d is created with its do-not-translate flag set", ErrGoverned, i+1)
		}
		if terms.GovernedCreate(termsFromInfo(c.Terms)) {
			return fmt.Errorf("%w: concept %d is created with a forbidden or preferred term", ErrGoverned, i+1)
		}
	}
	for _, u := range edits.Updates {
		if u.ConceptID == "" {
			return errors.New("a concept edit names no concept")
		}
	}
	for _, a := range edits.RelationAdds {
		if a.SourceID == "" || a.Relation.TargetID == "" {
			return errors.New("a relation names no source or no target")
		}
		if a.Relation.RelationType == graph.LabelReplacedBy {
			return fmt.Errorf("%w: a REPLACED_BY relation from %s", ErrGoverned, a.SourceID)
		}
	}
	for _, r := range edits.RelationRemoves {
		if r.RelationID == "" {
			return errors.New("a relation removal names no relation")
		}
	}
	if p := edits.Proposal; p != nil && len(p.Ops) > 0 {
		for _, in := range p.Ops {
			if err := knowledge.ValidateOp(knowledge.ChangeSetOp{Op: knowledge.OpType(in.Op), Payload: in.Payload, BaseRev: in.BaseRev}); err != nil {
				return fmt.Errorf("proposed operation %q: %w", in.Op, err)
			}
		}
	}
	return nil
}

// AssertRef refuses a proposal whose operations were computed against
// terminology that has moved since: the terms component tb reads now must be
// the one the project last observed. A push that proposes nothing, or that
// observed nothing, asserts nothing.
func AssertRef(ctx context.Context, tb terms.Store, edits *apiclient.TermsEdits) error {
	if edits == nil || edits.Proposal == nil || len(edits.Proposal.Ops) == 0 ||
		edits.Proposal.ExpectedTermsRef == "" || tb == nil {
		return nil
	}
	current, err := bowsync.TermsComponentOf(ctx, tb)
	if err != nil {
		return err
	}
	return ref.Assert(ref.ComponentTerms, edits.Proposal.ExpectedTermsRef, current)
}

// Stores are the stores Apply writes, bound to the push's transaction.
type Stores struct {
	Terms     terms.Store
	Knowledge knowledge.Store
}

// streamer is a terms store that files a concept under a stream.
type streamer interface {
	AddConceptWithStream(ctx context.Context, concept terms.Concept, stream string) error
}

// Params name where the edits apply and who made them.
type Params struct {
	// WorkspaceID scopes the change-set; the terms store is the workspace's.
	WorkspaceID string
	// Stream is the stream the push targets. A concept created on a stream
	// other than main is filed under it.
	Stream string
	// Actor authored the edits, and OnBehalfOf names the person accountable
	// when Actor is a machine identity.
	Actor      string
	OnBehalfOf string
}

// Result is what Apply did: the outcome the push reports, the events to
// publish once the transaction commits, and a digest of the change-set it
// proposed, for those events.
type Result struct {
	Outcome *venue.PushTermsOutcome
	Events  []knowledge.MergeEvent
	Digest  *knowledge.ChangeSetDigest
	// Submitted is the change-set this push put in review, nil when it
	// proposed nothing or the one in review already held the same operations.
	Submitted *knowledge.ChangeSet
	// Governed reports whether Submitted touches governed state.
	Governed bool
}

// Apply writes edits through st. Every write runs on the stores' transaction,
// so an error leaves nothing for the caller to undo: it rolls back.
func Apply(ctx context.Context, st Stores, p Params, edits *apiclient.TermsEdits) (*Result, error) {
	res := &Result{Outcome: &venue.PushTermsOutcome{}}
	if edits.Empty() {
		return res, nil
	}
	if st.Terms == nil {
		return nil, errors.New("terms: the workspace's terms store is unavailable")
	}
	if err := Validate(edits); err != nil {
		return nil, err
	}
	if err := AssertRef(ctx, st.Terms, edits); err != nil {
		return nil, err
	}
	event := func(t knowledge.EventType, conceptID string) {
		res.Events = append(res.Events, knowledge.MergeEvent{
			Type: t, WorkspaceID: p.WorkspaceID, ConceptID: conceptID, Actor: p.Actor, OnBehalfOf: p.OnBehalfOf,
		})
	}
	now := time.Now()

	for _, c := range edits.Creates {
		concept := terms.Concept{
			ID:         id.New(),
			ProjectID:  c.ProjectID,
			Domain:     c.Domain,
			Definition: c.Definition,
			Terms:      termsFromInfo(c.Terms),
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		var err error
		if s, ok := st.Terms.(streamer); ok && p.Stream != "" && p.Stream != "main" {
			err = s.AddConceptWithStream(ctx, concept, p.Stream)
		} else {
			err = st.Terms.AddConcept(ctx, concept)
		}
		if err != nil {
			return nil, fmt.Errorf("create concept: %w", err)
		}
		res.Outcome.ConceptsApplied++
		event(knowledge.EventConceptCreated, concept.ID)
	}

	for _, u := range edits.Updates {
		existing, ok, err := st.Terms.GetConcept(ctx, u.ConceptID)
		if err != nil {
			return nil, fmt.Errorf("read concept %s: %w", u.ConceptID, err)
		}
		if !ok {
			return nil, fmt.Errorf("edit concept %s: the workspace holds no such concept", u.ConceptID)
		}
		next := termsFromInfo(u.Concept.Terms)
		if terms.GovernedUpdate(existing.Terms, next) {
			return nil, fmt.Errorf("%w: concept %s: a term status transition to or from forbidden or preferred", ErrGoverned, u.ConceptID)
		}
		if u.Concept.DoNotTranslate != nil && *u.Concept.DoNotTranslate != existing.DoNotTranslate {
			return nil, fmt.Errorf("%w: concept %s: its do-not-translate flag", ErrGoverned, u.ConceptID)
		}
		existing.Domain = u.Concept.Domain
		existing.Definition = u.Concept.Definition
		existing.Terms = next
		existing.UpdatedAt = now
		if err := st.Terms.AddConcept(ctx, existing); err != nil {
			return nil, fmt.Errorf("edit concept %s: %w", u.ConceptID, err)
		}
		res.Outcome.ConceptsApplied++
		event(knowledge.EventConceptUpdated, u.ConceptID)
	}

	for _, a := range edits.RelationAdds {
		rel := terms.ConceptRelation{
			ID:           id.New(),
			SourceID:     a.SourceID,
			TargetID:     a.Relation.TargetID,
			RelationType: a.Relation.RelationType,
			Note:         a.Relation.Note,
			Validity:     a.Relation.Validity,
			CreatedAt:    now,
		}
		if err := st.Terms.AddRelation(ctx, rel); err != nil {
			return nil, fmt.Errorf("add relation from %s: %w", a.SourceID, err)
		}
		res.Outcome.RelationsApplied++
		event(knowledge.EventConceptRelationAdded, a.SourceID)
	}

	for _, r := range edits.RelationRemoves {
		if err := st.Terms.DeleteRelation(ctx, r.RelationID); err != nil {
			return nil, fmt.Errorf("remove relation %s: %w", r.RelationID, err)
		}
		res.Outcome.RelationsApplied++
		event(knowledge.EventConceptRelationRemoved, r.SourceID)
	}

	if edits.Proposal != nil && len(edits.Proposal.Ops) > 0 {
		if err := propose(ctx, st.Knowledge, p, edits.Proposal, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// propose records the governed operations as one change-set and submits it.
func propose(ctx context.Context, ks knowledge.Store, p Params, in *apiclient.TermsProposal, res *Result) error {
	if ks == nil {
		return errors.New("terms: governed edits need the workspace's knowledge store, which is unavailable")
	}
	if p.WorkspaceID == "" {
		return errors.New("terms: governed edits need a workspace")
	}
	cs := &knowledge.ChangeSet{
		WorkspaceID: p.WorkspaceID,
		Name:        in.Name,
		Description: in.Description,
		Status:      knowledge.ChangeSetDraft,
		Origin:      in.Origin,
		CreatedBy:   p.Actor,
	}
	if err := ks.CreateChangeSet(ctx, cs); err != nil {
		return fmt.Errorf("create change-set: %w", err)
	}
	ops := make([]*knowledge.ChangeSetOp, 0, len(in.Ops))
	for _, o := range in.Ops {
		op := &knowledge.ChangeSetOp{
			WorkspaceID: p.WorkspaceID,
			ChangesetID: cs.ID,
			Op:          knowledge.OpType(o.Op),
			Payload:     o.Payload,
			BaseRev:     o.BaseRev,
			CreatedBy:   p.Actor,
		}
		if err := ks.AppendOp(ctx, op); err != nil {
			return fmt.Errorf("append change-set operation %q: %w", o.Op, err)
		}
		ops = append(ops, op)
	}
	submitted, err := knowledge.SubmitChangeSet(ctx, ks, cs, ops)
	if err != nil {
		return fmt.Errorf("submit change-set: %w", err)
	}
	changeset := func(t knowledge.EventType, csID string) {
		res.Events = append(res.Events, knowledge.MergeEvent{
			Type: t, WorkspaceID: p.WorkspaceID, ChangesetID: csID, Actor: p.Actor, OnBehalfOf: p.OnBehalfOf,
		})
	}
	res.Outcome.ConceptsProposed = len(ops)
	res.Outcome.ChangesetID = submitted.ID
	res.Outcome.ChangesetUnchanged = submitted.Unchanged
	res.Digest = knowledge.DigestChangeSet(cs.Name, ops)
	if submitted.Unchanged {
		return nil
	}
	changeset(knowledge.EventChangeSetCreated, cs.ID)
	for _, prior := range submitted.Superseded {
		changeset(knowledge.EventChangeSetSuperseded, prior)
	}
	changeset(knowledge.EventChangeSetSubmitted, cs.ID)
	res.Submitted = cs
	values := make([]knowledge.ChangeSetOp, 0, len(ops))
	for _, op := range ops {
		values = append(values, *op)
	}
	res.Governed, _ = knowledge.ChangeSetIsGoverned(values)
	return nil
}

// termsFromInfo reads wire terms the way the concept routes do: a term with
// no status is approved.
func termsFromInfo(infos []apiclient.TermInfo) []terms.Term {
	out := make([]terms.Term, len(infos))
	for i, t := range infos {
		out[i] = terms.Term{
			Text:         t.Text,
			Locale:       model.LocaleID(t.Locale),
			Status:       model.TermStatus(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
			Forms:        t.Forms,
		}
		if out[i].Status == "" {
			out[i].Status = model.TermApproved
		}
	}
	return out
}
