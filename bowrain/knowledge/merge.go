package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/termbase"
)

// ErrMergeConflict is the sentinel wrapped by MergeChangeSet when one or more
// ops were authored against a concept revision that is no longer current. The
// returned *MergeResult carries the per-op conflicts; nothing was applied.
var ErrMergeConflict = errors.New("knowledge: change-set has stale-draft conflicts")

// MergeEvent describes a domain event a merge (or a pilot stop) produced. The
// engine does not touch the platform event bus — it returns these descriptors
// so the server layer (P4) maps each onto a platform event (ID-minted,
// timestamped, fed to the audit chain, notifications, SSE, and desktop watch).
// Type is one of the knowledge-graph EventType constants in events.go.
type MergeEvent struct {
	Type        EventType `json:"type"`
	WorkspaceID string    `json:"workspace_id"`
	ChangesetID string    `json:"changeset_id,omitempty"`
	ConceptID   string    `json:"concept_id,omitempty"`
	ProfileID   string    `json:"profile_id,omitempty"`
	ProjectID   string    `json:"project_id,omitempty"`
	Stream      string    `json:"stream,omitempty"`
	Actor       string    `json:"actor,omitempty"`
}

// MergeResult reports the outcome of merging a change-set: the conflicts that
// blocked it (when non-empty, nothing was applied and the error wraps
// ErrMergeConflict), the ops that were applied, the concept revisions recorded,
// the concepts and voice profiles touched, the pilots retired, and the domain
// events the caller should publish.
type MergeResult struct {
	ChangeSetID      string       `json:"changeset_id"`
	Conflicts        []OpConflict `json:"conflicts,omitempty"`
	AppliedOps       []int64      `json:"applied_ops,omitempty"`
	RevisionsCreated int          `json:"revisions_created"`
	ConceptsTouched  []string     `json:"concepts_touched,omitempty"`
	ProfilesTouched  []string     `json:"profiles_touched,omitempty"`
	PilotsStopped    int          `json:"pilots_stopped"`
	Events           []MergeEvent `json:"events,omitempty"`
}

// conceptSnapshot is the immutable record stored per touched concept at merge:
// the resulting concept and its relations (or a tombstone for a deleted
// concept). It is the snapshot half of a knowledge ConceptRevision (the
// "termbase.Concept + relations delta" the data model calls for).
type conceptSnapshot struct {
	ConceptID string                     `json:"concept_id"`
	Concept   *termbase.Concept          `json:"concept,omitempty"`
	Relations []termbase.ConceptRelation `json:"relations,omitempty"`
	Deleted   bool                       `json:"deleted,omitempty"`
}

// MergeChangeSet merges an approved (or ordinary draft) change-set into the
// workspace graph and brand profiles, recording a concept revision per touched
// concept and retiring the change-set's pilots.
//
// The flow is check-all-then-apply:
//
//  1. Gate. The change-set is classified (ChangeSetIsGoverned) and the
//     separation-of-duties merge gate (CanMerge) is enforced against its
//     reviews. A governed change-set that is not approved by someone other than
//     its author is refused with a clear error before anything is read further.
//  2. Conflict pass (read-only). Every op pinned to a base revision is compared
//     against the concept's current revision (CheckBaseRev). If any op is stale,
//     the merge aborts with the full conflict report and *no* writes — this is
//     what makes a stale draft conflict loudly instead of clobbering an
//     intervening edit (AD-021).
//  3. Apply pass. Concept/term/relation ops are applied to the workspace
//     termbase in seq order via the shared applyTermbaseOp; voice ops are
//     applied to the brand store, version-bumping each touched profile exactly
//     like AD-019 promotion. One immutable ConceptRevision is recorded per
//     touched concept (snapshot, summary, actor, changeset_id).
//  4. Finalize. The change-set is marked merged (store.SetMergeResult) and its
//     pilot shadows are retired (StopPilot per pilot). The domain events that
//     should fire are returned for the caller to publish; the engine never
//     touches the event bus.
//
// Cross-store atomicity. The workspace termbase, the brand store, and the
// knowledge store are three separate stores that share one PostgreSQL database
// but are written here without a single enclosing transaction. The conflict
// pre-check (step 2) makes stale-draft clobbering impossible, so the common
// failure mode — a draft racing a concurrent edit — is caught before any write.
// A mid-apply failure (step 3) is surfaced honestly: the returned error reports
// the failing op, and the returned *MergeResult records which ops were already
// applied (AppliedOps) so an operator can see the partial state. The change-set
// is left un-merged in that case (SetMergeResult is the last apply-pass step),
// so it can be re-driven once the cause is fixed.
//
// The merging actor is taken from cs.MergedBy when the caller pre-sets it to the
// authenticated merger, falling back to the change-set's author.
func (e *Engine) MergeChangeSet(ctx context.Context, workspaceID string, store Store, cs ChangeSet) (*MergeResult, error) {
	if store == nil {
		return nil, errors.New("knowledge: MergeChangeSet requires a non-nil store")
	}

	// 1. Load ops + reviews; classify; enforce the merge gate.
	ops, err := e.loadOps(ctx, store, workspaceID, cs.ID)
	if err != nil {
		return nil, err
	}
	reviews, err := loadReviews(ctx, store, workspaceID, cs.ID)
	if err != nil {
		return nil, err
	}
	governed, err := ChangeSetIsGoverned(ops)
	if err != nil {
		return nil, fmt.Errorf("classify change-set %q: %w", cs.ID, err)
	}
	if err := CanMerge(cs, governed, reviews); err != nil {
		return nil, fmt.Errorf("change-set %q is not mergeable: %w", cs.ID, err)
	}

	res := &MergeResult{ChangeSetID: cs.ID}

	// 2. Conflict pass — read-only, check all before applying any.
	conflicts, err := detectConflicts(ctx, store, workspaceID, ops)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		res.Conflicts = conflicts
		return res, fmt.Errorf("merge aborted with %d conflict(s): %w", len(conflicts), ErrMergeConflict)
	}

	// 3. Apply pass.
	actor := mergeActor(cs)
	live, liveOK := e.concepts.(termbase.TermBase)
	if !liveOK && hasTermbaseOps(ops) {
		return res, errors.New("knowledge: workspace concept store is not writable (need termbase.TermBase)")
	}

	var events []MergeEvent
	for _, op := range ops {
		if isVoiceOp(op.Op) {
			continue // applied as a per-profile batch below
		}
		if err := applyTermbaseOp(ctx, live, op); err != nil {
			return res, fmt.Errorf("apply op seq %d (%s): %w", op.Seq, op.Op, err)
		}
		res.AppliedOps = append(res.AppliedOps, op.Seq)
		if ev := opConceptEvent(workspaceID, cs.ID, actor, op); ev != nil {
			events = append(events, *ev)
		}
	}

	profilesTouched, err := e.applyVoiceOps(ctx, ops, cs)
	if err != nil {
		return res, fmt.Errorf("apply voice ops: %w", err)
	}
	res.ProfilesTouched = profilesTouched
	for _, op := range ops {
		if isVoiceOp(op.Op) {
			res.AppliedOps = append(res.AppliedOps, op.Seq)
		}
	}

	// Record one revision per touched concept.
	touched := touchedConceptIDs(ops)
	now := time.Now().UTC()
	for _, cid := range touched {
		snap, err := snapshotConcept(ctx, live, cid)
		if err != nil {
			return res, fmt.Errorf("snapshot concept %q: %w", cid, err)
		}
		base, err := store.LatestRev(ctx, workspaceID, cid)
		if err != nil {
			return res, fmt.Errorf("latest revision for concept %q: %w", cid, err)
		}
		rev := &ConceptRevision{
			WorkspaceID: workspaceID,
			ConceptID:   cid,
			Rev:         base + 1,
			Snapshot:    snap,
			Summary:     fmt.Sprintf("merged change-set %q", changeSetLabel(cs)),
			Actor:       actor,
			ChangesetID: cs.ID,
			CreatedAt:   now,
		}
		if err := store.AddRevision(ctx, rev); err != nil {
			return res, fmt.Errorf("record revision for concept %q: %w", cid, err)
		}
		res.RevisionsCreated++
	}
	res.ConceptsTouched = touched

	// 4. Finalize: mark merged, then retire pilot shadows.
	if err := store.SetMergeResult(ctx, workspaceID, cs.ID, actor, now); err != nil {
		return res, fmt.Errorf("finalize merge of change-set %q: %w", cs.ID, err)
	}

	stopped, pilotEvents, err := e.StopAllPilots(ctx, workspaceID, store, cs)
	if err != nil {
		return res, fmt.Errorf("retire pilots of change-set %q: %w", cs.ID, err)
	}
	res.PilotsStopped = stopped
	events = append(events, pilotEvents...)

	events = append(events, MergeEvent{
		Type:        EventChangeSetMerged,
		WorkspaceID: workspaceID,
		ChangesetID: cs.ID,
		Actor:       actor,
	})
	res.Events = events
	return res, nil
}

// detectConflicts compares each base-pinned op against the concept's current
// revision, returning every stale op without applying anything. Ops with a zero
// BaseRev (not pinned) never conflict and are skipped without a store read.
// Relation and voice ops carry no concept ID (conceptIDOf returns "") and are
// not pinned to a concept revision, so they are skipped too — checking them
// against LatestRev(ws, "") would always read revision 0 and spuriously flag any
// such op authored with a non-zero BaseRev.
func detectConflicts(ctx context.Context, store Store, workspaceID string, ops []ChangeSetOp) ([]OpConflict, error) {
	var conflicts []OpConflict
	for _, op := range ops {
		if op.BaseRev == 0 {
			continue
		}
		cid := conceptIDOf(op)
		if cid == "" {
			continue // relation and voice ops carry no concept-revision pin
		}
		current, err := store.LatestRev(ctx, workspaceID, cid)
		if err != nil {
			return nil, fmt.Errorf("current revision for op seq %d: %w", op.Seq, err)
		}
		if c := CheckBaseRev(op, current); c != nil {
			conflicts = append(conflicts, *c)
		}
	}
	return conflicts, nil
}

// applyVoiceOps applies the change-set's voice-rule ops to the brand profiles
// they target, version-bumping each touched profile once (AD-019). A voice op
// against a missing profile is a hard error — a merge commits, so it cannot
// silently drop a rule. It returns the distinct profile IDs that were updated.
func (e *Engine) applyVoiceOps(ctx context.Context, ops []ChangeSetOp, cs ChangeSet) ([]string, error) {
	ids := voiceProfileIDs(ops)
	if len(ids) == 0 {
		return nil, nil
	}
	if e.profiles == nil {
		return nil, errors.New("knowledge: voice ops require a profile store")
	}
	touched := make([]string, 0, len(ids))
	for _, id := range ids {
		baseline, err := e.profiles.GetProfile(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("load profile %q: %w", id, err)
		}
		if baseline == nil {
			return nil, fmt.Errorf("voice op references missing profile %q", id)
		}
		cand := ApplyVoiceOpsToProfile(baseline, ops)
		cand.Version = baseline.Version + 1
		cand.VersionNote = fmt.Sprintf("merged change-set %q", changeSetLabel(cs))
		cand.UpdatedAt = time.Now().UTC()
		if err := e.profiles.UpdateProfile(ctx, cand); err != nil {
			return nil, fmt.Errorf("update profile %q: %w", id, err)
		}
		touched = append(touched, id)
	}
	return touched, nil
}

// snapshotConcept builds the revision snapshot for a concept after a merge: the
// resulting concept plus its relations, or a tombstone when the change-set
// deleted it.
func snapshotConcept(ctx context.Context, tb termbase.TermBase, conceptID string) (json.RawMessage, error) {
	snap := conceptSnapshot{ConceptID: conceptID}
	if tb == nil {
		snap.Deleted = true
		return json.Marshal(snap)
	}
	c, ok, err := tb.GetConcept(ctx, conceptID)
	if err != nil {
		return nil, err
	}
	if !ok {
		snap.Deleted = true
		return json.Marshal(snap)
	}
	cc := c
	snap.Concept = &cc
	rels, err := tb.RelationsOf(ctx, conceptID, nil)
	if err != nil {
		return nil, err
	}
	snap.Relations = rels
	return json.Marshal(snap)
}

// opConceptEvent maps a concept/term/relation op to the domain event its
// application should publish, or nil for ops that carry no concept-scoped event.
func opConceptEvent(workspaceID, changesetID, actor string, op ChangeSetOp) *MergeEvent {
	var t EventType
	switch op.Op {
	case OpConceptCreate:
		t = EventConceptCreated
	case OpConceptUpdate, OpTermAdd, OpTermUpdate, OpTermRemove:
		t = EventConceptUpdated
	case OpConceptDelete:
		t = EventConceptDeleted
	case OpTermStatus:
		t = EventConceptTermStatusChanged
	case OpRelationAdd:
		t = EventConceptRelationAdded
	case OpRelationRemove:
		t = EventConceptRelationRemoved
	default:
		return nil
	}
	return &MergeEvent{
		Type:        t,
		WorkspaceID: workspaceID,
		ChangesetID: changesetID,
		ConceptID:   conceptIDOf(op),
		Actor:       actor,
	}
}

// loadOps reads a change-set's ops as values (the pure functions operate on
// value slices).
func (e *Engine) loadOps(ctx context.Context, store Store, workspaceID, changesetID string) ([]ChangeSetOp, error) {
	ptrs, err := store.ListOps(ctx, workspaceID, changesetID)
	if err != nil {
		return nil, fmt.Errorf("list ops of change-set %q: %w", changesetID, err)
	}
	ops := make([]ChangeSetOp, 0, len(ptrs))
	for _, p := range ptrs {
		if p != nil {
			ops = append(ops, *p)
		}
	}
	return ops, nil
}

// loadReviews reads a change-set's reviews as values.
func loadReviews(ctx context.Context, store Store, workspaceID, changesetID string) ([]ChangeSetReview, error) {
	ptrs, err := store.ListReviews(ctx, workspaceID, changesetID)
	if err != nil {
		return nil, fmt.Errorf("list reviews of change-set %q: %w", changesetID, err)
	}
	reviews := make([]ChangeSetReview, 0, len(ptrs))
	for _, p := range ptrs {
		if p != nil {
			reviews = append(reviews, *p)
		}
	}
	return reviews, nil
}

// isVoiceOp reports whether an op targets a brand profile rather than the
// termbase.
func isVoiceOp(o OpType) bool {
	return o == OpVoiceRuleAdd || o == OpVoiceRuleRemove
}

// hasTermbaseOps reports whether ops contains any concept/term/relation op (the
// ops that need a writable workspace termbase at merge).
func hasTermbaseOps(ops []ChangeSetOp) bool {
	for _, op := range ops {
		if !isVoiceOp(op.Op) {
			return true
		}
	}
	return false
}

// mergeActor resolves the identity recorded as the merger: the caller-supplied
// cs.MergedBy when set (the authenticated merger), else the change-set's author.
func mergeActor(cs ChangeSet) string {
	if cs.MergedBy != "" {
		return cs.MergedBy
	}
	return cs.CreatedBy
}

// changeSetLabel is the human-readable handle for a change-set in summaries and
// version notes: its name, or its ID when unnamed.
func changeSetLabel(cs ChangeSet) string {
	if cs.Name != "" {
		return cs.Name
	}
	return cs.ID
}
