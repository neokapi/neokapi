package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/termscope"
	"github.com/neokapi/neokapi/terms"
)

var (
	ErrInvalidPilotProject  = errors.New("pilot requires one project ID")
	ErrPilotChangeSetClosed = errors.New("change set is closed")
)

func checkPilotStatus(status ChangeSetStatus) error {
	if len(allowedChangeSetTransitions[status]) == 0 {
		return fmt.Errorf("%w: %s", ErrPilotChangeSetClosed, status)
	}
	return nil
}

func (p Pilot) shadowNamespace(kind string) string {
	return fmt.Sprintf(":v2:%s:%x:%x:%x:", kind, p.ChangesetID, p.ProjectID, p.Stream)
}

func (p Pilot) shadowID(kind, originalID string) string {
	return terms.ShadowIDPrefix + p.shadowNamespace(kind) + originalID
}

// StartPilot publishes the proposed terms and relations that apply to one project and stream.
// A retry replaces the binding's copies with the current proposal.
func (e *Engine) StartPilot(ctx context.Context, workspaceID string, store Store, cs ChangeSet, projectID, stream string) (*Pilot, error) {
	if projectID == "" || projectID != strings.TrimSpace(projectID) || strings.Contains(projectID, ",") {
		return nil, ErrInvalidPilotProject
	}
	if strings.TrimSpace(stream) == "" {
		return nil, errors.New("knowledge: StartPilot requires a stream")
	}
	pilot := &Pilot{
		WorkspaceID: workspaceID, ChangesetID: cs.ID, ProjectID: projectID, Stream: stream,
		CreatedBy: mergeActor(cs), CreatedAt: time.Now().UTC(),
	}
	err := e.withLockedChangeSet(ctx, store, workspaceID, cs.ID, func(engine *Engine, store Store, status ChangeSetStatus) error {
		if err := checkPilotStatus(status); err != nil {
			return err
		}
		ops, err := engine.loadOps(ctx, store, workspaceID, cs.ID)
		if err != nil {
			return err
		}
		if err := engine.replacePilotShadow(ctx, pilot, ops); err != nil {
			return err
		}
		return store.AddPilot(ctx, pilot)
	})
	if err != nil {
		return nil, err
	}
	return pilot, nil
}

// StopPilot removes one binding and its copies, including copies whose operations have since been removed.
func (e *Engine) StopPilot(ctx context.Context, workspaceID string, store Store, cs ChangeSet, projectID, stream string) error {
	pilot := &Pilot{WorkspaceID: workspaceID, ChangesetID: cs.ID, ProjectID: projectID, Stream: stream}
	return e.withLockedChangeSet(ctx, store, workspaceID, cs.ID, func(engine *Engine, store Store, _ ChangeSetStatus) error {
		if err := engine.replacePilotShadow(ctx, pilot, nil); err != nil {
			return err
		}
		if err := store.RemovePilot(ctx, workspaceID, cs.ID, projectID, stream); err != nil && !isNotFound(err) {
			return fmt.Errorf("remove pilot: %w", err)
		}
		return nil
	})
}

// AbandonChangeSet abandons the proposal and retires its active pilots.
func (e *Engine) AbandonChangeSet(ctx context.Context, workspaceID string, store Store, cs ChangeSet) ([]MergeEvent, error) {
	return e.retireChangeSet(ctx, workspaceID, store, cs, func(store Store) error {
		return store.SetChangeSetStatus(ctx, workspaceID, cs.ID, ChangeSetAbandoned)
	})
}

// SupersedeChangeSet records the successor and retires the prior proposal's active pilots.
func (e *Engine) SupersedeChangeSet(ctx context.Context, workspaceID string, store Store, cs ChangeSet, successorID string) ([]MergeEvent, error) {
	return e.retireChangeSet(ctx, workspaceID, store, cs, func(store Store) error {
		return store.SupersedeChangeSet(ctx, workspaceID, cs.ID, successorID)
	})
}

func (e *Engine) retireChangeSet(ctx context.Context, workspaceID string, store Store, cs ChangeSet, transition func(Store) error) ([]MergeEvent, error) {
	var events []MergeEvent
	err := e.withLockedChangeSet(ctx, store, workspaceID, cs.ID, func(engine *Engine, store Store, status ChangeSetStatus) error {
		if err := checkPilotStatus(status); err != nil {
			return err
		}
		if _, ok := engine.concepts.(terms.Store); !ok {
			pilots, err := store.ListPilots(ctx, workspaceID, cs.ID)
			if err != nil {
				return err
			}
			if len(pilots) != 0 {
				return errors.New("retiring active pilots requires the workspace terms store")
			}
			return transition(store)
		}
		if err := transition(store); err != nil {
			return err
		}
		_, stopped, err := engine.StopAllPilots(ctx, workspaceID, store, cs)
		events = stopped
		return err
	})
	if err != nil {
		return nil, err
	}
	return events, nil
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

func (e *Engine) replacePilotShadow(ctx context.Context, pilot *Pilot, ops []ChangeSetOp) error {
	shadow, ok := e.concepts.(terms.Store)
	if !ok {
		if len(ops) == 0 {
			return nil
		}
		return errors.New("knowledge: concept store does not support pilot shadows")
	}

	if len(ops) == 0 {
		return pilot.clearShadow(ctx, shadow)
	}

	before, err := e.buildBeforeTerms(ctx, ops)
	if err != nil {
		return fmt.Errorf("build before terms: %w", err)
	}
	after, err := ApplyOpsToTerms(ctx, before, ops)
	if err != nil {
		return fmt.Errorf("build after terms: %w", err)
	}
	if err := pilot.clearShadow(ctx, shadow); err != nil {
		return fmt.Errorf("clear pilot concepts: %w", err)
	}

	visible := make(map[string]bool)
	for _, cid := range touchedConceptIDs(ops) {
		original, _, err := before.GetConcept(ctx, cid)
		if err != nil {
			return err
		}
		c, ok, err := after.GetConcept(ctx, cid)
		if err != nil {
			return err
		}
		if !ok || !termscope.AppliesIn(original, pilot.ProjectID) || !termscope.AppliesIn(c, pilot.ProjectID) {
			continue
		}
		termscope.Limit(&c, pilot.ProjectID)
		c.ID = pilot.shadowID("c", cid)
		if err := shadow.AddConceptWithStream(ctx, c, pilot.Stream); err != nil {
			return fmt.Errorf("write pilot concept %q: %w", cid, err)
		}
		visible[cid] = true
	}
	added := make(map[string]bool)
	for _, op := range ops {
		if op.Op == OpRelationAdd {
			var p RelationAddPayload
			if err := decodePayload(op, &p); err != nil {
				return err
			}
			added[p.Relation.ID] = true
		}
	}
	relations, err := after.ListRelations(ctx, nil)
	if err != nil {
		return err
	}
	for _, rel := range relations {
		if !added[rel.ID] || !visible[rel.SourceID] || !visible[rel.TargetID] {
			continue
		}
		rel.ID = pilot.shadowID("r", rel.ID)
		rel.SourceID = pilot.shadowID("c", rel.SourceID)
		rel.TargetID = pilot.shadowID("c", rel.TargetID)
		if err := shadow.AddRelationWithStream(ctx, rel, pilot.Stream); err != nil {
			return fmt.Errorf("write pilot relation %q: %w", rel.ID, err)
		}
	}
	return nil
}

// clearShadow removes the pilot's shadow concepts. A store that keeps
// revisions deletes them at the revisions read here, in one statement: a
// shadow someone edited since is a *sqlterms.StaleConceptError and nothing is
// deleted, so the edit is reported rather than lost.
func (p Pilot) clearShadow(ctx context.Context, store terms.Store) error {
	prefix, err := terms.ShadowNamespacePrefix(p.shadowNamespace("c"))
	if err != nil {
		return err
	}
	var ids []string
	for offset := 0; ; {
		page, total, err := store.SearchForStream(ctx, "", "", "", p.Stream, nil, offset, 128)
		if err != nil {
			return err
		}
		for _, cp := range page {
			if strings.HasPrefix(cp.ID, prefix) {
				ids = append(ids, cp.ID)
			}
		}
		offset += len(page)
		if len(page) == 0 || offset >= total {
			break
		}
	}
	if w, ok := store.(sqlterms.ConceptWriter); ok {
		revs, err := w.ConceptRevisions(ctx, ids)
		if err != nil {
			return err
		}
		if err := w.DeleteConceptsAt(ctx, revs); err != nil {
			return fmt.Errorf("clear pilot shadow: %w", err)
		}
		return nil
	}
	for _, id := range ids {
		if err := store.DeleteConcept(ctx, id); err != nil && !isNotFound(err) {
			return err
		}
	}
	return nil
}
