package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncRecorder is a minimal thread-safe http.ResponseWriter (+ http.Flusher)
// for SSE tests. The handler streams frames from its own goroutine while the
// test reads the accumulated body, so every access is mutex-guarded —
// httptest.ResponseRecorder is not safe for concurrent read/write, which the
// race detector flags. Flush is a no-op so the handler's Flush() never panics.
type syncRecorder struct {
	mu   sync.Mutex
	hdr  http.Header
	buf  bytes.Buffer
	code int
}

func newSyncRecorder() *syncRecorder {
	return &syncRecorder{hdr: make(http.Header), code: http.StatusOK}
}

func (r *syncRecorder) Header() http.Header { return r.hdr }

func (r *syncRecorder) WriteHeader(code int) {
	r.mu.Lock()
	r.code = code
	r.mu.Unlock()
}

func (r *syncRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

func (r *syncRecorder) Flush() {}

func (r *syncRecorder) body() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

func (r *syncRecorder) statusCode() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.code
}

func TestHandleWorkspaceEventsSSE_StreamsEvents(t *testing.T) {
	bus := event.NewChannelEventBus()
	defer bus.Close()
	srv := &Server{EventBus: bus}
	srv.changeRelay = event.NewChangeRelay(bus, nil)
	defer srv.changeRelay.Close()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/acme/events?project=p1", nil)
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	rec := newSyncRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws-1")

	done := make(chan error, 1)
	go func() { done <- srv.HandleWorkspaceEventsSSE(c) }()

	// Wait for the relay client to register before publishing.
	require.Eventually(t, func() bool { return srv.changeRelay.ClientCount() == 1 },
		2*time.Second, 10*time.Millisecond)

	bus.Publish(platev.Event{
		Type:      platev.EventBlockUpdated,
		ProjectID: "p1",
		Data:      map[string]string{"block_id": "b1", "item_name": "home.json"},
	})

	// Give the handler time to write the frame.
	require.Eventually(t, func() bool {
		return strings.Contains(rec.body(), `"type":"block.updated"`)
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after context cancel")
	}

	body := rec.body()
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Contains(t, body, ": connected")
	assert.Contains(t, body, "event: change")
	assert.Contains(t, body, `"blockId":"b1"`)
	assert.Contains(t, body, `"itemName":"home.json"`)

	// Client must be cleaned up on disconnect.
	assert.Eventually(t, func() bool { return srv.changeRelay.ClientCount() == 0 },
		2*time.Second, 10*time.Millisecond)
}

func TestHandleWorkspaceEventsSSE_RequiresWorkspace(t *testing.T) {
	bus := event.NewChannelEventBus()
	defer bus.Close()
	srv := &Server{EventBus: bus}
	srv.changeRelay = event.NewChangeRelay(bus, nil)
	defer srv.changeRelay.Close()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/acme/events", nil)
	rec := newSyncRecorder()
	c := e.NewContext(req, rec)
	// No workspace_id set → forbidden.

	err := srv.HandleWorkspaceEventsSSE(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, rec.statusCode())
}

func TestHandleWorkspaceEventsSSE_RelayUnconfigured(t *testing.T) {
	srv := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/acme/events", nil)
	rec := newSyncRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws-1")

	err := srv.HandleWorkspaceEventsSSE(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.statusCode())
}
