package server

import (
	"errors"
	"time"

	"github.com/labstack/echo/v4"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/id"
)

// errKnowledgeUnavailable is returned (as a 503) when the brand knowledge graph
// is not configured — the platform runs without PostgreSQL, or the knowledge
// store failed to initialize.
var errKnowledgeUnavailable = errors.New("brand knowledge graph not configured")

// knowledgeEngineFor builds a knowledge.Engine bound to one workspace. The
// engine is store-agnostic: it depends on a narrow BlockSource (the content
// store), a ConceptStore (the workspace termbase), a ProfileStore (the brand
// store), and the governance Store. The production stores satisfy those small
// interfaces directly — the compile-time checks in knowledge/engine.go and
// knowledge/pilot.go prove the content store is a BlockSource /
// CollectionResolver / StreamBindingStore, the framework termbase is a
// ConceptStore, and the brand store is a ProfileStore — so no adapters are
// needed. An engine holds only pointers, so building a fresh one per request is
// cheap; the workspace termbase it carries is cached by workspaceStores.
//
// wsSlug is the workspace slug (c.Param("ws")), which keys the workspace
// termbase; the read-side walk methods (EvaluateChangeSet, ConceptUsage) take
// the workspace ID separately so they can filter stored projects by
// Project.WorkspaceID.
func (s *Server) knowledgeEngineFor(wsSlug string) (*knowledge.Engine, error) {
	if s.KnowledgeStore == nil || s.ContentStore == nil || s.wsStores == nil {
		return nil, errKnowledgeUnavailable
	}
	tb, err := s.wsStores.getTB(wsSlug)
	if err != nil {
		return nil, err
	}
	return knowledge.NewEngine(s.ContentStore, tb, s.BrandStore, s.KnowledgeStore), nil
}

// publishKnowledgeEvents publishes the domain events a knowledge-graph mutation
// produced to the platform event bus, where the audit logger (it subscribes to
// the bus), notification dispatcher, SSE relay, and desktop watch pick them up —
// exactly the brand-loop publish pattern. The engine never touches the bus; it
// returns knowledge.MergeEvent descriptors and the server layer maps each onto a
// minted, timestamped platform event here. The ordinary-edit handlers in
// handlers_concepts.go and the change-set handlers in handlers_changesets.go
// build the same descriptors for the events they fire directly.
func (s *Server) publishKnowledgeEvents(c echo.Context, events []knowledge.MergeEvent) {
	if s.EventBus == nil || len(events) == 0 {
		return
	}
	fallbackActor, _ := c.Get("user_id").(string)
	now := time.Now().UTC()
	for _, ev := range events {
		actor := ev.Actor
		if actor == "" {
			actor = fallbackActor
		}
		data := map[string]string{}
		put := func(k, v string) {
			if v != "" {
				data[k] = v
			}
		}
		put("changeset_id", ev.ChangesetID)
		put("concept_id", ev.ConceptID)
		put("profile_id", ev.ProfileID)
		put("project_id", ev.ProjectID)
		put("stream", ev.Stream)
		resType, resID := knowledgeEventResource(ev)
		s.EventBus.Publish(platev.Event{
			ID:           id.New(),
			Type:         ev.Type,
			Source:       "knowledge",
			WorkspaceID:  ev.WorkspaceID,
			Actor:        actor,
			Data:         data,
			ResourceType: resType,
			ResourceID:   resID,
			Timestamp:    now,
		})
	}
}

// knowledgeEventResource maps a knowledge event to the audit-enrichment
// resource (type, ID) that best identifies what it touched.
func knowledgeEventResource(ev knowledge.MergeEvent) (resourceType, resourceID string) {
	switch {
	case ev.ConceptID != "":
		return "concept", ev.ConceptID
	case ev.ChangesetID != "":
		return "changeset", ev.ChangesetID
	case ev.ProfileID != "":
		return "voice_profile", ev.ProfileID
	default:
		return "", ""
	}
}

// conceptEvent is a small constructor for a concept-scoped knowledge event the
// ordinary-edit handlers publish directly (no change-set, no engine merge).
func conceptEvent(t knowledge.EventType, workspaceID, conceptID, actor string) knowledge.MergeEvent {
	return knowledge.MergeEvent{
		Type:        t,
		WorkspaceID: workspaceID,
		ConceptID:   conceptID,
		Actor:       actor,
	}
}
