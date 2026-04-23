package server

import (
	"context"
	"testing"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/stretchr/testify/require"
)

// TestAutomation_WriteOverlay_EndToEnd proves #401: the in-process
// blockstore adapter (from #385) is reachable from the automation
// engine, and a rule-driven `write_overlay` action persists to the
// same `block_overlays` table a CLI flow would write to.
//
// Uses the Postgres test harness (pgtest) like the rest of the
// automation workflow tests — SQLite dev mode is covered by the
// adapter's own tests in bowrain/store/blockstore/.
func TestAutomation_WriteOverlay_EndToEnd(t *testing.T) {
	cfg := DefaultConfig()
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := context.Background()

	// Seed a project so the blockstore adapter has something to bind to.
	projectID := "proj-automation-overlay"
	err := srv.ContentStore.CreateProject(ctx, &platstore.Project{
		ID:                    projectID,
		Name:                  "Overlay E2E",
		DefaultSourceLanguage: "en",
	})
	require.NoError(t, err)

	// Fire the action directly (no event-bus routing — the engine loop
	// is exercised by other workflow tests). The action payload mimics
	// what a rule on `content.extracted` would dispatch.
	const wantKind = "annotations/qa"
	const wantBlock = "block-hash-abc"
	const wantPayload = `{"findings":[{"rule":"punctuation","severity":"info"}]}`

	action := event.AutomationAction{
		Type: "write_overlay",
		Name: "qa-on-extract",
		Config: map[string]string{
			"kind":    wantKind,
			"payload": wantPayload,
		},
	}
	ev := platev.Event{
		Type:      "content.extracted",
		ProjectID: projectID,
		Data:      map[string]string{"block_id": wantBlock},
	}

	// stepID="" skips AutomationRunStore logging (that surface is
	// covered separately). This test asserts the in-process Store path.
	srv.executeWriteOverlay(ctx, action, ev, "")

	// Now open the blockstore directly and assert the overlay landed.
	bs, err := srv.OpenBlockstore(projectID, "main")
	require.NoError(t, err)
	t.Cleanup(func() { _ = bs.Close() })

	sess, err := bs.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	got, err := sess.GetOverlay(wantKind, wantBlock)
	require.NoError(t, err, "overlay should be retrievable via the same adapter")
	require.Equal(t, wantKind, got.Kind)
	require.Equal(t, wantBlock, got.BlockHash)
	require.JSONEq(t, wantPayload, string(got.Payload))
}

// TestAutomation_WriteOverlay_MissingInputs exercises the error paths
// so an invalid rule configuration doesn't silently no-op.
func TestAutomation_WriteOverlay_MissingInputs(t *testing.T) {
	cfg := DefaultConfig()
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := context.Background()
	require.NoError(t, srv.ContentStore.CreateProject(ctx, &platstore.Project{
		ID:                    "proj-invalid",
		Name:                  "Invalid",
		DefaultSourceLanguage: "en",
	}))

	// Missing kind — no overlay should land.
	srv.executeWriteOverlay(ctx, event.AutomationAction{
		Type:   "write_overlay",
		Config: map[string]string{"payload": "{}"},
	}, platev.Event{ProjectID: "proj-invalid", Data: map[string]string{"block_id": "b1"}}, "")

	bs, _ := srv.OpenBlockstore("proj-invalid", "main")
	sess, _ := bs.Begin(ctx)
	defer sess.Close()
	_, err := sess.GetOverlay("annotations/qa", "b1")
	require.ErrorIs(t, err, blockstore.ErrNotFound, "missing kind should produce no overlay")
}

// Compile-time sanity: OpenBlockstore must produce a working
// blockstore.Store. Keeps the integration point from regressing.
func TestAutomation_OpenBlockstore_Works(t *testing.T) {
	cfg := DefaultConfig()
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := context.Background()
	require.NoError(t, srv.ContentStore.CreateProject(ctx, &platstore.Project{
		ID:                    "proj-wire",
		Name:                  "Wire",
		DefaultSourceLanguage: "en",
	}))

	bs, err := srv.OpenBlockstore("proj-wire", "main")
	require.NoError(t, err)
	require.True(t, bs.Capabilities().Writable)
	require.True(t, bs.Capabilities().RandomAccess)
}

// Keep the import hooked so removing the package's Open in a future
// refactor flags this file too.
var _ = func() blockstore.Store {
	var s *Server
	if s == nil {
		return nil
	}
	bs, _ := s.OpenBlockstore("", "")
	return bs
}

// Keep references to bstore + goimports happy if fields shift.
var _ = bstore.NewAutomationRunStore
