package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v4"
)

// collabRoom represents a single collaborative editing room.
// Each room is identified by: workspace:project:file:locale.
type collabRoom struct {
	mu      sync.RWMutex
	clients map[*collabClient]struct{}
}

// collabClient represents a single WebSocket connection to a room.
type collabClient struct {
	conn   *websocket.Conn
	userID string
	name   string
	room   *collabRoom
}

// collabHub manages all active rooms and clients.
type collabHub struct {
	mu    sync.RWMutex
	rooms map[string]*collabRoom
}

func newCollabHub() *collabHub {
	return &collabHub{
		rooms: make(map[string]*collabRoom),
	}
}

// getOrCreateRoom returns the room for the given key, creating it if needed.
func (h *collabHub) getOrCreateRoom(key string) *collabRoom {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, ok := h.rooms[key]
	if !ok {
		room = &collabRoom{
			clients: make(map[*collabClient]struct{}),
		}
		h.rooms[key] = room
	}
	return room
}

// removeClient removes a client from its room and cleans up empty rooms.
func (h *collabHub) removeClient(client *collabClient) {
	room := client.room
	room.mu.Lock()
	delete(room.clients, client)
	empty := len(room.clients) == 0
	room.mu.Unlock()

	if empty {
		h.mu.Lock()
		// Double-check under write lock.
		room.mu.RLock()
		if len(room.clients) == 0 {
			for k, r := range h.rooms {
				if r == room {
					delete(h.rooms, k)
					break
				}
			}
		}
		room.mu.RUnlock()
		h.mu.Unlock()
	}
}

// broadcast sends a message to all clients in the room except the sender.
func (room *collabRoom) broadcast(sender *collabClient, msg []byte) {
	room.mu.RLock()
	defer room.mu.RUnlock()

	for client := range room.clients {
		if client == sender {
			continue
		}
		// Non-blocking write with timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := client.conn.Write(ctx, websocket.MessageBinary, msg); err != nil {
			log.Printf("collab: failed to write to client %s: %v", client.userID, err)
		}
		cancel()
	}
}

// HandleCollabWebSocket handles WebSocket connections for collaborative editing.
// Route: GET /api/v1/workspaces/:ws/editor/projects/:pid/collab/*
// Query params: locale (required)
func (s *Server) HandleCollabWebSocket(c echo.Context) error {
	ws := c.Param("ws")
	pid := c.Param("pid")
	fname := fileParam(c)
	locale := c.QueryParam("locale")

	if locale == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "locale query param required")
	}

	// Extract user info from auth context.
	userID := getUserID(c)
	userName := getUserName(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	roomKey := fmt.Sprintf("%s:%s:%s:%s", ws, pid, fname, locale)

	// Upgrade to WebSocket.
	conn, err := websocket.Accept(c.Response().Writer, c.Request(), &websocket.AcceptOptions{
		Subprotocols:   []string{"yjs"},
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return fmt.Errorf("collab: websocket accept: %w", err)
	}
	defer func() { _ = conn.CloseNow() }()

	// Set read limit (1 MB for Yjs messages).
	conn.SetReadLimit(1 << 20)

	room := s.collabHub.getOrCreateRoom(roomKey)

	client := &collabClient{
		conn:   conn,
		userID: userID,
		name:   userName,
		room:   room,
	}

	// Add client to room.
	room.mu.Lock()
	room.clients[client] = struct{}{}
	room.mu.Unlock()

	log.Printf("collab: user %s (%s) joined room %s", userID, userName, roomKey)

	defer func() {
		s.collabHub.removeClient(client)
		log.Printf("collab: user %s left room %s", userID, roomKey)
	}()

	// Read loop: relay incoming messages to all other clients in the room.
	ctx := c.Request().Context()
	for {
		msgType, msg, err := conn.Read(ctx)
		if err != nil {
			// Normal close or context canceled.
			return nil
		}
		if msgType == websocket.MessageBinary {
			room.broadcast(client, msg)
		}
	}
}

// getUserID extracts the user ID from the Echo context (set by AuthMiddleware).
func getUserID(c echo.Context) string {
	if v := c.Get("user_id"); v != nil {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// getUserName extracts the user name from the Echo context (set by AuthMiddleware).
func getUserName(c echo.Context) string {
	if v := c.Get("user_name"); v != nil {
		if name, ok := v.(string); ok {
			return name
		}
	}
	return ""
}
