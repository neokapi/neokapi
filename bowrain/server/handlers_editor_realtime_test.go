package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleUpdatePresencePublishesEvent verifies the presence REST endpoint
// publishes an editor.presence.moved event carrying the caller's identity and
// focus, which the change relay fans out to watchers over SSE.
func TestHandleUpdatePresencePublishesEvent(t *testing.T) {
	bus := event.NewChannelEventBus()
	t.Cleanup(bus.Close)
	srv := &Server{EventBus: bus}

	received := make(chan platev.Event, 1)
	sub := bus.SubscribeAll(func(ev platev.Event) {
		if ev.Type == "editor.presence.moved" {
			select {
			case received <- ev:
			default:
			}
		}
	})
	t.Cleanup(func() { bus.Unsubscribe(sub) })

	e := echo.New()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/p1/presence",
		strings.NewReader(`{"item_name":"a.json","block_id":"b1"}`))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("acme", "p1")
	c.Set("user_id", "u1")
	c.Set("name", "Alice")

	require.NoError(t, srv.HandleUpdatePresence(c))
	assert.Equal(t, http.StatusNoContent, rec.Code)

	select {
	case ev := <-received:
		assert.Equal(t, "p1", ev.ProjectID)
		assert.Equal(t, "u1", ev.Data["user_id"])
		assert.Equal(t, "Alice", ev.Data["user_name"])
		assert.Equal(t, "a.json", ev.Data["item_name"])
		assert.Equal(t, "b1", ev.Data["block_id"])
		assert.Equal(t, "presence", ev.Data["event_kind"])
	case <-time.After(2 * time.Second):
		t.Fatal("presence event was not published")
	}
}

// TestHandleUpdatePresenceNoBus is a no-op (204) when no event bus is wired.
func TestHandleUpdatePresenceNoBus(t *testing.T) {
	srv := &Server{}
	e := echo.New()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/p1/presence", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("acme", "p1")

	require.NoError(t, srv.HandleUpdatePresence(c))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}
