package knowledge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/terms"
)

// pilotShadowPrefix namespaces every row the pilot lifecycle writes to the
// terms. The framework concept/relation tables key on ID alone (stream is a
// plain column), so a pilot shadow must use a *distinct* ID per (change-set,
// stream, original) — writing a live concept's own ID with a pilot stream would
// re-home the single live row onto the pilot branch and destroy the workspace
// graph. Namespacing keeps the live graph untouched and makes StopPilot a clean,
// deterministic delete-by-ID.
//
// The namespace is the framework's, not this package's: a store has to filter it
// out of its stream-blind reads for a pilot to stay on its own stream, so the
// reservation belongs where every store can see it.
const pilotShadowPrefix = terms.ShadowIDPrefix

// pilotConceptID is the stream-shadow ID for a concept under a pilot.
func pilotConceptID(changesetID, stream, conceptID string) string {
	return pilotShadowPrefix + ":c:" + changesetID + ":" + stream + ":" + conceptID
}

// pilotRelationID is the stream-shadow ID for a relation under a pilot.
func pilotRelationID(changesetID, stream, relationID string) string {
	return pilotShadowPrefix + ":r:" + changesetID + ":" + stream + ":" + relationID
}

// StartPilot binds a change-set to one content stream as a pilot so real content
// and real checks resolve through the draft before it merges (AD-021). It writes
// the change-set's resulting concepts and added relations into the terms store's
// stream-scoped shadow (AddConceptWithStream / AddRelationWithStream on the pilot
// stream, under namespaced IDs), then records the pilot. It returns the recorded pilot so callers
// surface the persisted creator and creation time rather than reconstructing
// them. It is safe to re-run: shadow writes are upserts and the pilot record
// upserts by its key.
func (e *Engine) StartPilot(ctx context.Context, workspaceID string, store Store, cs ChangeSet, projectID, stream string) (*Pilot, error) {
	if store == nil {
		return nil, errors.New("knowledge: StartPilot requires a non-nil store")
	}
	if stream == "" {
		return nil, errors.New("knowledge: StartPilot requires a stream")
	}

	ops, err := e.loadOps(ctx, store, workspaceID, cs.ID)
	if err != nil {
		return nil, err
	}

	// Write the change-set's resulting concepts and relations into the terms store
	// stream shadow. An empty change-set writes none.
	if len(ops) > 0 {
		if err := e.writePilotShadow(ctx, cs, ops, stream); err != nil {
			return nil, err
		}
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
		return nil, fmt.Errorf("record pilot: %w", err)
	}
	return pilot, nil
}

// StopPilot retires a pilot: it removes the change-set's stream-shadow concepts
// and relations and removes the pilot record. It is idempotent — every removal
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

	// Remove the terms store stream shadow (relations first, then their concepts).
	// An empty change-set wrote none.
	if len(ops) > 0 {
		if err := e.removePilotShadow(ctx, cs, ops, stream); err != nil {
			return err
		}
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
// relations into the terms store stream shadow under namespaced IDs.
func (e *Engine) writePilotShadow(ctx context.Context, cs ChangeSet, ops []ChangeSetOp, stream string) error {
	shadow, err := e.shadowStore()
	if err != nil {
		return err
	}

	// Build the "after" graph the change-set would produce, purely in memory.
	before, err := e.buildBeforeTerms(ctx, ops)
	if err != nil {
		return fmt.Errorf("build before terms: %w", err)
	}
	after, err := ApplyOpsToTerms(ctx, before, ops)
	if err != nil {
		return fmt.Errorf("build after terms: %w", err)
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

// shadowStore returns the engine's concept store as the stream-shadow write
// surface the pilot lifecycle needs (the framework terms Store).
func (e *Engine) shadowStore() (terms.Store, error) {
	s, ok := e.concepts.(terms.Store)
	if !ok {
		return nil, errors.New("knowledge: concept store does not support stream shadows (need terms.Store)")
	}
	return s, nil
}
