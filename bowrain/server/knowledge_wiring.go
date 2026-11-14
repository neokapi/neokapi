package server

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/knowledge"
)

// errKnowledgeUnavailable is returned (as a 503) when the brand knowledge graph
// is not configured — the platform runs without PostgreSQL, or the knowledge
// store failed to initialize.
var errKnowledgeUnavailable = errors.New("brand knowledge graph not configured")

// knowledgeEngineFor builds a knowledge.Engine bound to one workspace. The
// engine is store-agnostic: it depends on a narrow BlockSource (the content
// store), a ConceptStore (the workspace terms), a ProfileStore (the brand
// store), and the governance Store. The production stores satisfy those small
// interfaces directly — the compile-time checks in knowledge/engine.go and
// knowledge/pilot.go prove the content store is a BlockSource /
// CollectionResolver / StreamBindingStore, the framework terms is a
// ConceptStore, and the voice store is a ProfileStore — so no adapters are
// needed. An engine holds only pointers, so building a fresh one per request is
// cheap; the workspace terms it carries is cached by workspaceStores.
//
// wsSlug is the workspace slug (c.Param("ws")), which keys the workspace
// terms; the read-side walk methods (EvaluateChangeSet, ConceptUsage) take
// the workspace ID separately so they can filter stored projects by
// Project.WorkspaceID.
func (s *Server) knowledgeEngineFor(ctx context.Context, wsSlug string) (*knowledge.Engine, error) {
	if s.KnowledgeStore == nil || s.ContentStore == nil || s.wsStores == nil {
		return nil, errKnowledgeUnavailable
	}
	tb, err := s.wsStores.getTerms(ctx, wsSlug)
	if err != nil {
		return nil, err
	}
	return knowledge.NewEngine(s.ContentStore, tb, s.KnowledgeStore), nil
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
	fallbackActor, _ := c.Get("user_id").(string)
	s.publishKnowledgeEventsIn(c.Request().Context(), c.Param("ws"), fallbackActor, events)
}

// publishKnowledgeEventsIn is publishKnowledgeEvents for a caller outside a
// request: the workspace slug the events are keyed by, and the actor an event
// that names none is attributed to.
func (s *Server) publishKnowledgeEventsIn(ctx context.Context, wsSlug, fallbackActor string, events []knowledge.MergeEvent) {
	if s.EventBus == nil || len(events) == 0 {
		return
	}
	now := time.Now().UTC()
	for _, ev := range events {
		var summary *knowledge.ChangeSetDigest
		if strings.HasPrefix(string(ev.Type), "changeset.") {
			summary = s.changeSetDigest(ctx, ev.WorkspaceID, ev.ChangesetID)
		}
		s.EventBus.Publish(knowledge.PlatformEvent(ev, "knowledge", wsSlug, fallbackActor, summary, now))
	}
}

// changeSetDigest reads a change set's name and the number of its ops by op
// type, so the feed can show what the set holds. The op types are a closed
// list, which bounds the counts. A set that cannot be read gives nil.
func (s *Server) changeSetDigest(ctx context.Context, workspaceID, changesetID string) *knowledge.ChangeSetDigest {
	if s.KnowledgeStore == nil || workspaceID == "" || changesetID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	cs, err := s.KnowledgeStore.GetChangeSet(ctx, workspaceID, changesetID)
	if err != nil || cs == nil {
		return nil
	}
	ops, err := s.KnowledgeStore.ListOps(ctx, workspaceID, changesetID)
	if err != nil {
		ops = nil
	}
	return knowledge.DigestChangeSet(cs.Name, ops)
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
