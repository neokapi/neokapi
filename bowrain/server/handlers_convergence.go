package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/convergence"
)

// convergenceRunView is the REST shape of a run — the JSON the kapi-bowrain
// client (bowrain/core/client.ConvergenceRun) decodes. It renames the stored
// `standing` to `locales` and carries RFC3339 timestamps.
type convergenceRunView struct {
	ID            string                             `json:"id"`
	ProjectID     string                             `json:"project_id"`
	Trigger       string                             `json:"trigger"`
	State         string                             `json:"state"`
	Passes        int                                `json:"passes"`
	Locales       []bstore.ConvergenceLocaleStanding `json:"locales,omitempty"`
	FailingChecks int                                `json:"failing_checks,omitempty"`
	CreatedAt     string                             `json:"created_at,omitempty"`
	FinishedAt    string                             `json:"finished_at,omitempty"`
}

func toConvergenceRunView(r *bstore.ConvergenceRun) convergenceRunView {
	v := convergenceRunView{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		Trigger:       r.Trigger,
		State:         r.State,
		Passes:        r.Passes,
		Locales:       r.Standing,
		FailingChecks: r.FailingChecks,
	}
	if !r.CreatedAt.IsZero() {
		v.CreatedAt = r.CreatedAt.UTC().Format(time.RFC3339)
	}
	if r.FinishedAt != nil && !r.FinishedAt.IsZero() {
		v.FinishedAt = r.FinishedAt.UTC().Format(time.RFC3339)
	}
	return v
}

// startConvergenceRunRequest is the POST body to start a run.
type startConvergenceRunRequest struct {
	Trigger string   `json:"trigger"`
	Locales []string `json:"locales,omitempty"`
}

// HandleStartConvergenceRun starts (or joins) a project convergence run.
// POST /…/projects/:id/convergence/runs
func (s *Server) HandleStartConvergenceRun(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermTranslate); err != nil {
		return err
	}
	if s.convergence == nil || s.ConvergenceRunStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "convergence runs not configured"})
	}
	var req startConvergenceRunRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	trigger := req.Trigger
	if trigger == "" {
		trigger = "manual"
	}
	run, created, err := s.convergence.StartRun(c.Request().Context(), c.Param("id"), trigger, req.Locales)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	status := http.StatusOK // an already-running run is returned, not re-created
	if created {
		status = http.StatusCreated
	}
	return c.JSON(status, toConvergenceRunView(run))
}

// HandleListConvergenceRuns returns a project's runs, newest first.
// GET /…/projects/:id/convergence/runs?limit=N
func (s *Server) HandleListConvergenceRuns(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ConvergenceRunStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "convergence runs not configured"})
	}
	limit := 20
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	runs, err := s.ConvergenceRunStore.ListRuns(c.Request().Context(), c.Param("id"), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	views := make([]convergenceRunView, 0, len(runs))
	for _, r := range runs {
		views = append(views, toConvergenceRunView(r))
	}
	return c.JSON(http.StatusOK, views)
}

// HandleGetConvergenceRun returns one run by ID.
// GET /…/projects/:id/convergence/runs/:runID
func (s *Server) HandleGetConvergenceRun(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ConvergenceRunStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "convergence runs not configured"})
	}
	run, err := s.ConvergenceRunStore.GetRun(c.Request().Context(), c.Param("runID"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "run not found"})
	}
	return c.JSON(http.StatusOK, toConvergenceRunView(run))
}

// HandleCancelConvergenceRun requests cancellation of an in-flight run.
// POST /…/projects/:id/convergence/runs/:runID/cancel
func (s *Server) HandleCancelConvergenceRun(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermTranslate); err != nil {
		return err
	}
	if s.convergence == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "convergence runs not configured"})
	}
	s.convergence.Cancel(c.Param("runID"))
	return c.NoContent(http.StatusOK)
}

// HandleConvergenceRunSSE streams a run's convergence.Event feed: it replays
// the persisted events from seq 0, then follows live until the terminal done
// event (or the client disconnects). One `data: <Event JSON>` frame per event —
// the exact protocol the CLI renders a local run from.
// GET /…/projects/:id/convergence/runs/:runID/events
func (s *Server) HandleConvergenceRunSSE(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.convergence == nil || s.ConvergenceRunStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "convergence runs not configured"})
	}
	runID := c.Param("runID")

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	ctx := c.Request().Context()

	// Subscribe to live frames BEFORE replaying the store so no event emitted
	// mid-replay is lost; dedupe by sequence.
	ch := s.convergence.hub.subscribe(runID)
	defer s.convergence.hub.unsubscribe(runID, ch)

	writeFrame := func(payload []byte) bool {
		if _, err := fmt.Fprintf(c.Response(), "data: %s\n\n", payload); err != nil {
			return false
		}
		c.Response().Flush()
		return isTerminalEvent(payload)
	}

	// Replay persisted events.
	seqs, payloads, err := s.ConvergenceRunStore.ListEvents(ctx, runID, 0)
	if err != nil {
		return nil
	}
	lastSeq := -1
	for i, payload := range payloads {
		lastSeq = seqs[i]
		if writeFrame(payload) {
			return nil // the run already finished; the done frame closes it
		}
	}

	// Follow live.
	for {
		select {
		case <-ctx.Done():
			return nil
		case frame, ok := <-ch:
			if !ok {
				return nil
			}
			if frame.seq <= lastSeq {
				continue // already replayed
			}
			lastSeq = frame.seq
			if writeFrame(frame.data) {
				return nil
			}
		}
	}
}

// isTerminalEvent reports whether a persisted event payload is the run's
// closing `done` frame.
func isTerminalEvent(payload []byte) bool {
	var probe struct {
		Type convergence.EventType `json:"type"`
	}
	if err := json.Unmarshal(payload, &probe); err != nil {
		return false
	}
	return probe.Type == convergence.EventDone
}

// updateProjectSettingsRequest is the PATCH body for project settings the kapi
// client sends to keep server-side policy in step with the recipe.
type updateProjectSettingsRequest struct {
	ConvergePolicy string `json:"converge_policy,omitempty"`
}

// HandleUpdateProjectSettings applies client-sent project settings (currently
// the server-side convergence policy). It is the seam the kapi-bowrain plugin
// uses to push the recipe's server.converge value before a push/up.
// PATCH /…/projects/:id/settings
func (s *Server) HandleUpdateProjectSettings(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
		return err
	}
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	var req updateProjectSettingsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	ctx := c.Request().Context()
	proj, err := s.Services.Project.GetProject(ctx, c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	if req.ConvergePolicy != "" {
		proj.ConvergePolicy = store.NormalizeConvergePolicy(req.ConvergePolicy)
	}
	if err := s.Services.Project.UpdateProject(ctx, proj); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusOK)
}
