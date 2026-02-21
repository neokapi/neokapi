package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollabHub_GetOrCreateRoom(t *testing.T) {
	hub := newCollabHub()

	room1 := hub.getOrCreateRoom("ws1:proj1:file.html:fr")
	room2 := hub.getOrCreateRoom("ws1:proj1:file.html:fr")
	room3 := hub.getOrCreateRoom("ws1:proj1:file.html:de")

	assert.Same(t, room1, room2, "same key should return same room")
	assert.NotSame(t, room1, room3, "different key should return different room")
}

func TestCollabHub_RemoveClient_CleansUpEmptyRoom(t *testing.T) {
	hub := newCollabHub()
	room := hub.getOrCreateRoom("ws1:proj1:file.html:fr")

	client := &collabClient{userID: "user1", room: room}
	room.mu.Lock()
	room.clients[client] = struct{}{}
	room.mu.Unlock()

	hub.removeClient(client)

	hub.mu.RLock()
	_, exists := hub.rooms["ws1:proj1:file.html:fr"]
	hub.mu.RUnlock()
	assert.False(t, exists, "empty room should be cleaned up")
}

func TestCollabHub_RemoveClient_KeepsNonEmptyRoom(t *testing.T) {
	hub := newCollabHub()
	room := hub.getOrCreateRoom("ws1:proj1:file.html:fr")

	client1 := &collabClient{userID: "user1", room: room}
	client2 := &collabClient{userID: "user2", room: room}
	room.mu.Lock()
	room.clients[client1] = struct{}{}
	room.clients[client2] = struct{}{}
	room.mu.Unlock()

	hub.removeClient(client1)

	hub.mu.RLock()
	_, exists := hub.rooms["ws1:proj1:file.html:fr"]
	hub.mu.RUnlock()
	assert.True(t, exists, "room with remaining clients should not be cleaned up")
}

func TestCollabWebSocket_RequiresLocale(t *testing.T) {
	s := &Server{collabHub: newCollabHub()}
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/ws1/editor/projects/p1/collab/file.html", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "pid", "fname")
	c.SetParamValues("ws1", "p1", "file.html")

	err := s.HandleCollabWebSocket(c)
	require.Error(t, err)
	he, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, he.Code)
}

func TestCollabWebSocket_RequiresAuth(t *testing.T) {
	s := &Server{collabHub: newCollabHub()}
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/ws1/editor/projects/p1/collab/file.html?locale=fr", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "pid", "fname")
	c.SetParamValues("ws1", "p1", "file.html")

	err := s.HandleCollabWebSocket(c)
	require.Error(t, err)
	he, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusUnauthorized, he.Code)
}

func TestCollabWebSocket_RelayMessages(t *testing.T) {
	s := &Server{collabHub: newCollabHub()}
	e := echo.New()
	e.GET("/ws/:ws/:pid/:fname", s.HandleCollabWebSocket, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Inject auth context for test.
			c.Set("user_id", c.Request().Header.Get("X-Test-User-ID"))
			c.Set("user_name", c.Request().Header.Get("X-Test-User-Name"))
			return next(c)
		}
	})

	srv := httptest.NewServer(e)
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:] // http -> ws

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect two clients to the same room.
	headers1 := http.Header{}
	headers1.Set("X-Test-User-ID", "user1")
	headers1.Set("X-Test-User-Name", "Alice")
	conn1, _, err := websocket.Dial(ctx, wsURL+"/ws/acme/proj1/file.html?locale=fr", &websocket.DialOptions{
		Subprotocols:  []string{"yjs"},
		HTTPHeader:    headers1,
	})
	require.NoError(t, err)
	defer conn1.CloseNow()

	headers2 := http.Header{}
	headers2.Set("X-Test-User-ID", "user2")
	headers2.Set("X-Test-User-Name", "Bob")
	conn2, _, err := websocket.Dial(ctx, wsURL+"/ws/acme/proj1/file.html?locale=fr", &websocket.DialOptions{
		Subprotocols:  []string{"yjs"},
		HTTPHeader:    headers2,
	})
	require.NoError(t, err)
	defer conn2.CloseNow()

	// Give server time to register both clients.
	time.Sleep(100 * time.Millisecond)

	// Send a binary message from conn1.
	testMsg := []byte{0x01, 0x02, 0x03}
	err = conn1.Write(ctx, websocket.MessageBinary, testMsg)
	require.NoError(t, err)

	// conn2 should receive the relayed message.
	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()
	msgType, data, err := conn2.Read(readCtx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageBinary, msgType)
	assert.Equal(t, testMsg, data)

	// conn1 should NOT receive its own message (no echo).
	// Use a short timeout for this negative test.
	noEchoCtx, noEchoCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer noEchoCancel()
	_, _, err = conn1.Read(noEchoCtx)
	assert.Error(t, err, "sender should not receive their own message")
}
