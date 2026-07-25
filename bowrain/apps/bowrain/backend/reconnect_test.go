package backend

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoOfflineSetsState(t *testing.T) {
	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = "http://localhost:8080"
	app.mu.Unlock()

	app.goOffline()

	assert.True(t, app.isOffline())
	assert.Equal(t, StateOffline, app.GetConnectionState().State)
}

func TestGoOfflineIdempotent(t *testing.T) {
	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = "http://localhost:8080"
	app.mu.Unlock()

	app.goOffline()
	assert.True(t, app.isOffline())

	// Second call should not panic or change state.
	app.goOffline()
	assert.True(t, app.isOffline())
}

func TestTryReconnectNoServerURL(t *testing.T) {
	app := newTestApp(t)
	assert.False(t, app.tryReconnect())
}

func TestTryReconnectNoAuth(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("BOWRAIN_DESKTOP_CONFIG_DIR", tmpDir)

	app := newTestApp(t)
	app.mu.Lock()
	app.serverURL = "http://localhost:8080"
	app.mu.Unlock()

	// No stored auth → tryReconnect fails.
	assert.False(t, app.tryReconnect())
}

func TestStopReconnectNilCancel(t *testing.T) {
	app := newTestApp(t)
	// Should not panic when reconnectCancel is nil.
	app.stopReconnect()
}

func TestStopReconnectCancelsContext(t *testing.T) {
	app := newTestApp(t)

	// Simulate an active reconnect goroutine.
	cancelled := false
	app.mu.Lock()
	app.reconnectCancel = func() { cancelled = true }
	app.mu.Unlock()

	app.stopReconnect()
	assert.True(t, cancelled)

	// Cancel should be cleared.
	app.mu.RLock()
	assert.Nil(t, app.reconnectCancel)
	app.mu.RUnlock()
}

func TestReplayPendingChangesEmptyQueue(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	// Should not panic with empty queue.
	app.replayPendingChanges(context.Background())
}

func TestReplayPendingChangesNilQueue(t *testing.T) {
	app := newTestApp(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = nil

	// Should not panic with nil queue.
	app.replayPendingChanges(context.Background())
}

func TestReplayPendingChangesNoClient(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	// Enqueue a change.
	_ = q.Enqueue("update_block_target", UpdateBlockRequest{
		ProjectID:    "p1",
		ItemName:     "hello.txt",
		BlockID:      "b1",
		TargetLocale: "fr",
		Text:         "Bonjour",
	})

	// No remote client → replay should fail and mark change as failed.
	app.replayPendingChanges(context.Background())

	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, 1, changes[0].Attempts)
	assert.Contains(t, changes[0].LastError, "not connected")
}

// TestReplayPendingChangesPermanent4xx verifies that a queued change the
// server permanently rejects (4xx — e.g. the review endpoint's 422 for a block
// with no translation) is marked terminally failed after ONE attempt and the
// replay loop drains past it instead of busy-looping on the same change.
func TestReplayPendingChangesPermanent4xx(t *testing.T) {
	calls := 0
	app, _ := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"block has no fr translation to review"}`))
	})
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	_ = q.Enqueue("review_block", reviewBlockOp{
		ProjectID: "p1", ItemName: "hello.txt", BlockID: "b1",
		TargetLocale: "fr", Reviewed: true,
	})

	app.replayPendingChanges(context.Background())

	assert.Equal(t, 1, calls, "a permanently rejected change must not be retried")
	assert.Equal(t, 0, q.PendingCount())
	assert.Equal(t, 1, q.FailedCount())
}

// TestReplayPendingChangesNoProgress verifies that a replay pass in which
// every change fails transiently (server 5xx while the connection stays up)
// stops after one pass instead of spinning a tight retry loop; the change
// stays pending for the next reconnect.
func TestReplayPendingChangesNoProgress(t *testing.T) {
	calls := 0
	app, _ := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	})
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	_ = q.Enqueue("review_block", reviewBlockOp{
		ProjectID: "p1", ItemName: "hello.txt", BlockID: "b1",
		TargetLocale: "fr", Reviewed: true,
	})

	app.replayPendingChanges(context.Background())

	assert.Equal(t, 1, calls, "a no-progress pass must stop, not re-peek immediately")
	assert.Equal(t, 1, q.PendingCount())

	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, 1, changes[0].Attempts)
	assert.Contains(t, changes[0].LastError, "HTTP 500")
}

func TestReplayChangeNoClient(t *testing.T) {
	app := newTestApp(t)

	// With no remote client, replay should return errNotConnected.
	change := PendingChange{
		ID:        1,
		Operation: "update_block_target",
		Payload:   `{"project_id":"p1","block_id":"b1","target_locale":"fr","text":"Bonjour"}`,
	}
	err := app.replayChange(context.Background(), change)
	assert.ErrorIs(t, err, errNotConnected)
}

func TestIsOfflineDefault(t *testing.T) {
	app := newTestApp(t)
	assert.False(t, app.isOffline())
}

func TestIsOfflineAfterGoOffline(t *testing.T) {
	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.mu.Unlock()

	app.goOffline()
	assert.True(t, app.isOffline())
}

func TestEmitConnectionStateNilApp(t *testing.T) {
	app := newTestApp(t)
	// app.app is nil in test — should not panic.
	app.emitConnectionState()
}

// TestOfflineQueueIntegrationWithUpdateBlockTarget verifies that when an app
// is offline, UpdateBlockTarget both updates the local store AND enqueues
// the change for later replay.
func TestOfflineQueueIntegrationWithUpdateBlockTarget(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	// Create a project and add a file.
	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	writeTestFile(t, tmpDir, "hello.txt", "Hello World")
	_, err = app.AddItems(proj.ID, []string{tmpDir + "/hello.txt"})
	require.NoError(t, err)

	blocks, err := app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	require.NotEmpty(t, blocks)

	// Put app in offline state.
	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	// Update a block — should succeed locally and enqueue.
	err = app.UpdateBlockTarget(UpdateBlockRequest{
		ProjectID:    proj.ID,
		ItemName:     "hello.txt",
		BlockID:      blocks[0].ID,
		TargetLocale: "fr",
		Text:         "Bonjour le monde",
	})
	require.NoError(t, err)

	// Verify local update.
	blocks, err = app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", flattenTargetRuns(blocks[0], "fr"))

	// Verify queued.
	assert.Equal(t, 1, q.PendingCount())

	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	assert.Equal(t, "update_block_target", changes[0].Operation)
}

// TestOfflineQueueIntegrationWithReviewBlock verifies offline review queuing.
func TestOfflineQueueIntegrationWithReviewBlock(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	writeTestFile(t, tmpDir, "hello.txt", "Hello World")
	_, err = app.AddItems(proj.ID, []string{tmpDir + "/hello.txt"})
	require.NoError(t, err)

	blocks, err := app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	require.NotEmpty(t, blocks)

	// Set a target first (needed before review).
	_ = app.UpdateBlockTarget(UpdateBlockRequest{
		ProjectID:    proj.ID,
		ItemName:     "hello.txt",
		BlockID:      blocks[0].ID,
		TargetLocale: "fr",
		Text:         "Bonjour",
	})

	// Go offline.
	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	// Review — should succeed locally and enqueue.
	err = app.ReviewBlock(proj.ID, "hello.txt", blocks[0].ID, "fr", true, "")
	require.NoError(t, err)

	// Verify local review: per-locale Target.Status, not the legacy
	// block-global property (write-never since the per-locale migration).
	blocks, err = app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "reviewed", blocks[0].Targets["fr"].Status)
	assert.NotContains(t, blocks[0].Properties, "translation-status")

	// Verify queued.
	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	// At least the review should be queued (UpdateBlockTarget may also be queued).
	found := false
	for _, c := range changes {
		if c.Operation == "review_block" {
			found = true
			break
		}
	}
	assert.True(t, found, "review_block should be in the queue")
}

// TestOfflineQueueIntegrationWithMemory verifies content memory add/delete offline queuing.
func TestOfflineQueueIntegrationWithMemory(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	// Go offline.
	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	// Add content-memory entry — should succeed locally and enqueue.
	entry, err := app.AddMemoryEntry(proj.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)
	assert.NotEmpty(t, entry.ID)

	// Verify queued.
	assert.Equal(t, 1, q.PendingCount())
	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	assert.Equal(t, "add_tm_entry", changes[0].Operation)
}

// TestOfflineQueueIntegrationWithTerms verifies concept add offline queuing.
func TestOfflineQueueIntegrationWithTerms(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	// Go offline.
	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	// Add concept — should succeed locally and enqueue.
	concept, err := app.AddConcept(AddConceptRequest{
		ProjectID:  proj.ID,
		Domain:     "IT",
		Definition: "A program",
		Terms: []TermInfo{
			{Text: "software", Locale: "en", Status: "approved"},
			{Text: "logiciel", Locale: "fr", Status: "approved"},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, concept.ID)

	// Verify queued.
	assert.Equal(t, 1, q.PendingCount())
	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	assert.Equal(t, "add_concept", changes[0].Operation)
}
