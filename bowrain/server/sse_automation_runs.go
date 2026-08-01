package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

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

	// Snapshot loop. This stream has always been driven by polling the store
	// rather than by the run itself pushing: there was a hub here to carry
	// live events, but nothing ever called its broadcast, so every update a
	// client saw came from this ticker. The hub is gone rather than left
	// waiting for a producer — see the note above sendRunSnapshot for what
	// making this event-driven would take.
	ticker := time.NewTicker(runSnapshotInterval)
	defer ticker.Stop()

	// Send initial snapshot.
	s.sendRunSnapshot(c, runID)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
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

// runSnapshotInterval is how often a connected client is re-sent the run's
// state. It is the stream's resolution, since nothing pushes.
//
// Making this event-driven is a small change, and worth writing down rather
// than leaving a nolint to imply it: AutomationRunManager (bowrain/event) is
// the producer — it creates the run, creates each step, and moves each step to
// completed or failed. Giving it an optional notifier callback and calling that
// at those transitions, then setting the callback where the run manager is
// constructed in server.go, is all it needs. That was never done, and a hub
// waiting here for it only made the stream look event-driven.
const runSnapshotInterval = 3 * time.Second

// sendRunSnapshot writes the run and its steps to the stream.
func (s *Server) sendRunSnapshot(c echo.Context, runID string) {
	ctx := c.Request().Context()
	run, err := s.AutomationRunStore.GetRun(ctx, runID)
	if err != nil {
		return
	}
	// A failed step listing would otherwise send "steps": null, which the
	// client draws as a run that has no steps rather than one we could not
	// read. Send the run anyway — the next tick may well succeed — but say so.
	steps, err := s.AutomationRunStore.ListSteps(ctx, runID)
	if err != nil {
		slog.WarnContext(ctx, "automation run stream: step listing failed; sending the run without steps",
			"run", runID, "error", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"type":  "snapshot",
		"run":   run,
		"steps": steps,
	})
	fmt.Fprintf(c.Response(), "data: %s\n\n", payload)
	c.Response().Flush()
}
