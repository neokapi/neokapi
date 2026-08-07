package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v4"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// notificationHub manages per-user WebSocket connections for real-time notifications.
type notificationHub struct {
	mu      sync.RWMutex
	clients map[string]map[*notificationClient]struct{} // userID -> set of clients
}

// notificationClientBuffer is how many pending frames one socket may hold. A
// client further behind than this is not worth waiting for — it will reload
// its inbox on reconnect — and the alternative is holding up everyone else's.
const notificationClientBuffer = 64

type notificationClient struct {
	conn   *websocket.Conn
	userID string

	// Frames queue here rather than being written by the caller. The callers
	// are event-bus handlers running on a consumer group's read loop, and a
	// socket that has stopped reading blocks its write for the full timeout —
	// which, multiplied across a project's members, holds the whole group
	// behind one unresponsive browser tab and pushes the batch past the
	// reclaim threshold, where it is redelivered as if the consumer had died.
	out  chan []byte
	done chan struct{}
}

func newNotificationHub() *notificationHub {
	return &notificationHub{
		clients: make(map[string]map[*notificationClient]struct{}),
	}
}

// newNotificationClient starts one socket's writer. Call stop (via
// removeClient) to shut it down.
func newNotificationClient(conn *websocket.Conn, userID string) *notificationClient {
	c := &notificationClient{
		conn:   conn,
		userID: userID,
		out:    make(chan []byte, notificationClientBuffer),
		done:   make(chan struct{}),
	}
	go c.writeLoop()
	return c
}

func (c *notificationClient) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case msg := <-c.out:
			// Detached: the write goes to a long-lived socket, not to the
			// request that produced the notification; bounded by a timeout.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := c.conn.Write(ctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				slog.Info("notification-ws: failed to write to user", "id", c.userID, "error", err)
			}
		}
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
	if set, ok := h.clients[client.userID]; ok {
		delete(set, client)
		if len(set) == 0 {
			delete(h.clients, client.userID)
		}
	}
	h.mu.Unlock()
	close(client.done)
}

// notifyUser queues a notification for every socket the user has open. It never
// blocks: the send is non-blocking and a client whose buffer is full loses the
// frame rather than stalling the caller.
func (h *notificationHub) notifyUser(userID string, notification *bstore.Notification) {
	h.mu.RLock()
	clients := h.clients[userID]
	if len(clients) == 0 {
		h.mu.RUnlock()
		return
	}
	// Copy client pointers to avoid holding lock during the sends.
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
		select {
		case c.out <- msg:
		default:
			slog.Warn("notification-ws: dropping frame; client buffer full",
				"user_id", c.userID)
		}
	}
}

// HandleNotificationWebSocket handles WebSocket connections for real-time notifications.
// Route: GET /api/v1/:ws/notifications/ws
func (s *Server) HandleNotificationWebSocket(c echo.Context) error {
	userID := getUserID(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	conn, err := websocket.Accept(c.Response().Writer, c.Request(), &websocket.AcceptOptions{
		OriginPatterns: s.wsOriginPatterns(c),
	})
	if err != nil {
		return fmt.Errorf("notification-ws: websocket accept: %w", err)
	}
	defer func() { _ = conn.CloseNow() }()

	client := newNotificationClient(conn, userID)

	s.notificationHub.addClient(client)
	slog.Info("notification-ws: user connected", "user_id", userID)

	defer func() {
		s.notificationHub.removeClient(client)
		slog.Info("notification-ws: user disconnected", "user_id", userID)
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
