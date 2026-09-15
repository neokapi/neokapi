package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/core/model"
	fwterms "github.com/neokapi/neokapi/terms"
)

// wrapContentStoreLikeTheServer wraps the server's content store in the event
// decorator, as NewServer does for every running server. Stores seeded before
// the call are written without events.
func wrapContentStoreLikeTheServer(t *testing.T, srv *Server) {
	t.Helper()
	bus := event.NewChannelEventBus()
	t.Cleanup(func() { bus.Close() })
	srv.ContentStore = event.NewEventEmittingStore(srv.ContentStore, bus)
}

// A completed push rebuilds the workspace context graph through the server's
// own content store. That store carries the event decorator, and the rebuild
// opened blocks only on the bare Postgres or SQLite store, so every rebuild
// failed and the concept explorer answered from its bounded scan.
func TestContextGraph_APushRebuildsTheGraphThroughTheServersStore(t *testing.T) {
	h := newContextGraphHarness(t)
	ctx := t.Context()

	require.NoError(t, h.terms.AddConcept(ctx, fwterms.Concept{
		ID: "c-memory", Domain: "product",
		Terms: []fwterms.Term{{Text: "content memory", Locale: "en", Status: model.TermApproved}},
	}))
	proj := h.seedProject(t, ctx, "docs-site", "docs", "The content memory recycles approved wording.")
	wrapContentStoreLikeTheServer(t, h.srv)

	h.srv.rebuildContextGraph(h.graph, platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: proj.ID,
		Data:      map[string]string{"stream": cgStream, "workspace_slug": cgWorkspace},
	})

	got := h.ask(t, "/", "c-memory")
	assert.Equal(t, "graph", got.Source, "the push's rebuild wrote the graph the explorer reads")
	require.Len(t, got.Projects, 1)
	assert.Equal(t, proj.ID, got.Projects[0].ProjectID)
}

// An automation action writes an overlay through the server's own content
// store, which carries the event decorator on every running server.
func TestAutomation_WriteOverlay_ThroughTheServersStore(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	initTestStores(t, srv)
	ctx := context.Background()

	const projectID = "proj-automation-event-store"
	require.NoError(t, srv.ContentStore.CreateProject(ctx, &platstore.Project{
		ID: projectID, Name: "Overlay through the event store", DefaultSourceLanguage: "en",
	}))
	require.NoError(t, srv.ContentStore.StoreBlocks(ctx, projectID, "main", []*model.Block{
		model.NewRunsBlock("blk-check", []model.Run{{Text: &model.TextRun{Text: "Hello."}}}),
	}))
	wrapContentStoreLikeTheServer(t, srv)

	const kind = "annotations/qa"
	const payload = `{"findings":[]}`
	srv.executeWriteOverlay(ctx, event.AutomationAction{
		Type:   "write_overlay",
		Config: map[string]string{"kind": kind, "payload": payload},
	}, platev.Event{ProjectID: projectID, Data: map[string]string{"block_id": "blk-check"}}, "")

	bs, err := srv.OpenBlockstore(projectID, "main")
	require.NoError(t, err, "the server's store opens as a blockstore")
	t.Cleanup(func() { _ = bs.Close() })
	sess, err := bs.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })
	got, err := sess.GetOverlay(kind, "blk-check")
	require.NoError(t, err, "the action's overlay landed")
	assert.JSONEq(t, payload, string(got.Payload))
}
