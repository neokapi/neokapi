package knowledge

import (
	"context"
	"errors"
	"fmt"
	"time"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/termbase"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// StreamBindingStore is the slice of the content StreamStore the pilot lifecycle
// uses to bind (and later unbind) a candidate brand-voice profile to a content
// stream via Stream.Properties. The real bowrain ContentStore satisfies it; the
// engine reaches it through its BlockSource, which the ContentStore also is.
type StreamBindingStore interface {
	GetStream(ctx context.Context, projectID, name string) (*store.Stream, error)
	UpdateStream(ctx context.Context, s *store.Stream) error
}

// PilotProfileStore is the slice of the brand store the pilot lifecycle needs to
// materialize and retire a stream-scoped candidate profile: the read+update
// ProfileStore plus create and delete. corebrand.BrandStore satisfies it.
type PilotProfileStore interface {
	ProfileStore
	CreateProfile(ctx context.Context, profile *corebrand.VoiceProfile) error
	DeleteProfile(ctx context.Context, id string) error
}

// Compile-time proof that the production stores satisfy the pilot interfaces:
// the bowrain ContentStore is a StreamBindingStore, the framework termbase
// TBStore carries the stream-shadow methods the pilot writes to, and the brand
// store is a PilotProfileStore.
var (
	_ StreamBindingStore = (store.ContentStore)(nil)
	_ PilotProfileStore  = (corebrand.BrandStore)(nil)
)

// pilotShadowPrefix namespaces every row the pilot lifecycle writes to the
// termbase. The framework concept/relation tables key on ID alone (stream is a
// plain column), so a pilot shadow must use a *distinct* ID per (change-set,
// stream, original) — writing a live concept's own ID with a pilot stream would
// re-home the single live row onto the pilot branch and destroy the workspace
// graph. Namespacing keeps the live graph untouched and makes StopPilot a clean,
// deterministic delete-by-ID.
const pilotShadowPrefix = "__pilot__"

// pilotConceptID is the stream-shadow ID for a concept under a pilot.
func pilotConceptID(changesetID, stream, conceptID string) string {
	return pilotShadowPrefix + ":c:" + changesetID + ":" + stream + ":" + conceptID
}

// pilotRelationID is the stream-shadow ID for a relation under a pilot.
func pilotRelationID(changesetID, stream, relationID string) string {
	return pilotShadowPrefix + ":r:" + changesetID + ":" + stream + ":" + relationID
}

// pilotProfileID is the ID of the throwaway candidate profile a pilot binds to a
// content stream for the change-set's voice ops.
func pilotProfileID(changesetID, stream, profileID string) string {
	return pilotShadowPrefix + ":v:" + changesetID + ":" + stream + ":" + profileID
}

// StartPilot binds a change-set to one content stream as a pilot so real content
// and real checks resolve through the draft before it merges (AD-021). It writes
// the change-set's resulting concepts and added relations into the termbase's
// stream-scoped shadow (AddConceptWithStream / AddRelationWithStream on the pilot
// stream, under namespaced IDs), materializes a candidate brand profile for the
// change-set's voice ops and binds it to the content stream's brand-voice
// property, then records the pilot. It is safe to re-run: shadow writes are
// upserts and the pilot record upserts by its key.
func (e *Engine) StartPilot(ctx context.Context, workspaceID string, store Store, cs ChangeSet, projectID, stream string) error {
	if store == nil {
		return errors.New("knowledge: StartPilot requires a non-nil store")
	}
	if stream == "" {
		return errors.New("knowledge: StartPilot requires a stream")
	}

	ops, err := e.loadOps(ctx, store, workspaceID, cs.ID)
	if err != nil {
		return err
	}

	// Write the change-set's resulting concepts and relations into the termbase
	// stream shadow. Skipped entirely for a voice-only change-set, which needs no
	// termbase store at all.
	if hasTermbaseOps(ops) {
		if err := e.writePilotShadow(ctx, cs, ops, stream); err != nil {
			return err
		}
	}

	// Bind a candidate voice profile to the content stream for voice ops.
	if err := e.bindPilotVoice(ctx, cs, ops, projectID, stream); err != nil {
		return err
	}

	pilot := &Pilot{
		WorkspaceID: workspaceID,
		ChangesetID: cs.ID,
		ProjectID:   projectID,
		Stream:      stream,
		CreatedBy:   mergeActor(cs),
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.AddPilot(ctx, pilot); err != nil {
		return fmt.Errorf("record pilot: %w", err)
	}
	return nil
}

// StopPilot retires a pilot: it removes the change-set's stream-shadow concepts
// and relations, clears (and deletes) the candidate brand-voice binding on the
// content stream, and removes the pilot record. It is idempotent — every removal
// tolerates an already-absent row — so merge and abandon can call it
// unconditionally.
func (e *Engine) StopPilot(ctx context.Context, workspaceID string, store Store, cs ChangeSet, projectID, stream string) error {
	if store == nil {
		return errors.New("knowledge: StopPilot requires a non-nil store")
	}

	ops, err := e.loadOps(ctx, store, workspaceID, cs.ID)
	if err != nil {
		return err
	}

	// Remove the termbase stream shadow (relations first, then their concepts).
	// Skipped for a voice-only change-set, which wrote no shadow.
	if hasTermbaseOps(ops) {
		if err := e.removePilotShadow(ctx, cs, ops, stream); err != nil {
			return err
		}
	}

	// Clear the candidate voice binding and delete the candidate profiles.
	if err := e.unbindPilotVoice(ctx, cs, ops, projectID, stream); err != nil {
		return err
	}

	if err := store.RemovePilot(ctx, workspaceID, cs.ID, projectID, stream); err != nil && !isNotFound(err) {
		return fmt.Errorf("remove pilot: %w", err)
	}
	return nil
}

// StopAllPilots retires every pilot of a change-set, returning how many were
// stopped and the pilot.stopped events the caller should publish. Merge calls it
// to retire shadows on success; the abandon path (P4) calls the same helper so
// both lifecycle exits clean up identically.
func (e *Engine) StopAllPilots(ctx context.Context, workspaceID string, store Store, cs ChangeSet) (int, []MergeEvent, error) {
	if store == nil {
		return 0, nil, errors.New("knowledge: StopAllPilots requires a non-nil store")
	}
	pilots, err := store.ListPilots(ctx, workspaceID, cs.ID)
	if err != nil {
		return 0, nil, fmt.Errorf("list pilots of change-set %q: %w", cs.ID, err)
	}
	var events []MergeEvent
	stopped := 0
	for _, p := range pilots {
		if p == nil {
			continue
		}
		if err := e.StopPilot(ctx, workspaceID, store, cs, p.ProjectID, p.Stream); err != nil {
			return stopped, events, fmt.Errorf("stop pilot %s/%s: %w", p.ProjectID, p.Stream, err)
		}
		stopped++
		events = append(events, MergeEvent{
			Type:        EventPilotStopped,
			WorkspaceID: workspaceID,
			ChangesetID: cs.ID,
			ProjectID:   p.ProjectID,
			Stream:      p.Stream,
			Actor:       mergeActor(cs),
		})
	}
	return stopped, events, nil
}

// writePilotShadow writes the change-set's resulting concepts and added
// relations into the termbase stream shadow under namespaced IDs.
func (e *Engine) writePilotShadow(ctx context.Context, cs ChangeSet, ops []ChangeSetOp, stream string) error {
	shadow, err := e.shadowStore()
	if err != nil {
		return err
	}

	// Build the "after" graph the change-set would produce, purely in memory.
	before, err := e.buildBeforeTermbase(ctx, ops)
	if err != nil {
		return fmt.Errorf("build before termbase: %w", err)
	}
	after, err := ApplyOpsToTermbase(ctx, before, ops)
	if err != nil {
		return fmt.Errorf("build after termbase: %w", err)
	}

	// Shadow every touched concept that survives, under a namespaced ID.
	for _, cid := range touchedConceptIDs(ops) {
		c, ok, err := after.GetConcept(ctx, cid)
		if err != nil {
			return fmt.Errorf("resolve resulting concept %q: %w", cid, err)
		}
		if !ok {
			continue // deleted by the change-set; nothing to shadow
		}
		sc := deepCopyConcept(c)
		sc.ID = pilotConceptID(cs.ID, stream, cid)
		if err := shadow.AddConceptWithStream(ctx, sc, stream); err != nil {
			return fmt.Errorf("write shadow concept %q: %w", cid, err)
		}
	}

	// Shadow the relations the change-set adds, with endpoints remapped to the
	// namespaced shadow concepts (both endpoints are touched, so both exist).
	for _, op := range ops {
		if op.Op != OpRelationAdd {
			continue
		}
		var p RelationAddPayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		rel := p.Relation
		rel.ID = pilotRelationID(cs.ID, stream, p.Relation.ID)
		rel.SourceID = pilotConceptID(cs.ID, stream, p.Relation.SourceID)
		rel.TargetID = pilotConceptID(cs.ID, stream, p.Relation.TargetID)
		if err := shadow.AddRelationWithStream(ctx, rel, stream); err != nil {
			return fmt.Errorf("write shadow relation %q: %w", p.Relation.ID, err)
		}
	}
	return nil
}

// removePilotShadow deletes the change-set's stream-shadow relations and
// concepts. Every delete tolerates an already-absent row, so it is idempotent.
func (e *Engine) removePilotShadow(ctx context.Context, cs ChangeSet, ops []ChangeSetOp, stream string) error {
	shadow, err := e.shadowStore()
	if err != nil {
		return err
	}
	for _, op := range ops {
		if op.Op != OpRelationAdd {
			continue
		}
		var p RelationAddPayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if err := shadow.DeleteRelation(ctx, pilotRelationID(cs.ID, stream, p.Relation.ID)); err != nil && !isNotFound(err) {
			return fmt.Errorf("remove shadow relation %q: %w", p.Relation.ID, err)
		}
	}
	for _, cid := range touchedConceptIDs(ops) {
		if err := shadow.DeleteConcept(ctx, pilotConceptID(cs.ID, stream, cid)); err != nil && !isNotFound(err) {
			return fmt.Errorf("remove shadow concept %q: %w", cid, err)
		}
	}
	return nil
}

// bindPilotVoice materializes a candidate profile for each voice-targeted
// profile (ApplyVoiceOpsToProfile — the CandidateWithRule semantics generalized
// to the change-set's voice ops) and binds the first one to the content stream's
// brand-voice property, so checks in the pilot stream resolve through the draft.
// A change-set with no voice ops is a no-op.
func (e *Engine) bindPilotVoice(ctx context.Context, cs ChangeSet, ops []ChangeSetOp, projectID, stream string) error {
	ids := voiceProfileIDs(ops)
	if len(ids) == 0 {
		return nil
	}
	profiles, ok := e.profiles.(PilotProfileStore)
	if !ok {
		return errors.New("knowledge: profile store cannot materialize pilot candidates (need CreateProfile/DeleteProfile)")
	}

	var bound string
	for _, id := range ids {
		baseline, err := e.profiles.GetProfile(ctx, id)
		if err != nil {
			return fmt.Errorf("load profile %q: %w", id, err)
		}
		if baseline == nil {
			continue
		}
		cand := ApplyVoiceOpsToProfile(baseline, ops)
		cand.ID = pilotProfileID(cs.ID, stream, id)
		cand.VersionNote = fmt.Sprintf("pilot candidate for change-set %q", changeSetLabel(cs))
		cand.UpdatedAt = time.Now().UTC()
		if err := profiles.CreateProfile(ctx, cand); err != nil {
			return fmt.Errorf("create pilot candidate profile for %q: %w", id, err)
		}
		if bound == "" {
			bound = cand.ID
		}
	}
	if bound == "" {
		return nil
	}

	streams, err := e.streamStore()
	if err != nil {
		return err
	}
	s, err := streams.GetStream(ctx, projectID, stream)
	if err != nil {
		return fmt.Errorf("load stream %s/%s: %w", projectID, stream, err)
	}
	if s == nil {
		return fmt.Errorf("stream %s/%s not found", projectID, stream)
	}
	if s.Properties == nil {
		s.Properties = map[string]string{}
	}
	s.Properties[corebrand.PropertyProfileID] = bound
	if err := streams.UpdateStream(ctx, s); err != nil {
		return fmt.Errorf("bind candidate voice profile to stream %s/%s: %w", projectID, stream, err)
	}
	return nil
}

// unbindPilotVoice clears the candidate brand-voice binding the pilot set (only
// when the stream still points at one of this pilot's candidates) and deletes the
// candidate profiles. Both steps tolerate an already-cleaned state.
func (e *Engine) unbindPilotVoice(ctx context.Context, cs ChangeSet, ops []ChangeSetOp, projectID, stream string) error {
	ids := voiceProfileIDs(ops)
	if len(ids) == 0 {
		return nil
	}

	if streams, ok := e.blocks.(StreamBindingStore); ok {
		s, err := streams.GetStream(ctx, projectID, stream)
		if err != nil {
			return fmt.Errorf("load stream %s/%s: %w", projectID, stream, err)
		}
		if s != nil && s.Properties != nil {
			current := s.Properties[corebrand.PropertyProfileID]
			for _, id := range ids {
				if current == pilotProfileID(cs.ID, stream, id) {
					delete(s.Properties, corebrand.PropertyProfileID)
					if err := streams.UpdateStream(ctx, s); err != nil {
						return fmt.Errorf("clear candidate voice binding on stream %s/%s: %w", projectID, stream, err)
					}
					break
				}
			}
		}
	}

	if profiles, ok := e.profiles.(PilotProfileStore); ok {
		for _, id := range ids {
			if err := profiles.DeleteProfile(ctx, pilotProfileID(cs.ID, stream, id)); err != nil && !isNotFound(err) {
				return fmt.Errorf("delete pilot candidate profile for %q: %w", id, err)
			}
		}
	}
	return nil
}

// shadowStore returns the engine's concept store as the stream-shadow write
// surface the pilot lifecycle needs (the framework termbase TBStore).
func (e *Engine) shadowStore() (termbase.TBStore, error) {
	s, ok := e.concepts.(termbase.TBStore)
	if !ok {
		return nil, errors.New("knowledge: concept store does not support stream shadows (need termbase.TBStore)")
	}
	return s, nil
}

// streamStore returns the content StreamStore slice the pilot lifecycle binds
// voice profiles through, reached via the engine's BlockSource (the real
// ContentStore is both).
func (e *Engine) streamStore() (StreamBindingStore, error) {
	s, ok := e.blocks.(StreamBindingStore)
	if !ok {
		return nil, errors.New("knowledge: block source does not provide stream binding (need a content StreamStore)")
	}
	return s, nil
}
