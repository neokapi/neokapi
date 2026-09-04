package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// automationRunHub fans out an automation run's transitions to the run's SSE
// subscribers. It is the event.AutomationRunNotifier the run manager and the
// step tracker report to, so a subscriber sees each step start and finish as
// it is persisted rather than on the next snapshot tick.
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

// subscribers reports how many streams follow the run.
func (h *automationRunHub) subscribers(runID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[runID])
}

// RunChanged implements event.AutomationRunNotifier: the transition is
// encoded once and handed to every subscriber of the run.
func (h *automationRunHub) RunChanged(_ context.Context, change event.AutomationRunChange) {
	if change.Run == nil {
		return
	}
	payload, err := encodeAutomationRunFrame(string(change.Kind), change.Run, change.Steps)
	if err != nil {
		return
	}
	h.broadcast(change.Run.ID, payload)
}

func (h *automationRunHub) broadcast(runID string, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients[runID] {
		select {
		case ch <- payload:
		default:
			// A slow subscriber drops the frame; the snapshot tick heals it.
		}
	}
}

// encodeAutomationRunFrame builds the one frame shape the stream carries: the
// kind of transition (or "snapshot"), the run, and its steps. A pushed frame
// and a snapshot are read the same way by a client.
func encodeAutomationRunFrame(kind string, run *bstore.AutomationRun, steps []*bstore.AutomationStep) ([]byte, error) {
	if steps == nil {
		steps = []*bstore.AutomationStep{}
	}
	return json.Marshal(map[string]any{
		"type":  kind,
		"run":   run,
		"steps": steps,
	})
}

// HandleAutomationRunSSE streams live updates for an automation run.
// GET /projects/:id/automation-runs/:runId/events
//
// The stream opens with a snapshot, then delivers every transition the run
// manager and the step tracker push through the hub. A snapshot every three
// seconds is the safety net: it heals a dropped frame and a client that
// connected mid-run, and it decides when the stream closes. Closing on a
// pushed run.finished would be early, because the actions one event
// triggers are grouped into one run and a run can gain a step for a few
// seconds after it settles.
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

	payload, err := encodeAutomationRunFrame("snapshot", run, steps)
	if err != nil {
		return
	}
	fmt.Fprintf(c.Response(), "data: %s\n\n", payload)
	c.Response().Flush()
}
