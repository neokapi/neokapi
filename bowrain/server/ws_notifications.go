package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	bstore "github.com/gokapi/gokapi/bowrain/store"
	"github.com/labstack/echo/v4"
)

// notificationHub manages per-user WebSocket connections for real-time notifications.
type notificationHub struct {
	mu      sync.RWMutex
	clients map[string]map[*notificationClient]struct{} // userID -> set of clients
}

type notificationClient struct {
	conn   *websocket.Conn
	userID string
}

func newNotificationHub() *notificationHub {
	return &notificationHub{
		clients: make(map[string]map[*notificationClient]struct{}),
	}
}

func (h *notificationHub) addClient(client *notificationClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[client.userID] == nil {
		h.clients[client.userID] = make(map[*notificationClient]struct{})
	}
	h.clients[client.userID][client] = struct{}{}
}

func (h *notificationHub) removeClient(client *notificationClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.clients[client.userID]; ok {
		delete(set, client)
		if len(set) == 0 {
			delete(h.clients, client.userID)
		}
	}
}

// notifyUser sends a notification to all connected WebSocket clients for a user.
func (h *notificationHub) notifyUser(userID string, notification *bstore.Notification) {
	h.mu.RLock()
	clients := h.clients[userID]
	if len(clients) == 0 {
		h.mu.RUnlock()
		return
	}
	// Copy client pointers to avoid holding lock during writes.
	targets := make([]*notificationClient, 0, len(clients))
	for c := range clients {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	msg, err := json.Marshal(map[string]any{
		"type":         "notification",
		"notification": notification,
	})
	if err != nil {
		return
	}

	for _, c := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.conn.Write(ctx, websocket.MessageText, msg); err != nil {
			log.Printf("notification-ws: failed to write to user %s: %v", c.userID, err)
		}
		cancel()
	}
}

// HandleNotificationWebSocket handles WebSocket connections for real-time notifications.
// Route: GET /api/v1/workspaces/:ws/notifications/ws
func (s *Server) HandleNotificationWebSocket(c echo.Context) error {
	userID := getUserID(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	conn, err := websocket.Accept(c.Response().Writer, c.Request(), &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return fmt.Errorf("notification-ws: websocket accept: %w", err)
	}
	defer func() { _ = conn.CloseNow() }()

	client := &notificationClient{
		conn:   conn,
		userID: userID,
	}

	s.notificationHub.addClient(client)
	log.Printf("notification-ws: user %s connected", userID)

	defer func() {
		s.notificationHub.removeClient(client)
		log.Printf("notification-ws: user %s disconnected", userID)
	}()

	// Read loop: keep connection alive, handle pings.
	ctx := c.Request().Context()
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			return nil
		}
	}
}

// NotifyUser sends a real-time notification via WebSocket. Safe to call even if hub is nil.
func (s *Server) NotifyUser(userID string, notification *bstore.Notification) {
	if s.notificationHub != nil {
		s.notificationHub.notifyUser(userID, notification)
	}
}
