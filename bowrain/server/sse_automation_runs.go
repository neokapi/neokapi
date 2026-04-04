package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// automationRunHub manages SSE connections for live automation run updates.
type automationRunHub struct {
	mu      sync.RWMutex
	clients map[string]map[chan []byte]struct{} // runID → set of channels
}

func newAutomationRunHub() *automationRunHub {
	return &automationRunHub{
		clients: make(map[string]map[chan []byte]struct{}),
	}
}

func (h *automationRunHub) subscribe(runID string) chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan []byte, 16)
	if h.clients[runID] == nil {
		h.clients[runID] = make(map[chan []byte]struct{})
	}
	h.clients[runID][ch] = struct{}{}
	return ch
}

func (h *automationRunHub) unsubscribe(runID string, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.clients[runID]; ok {
		delete(m, ch)
		if len(m) == 0 {
			delete(h.clients, runID)
		}
	}
	close(ch)
}

func (h *automationRunHub) broadcast(runID string, eventType string, data any) { //nolint:unused // will be used by RunManager for live push
	payload, err := json.Marshal(map[string]any{"type": eventType, "data": data})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients[runID] {
		select {
		case ch <- payload:
		default:
			// Drop if client is slow.
		}
	}
}

// HandleAutomationRunSSE streams live updates for an automation run.
// GET /projects/:id/automation-runs/:runId/events
func (s *Server) HandleAutomationRunSSE(c echo.Context) error {
	if s.AutomationRunStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "automation runs not configured"})
	}

	runID := c.Param("runId")

	// Set SSE headers.
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	ctx := c.Request().Context()

	// Subscribe to live updates if hub is available.
	var ch chan []byte
	if s.runHub != nil {
		ch = s.runHub.subscribe(runID)
		defer s.runHub.unsubscribe(runID, ch)
	}

	// Poll + stream loop.
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// Send initial snapshot.
	s.sendRunSnapshot(c, runID)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			fmt.Fprintf(c.Response(), "data: %s\n\n", msg)
			c.Response().Flush()
		case <-ticker.C:
			// Send periodic snapshot for clients that missed events.
			s.sendRunSnapshot(c, runID)

			// Check if run is complete — close stream.
			run, err := s.AutomationRunStore.GetRun(ctx, runID)
			if err != nil {
				return nil
			}
			if run.Status == bstore.RunStatusCompleted || run.Status == bstore.RunStatusFailed || run.Status == bstore.RunStatusPartial {
				s.sendRunSnapshot(c, runID)
				fmt.Fprintf(c.Response(), "event: done\ndata: {}\n\n")
				c.Response().Flush()
				return nil
			}
		}
	}
}

func (s *Server) sendRunSnapshot(c echo.Context, runID string) {
	ctx := c.Request().Context()
	run, err := s.AutomationRunStore.GetRun(ctx, runID)
	if err != nil {
		return
	}
	steps, _ := s.AutomationRunStore.ListSteps(ctx, runID)

	payload, _ := json.Marshal(map[string]any{
		"type":  "snapshot",
		"run":   run,
		"steps": steps,
	})
	fmt.Fprintf(c.Response(), "data: %s\n\n", payload)
	c.Response().Flush()
}
