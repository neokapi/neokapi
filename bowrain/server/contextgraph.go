package server

import (
	"context"
	"log/slog"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	platgraph "github.com/neokapi/neokapi/bowrain/graph"
)

// contextGraphGroup is the durable consumer-group name for the context-graph
// writer. Stable by design (see convergeOnPushGroup): the group's cursor is
// keyed by this name, so renaming it re-reads the stream from the beginning.
const contextGraphGroup = "context-graph"

// subscribeContextGraphOnPush keeps the workspace context graph in step with
// what a project has pushed.
//
// It is the server's half of the same contract `kapi up` keeps locally: the
// graph is a projection of the stores, rebuilt when the stores change, and the
// change server-side is a completed push. The writer rebuilds the pushed
// project stream's subgraph within its own scope, so a rebuild is idempotent
// and a redelivered event costs one extra pass over unchanged state.
//
// The handler always acknowledges. A failed rebuild leaves the previous
// projection in place and the next push rebuilds it, which is the same
// best-effort posture the local materializer takes on the up path: a derived
// index must never fail the work that produced its inputs.
func (s *Server) subscribeContextGraphOnPush() {
	if s.EventBus == nil || s.GraphStore == nil {
		return
	}
	sqlGraph, ok := s.GraphStore.(*platgraph.SQLGraphStore)
	if !ok {
		return
	}
	s.EventBus.SubscribeGroup(contextGraphGroup, func(ev platev.Event) error {
		if ev.Type != platev.EventPushCompleted {
			return nil
		}
		s.rebuildContextGraph(sqlGraph, ev)
		return nil
	})
}

// rebuildContextGraph materializes one pushed project stream's subgraph. Every
// exit reason is logged: a silently skipped rebuild is indistinguishable from a
// graph nobody ever wrote.
func (s *Server) rebuildContextGraph(g *platgraph.SQLGraphStore, ev platev.Event) {
	if ev.ProjectID == "" || s.ContentStore == nil {
		slog.Warn("context graph: rebuild skipped",
			"reason", "missing project id or content store", "project", ev.ProjectID)
		return
	}
	// Event-bus callback: the rebuild outlives the request that published the
	// event, so no request context applies.
	ctx := context.Background()
	proj, err := s.ContentStore.GetProject(ctx, ev.ProjectID)
	if err != nil || proj == nil {
		slog.Warn("context graph: rebuild skipped",
			"reason", "project lookup failed", "project", ev.ProjectID, "error", err)
		return
	}

	in := platgraph.ProjectSubgraph{
		Content:     s.ContentStore,
		WorkspaceID: proj.WorkspaceID,
		ProjectID:   proj.ID,
		ProjectName: proj.Name,
		Stream:      pushedStream(ev.Data["stream"], proj),
	}
	// The workspace's terminology is keyed by slug. Without it the pass still
	// writes membership, coordinates and blessings — it simply has no vocabulary
	// to attach occurrences to, which is the wrong graph rather than a smaller
	// one, so the slug is resolved from the project when the event does not
	// carry it (a connector ingest publishes no workspace).
	if slug := s.workspaceSlug(ctx, ev.Data["workspace_slug"], proj.WorkspaceID); slug != "" {
		if tb, terr := s.wsStores.getTerms(slug); terr == nil {
			in.Terms = tb
		} else {
			slog.Warn("context graph: no terminology for the workspace",
				"workspace", slug, "project", ev.ProjectID, "error", terr)
		}
	}

	// The workspace's vocabulary just changed, so this is when slug
	// fragmentation is visible. It proposes; it never resolves.
	s.raiseChannelAliasProposals(ctx, proj, in.Stream)

	edges, err := platgraph.MaterializeProjectContextGraph(ctx, g, in)
	if err != nil {
		slog.Warn("context graph: rebuild failed",
			"project", ev.ProjectID, "stream", in.Stream, "error", err)
		return
	}
	slog.Info("context graph: rebuilt",
		"project", ev.ProjectID, "stream", in.Stream, "edges", edges)
}

// workspaceSlug resolves the slug the workspace's terminology is keyed by:
// the one the event carries, else the one the project's workspace holds.
func (s *Server) workspaceSlug(ctx context.Context, fromEvent, workspaceID string) string {
	if fromEvent != "" {
		return fromEvent
	}
	if s.AuthStore == nil || workspaceID == "" {
		return ""
	}
	ws, err := s.AuthStore.GetWorkspace(ctx, workspaceID)
	if err != nil || ws == nil {
		return ""
	}
	return ws.Slug
}

// pushedStream resolves the stream a push landed on: the one the event names,
// then the project's default, then "main". The stream is part of a node's
// identity, so a rebuild that guessed differently from the push would write a
// second subgraph beside the first rather than replacing it.
func pushedStream(fromEvent string, proj *platstore.Project) string {
	if fromEvent != "" {
		return fromEvent
	}
	if proj != nil && proj.DefaultStream != "" {
		return proj.DefaultStream
	}
	return "main"
}
