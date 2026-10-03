package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
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
	// Nothing to reconnect to is not something a retry can fix, so it reports
	// the terminal kind and the loop stops on it.
	require.ErrorIs(t, app.tryReconnect(context.Background()), errAuthRequired)
}

func TestTryReconnectNoAuth(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)

	app := newTestApp(t)
	app.mu.Lock()
	app.serverURL = "http://localhost:8080"
	app.mu.Unlock()

	// No stored auth → tryReconnect fails, and says why.
	require.ErrorIs(t, app.tryReconnect(context.Background()), errAuthRequired)
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
	app.enqueue(queuedSave(t, "Bonjour"))

	// No remote client → replay should fail and mark change as failed.
	app.replayPendingChanges(context.Background())

	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, 1, changes[0].Attempts)
	assert.Contains(t, changes[0].LastError, "not connected")
}

// TestReplayPendingChangesPermanent4xx verifies that a queued change set the
// server refuses (a 4xx change result, such as a decision on a block with no
// translation) is marked terminally failed after ONE attempt, and the replay
// loop drains past it instead of busy-looping on the same change, while one
// the server could not complete (unreachable) stays pending.
func TestReplayPendingChangesPermanent4xx(t *testing.T) {
	cases := []struct {
		name            string
		status          int
		code            change.Code
		pending, failed int
	}{
		{"a refusal is retired", http.StatusUnprocessableEntity, change.CodeUnsupported, 0, 1},
		{"an unreachable store is retried", http.StatusServiceUnavailable, change.CodeUnreachable, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			app, _ := newGovTestApp(t, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"schema":"kapi.change-result/v1","status":"refused","record":null,"docs":[],"ops":[{"i":0,"op":"decide","status":"refused","error":{"code":"` +
					string(tc.code) + `","message":"block b1 has no fr translation to establish"}}]}`))
			})
			q := newTestQueue(t)
			if app.offlineQueue != nil {
				app.offlineQueue.Close()
			}
			app.offlineQueue = q

			app.enqueue(queuedSave(t, "Bonjour"))
			app.replayPendingChanges(context.Background())

			assert.Equal(t, 1, calls, "one pass sends the change set once")
			assert.Equal(t, tc.pending, q.PendingCount())
			assert.Equal(t, tc.failed, q.FailedCount())
		})
	}
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

	app.enqueue(queuedSave(t, "Bonjour"))

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
	payload, err := json.Marshal(queuedSave(t, "Bonjour"))
	require.NoError(t, err)
	pending := PendingChange{ID: 1, Operation: string(opChangeSet), Payload: string(payload)}
	err = app.replayChange(context.Background(), pending)
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
