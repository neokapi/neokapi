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
	// Error is the terminal cause for a failed/canceled run — the message the
	// CLI prints so `kapi up` explains why it didn't converge. Empty otherwise.
	Error string `json:"error,omitempty"`
	// StallReason is the machine-readable cause a run did not converge
	// (needs_credits | needs_ai_key | rate_limited | no_progress |
	// checks_failing), so a client distinguishes "out of credits" from "pending
	// review" and offers the right next action (theme C). Empty on converge.
	StallReason string `json:"stall_reason,omitempty"`
	// CurrentStage/CurrentLocale/LastActivity surface the run's live loop
	// position + heartbeat (theme D): a frozen LastActivity while awaiting jobs
	// reads as stalled, not merely slow.
	CurrentStage  string `json:"current_stage,omitempty"`
	CurrentLocale string `json:"current_locale,omitempty"`
	LastActivity  string `json:"last_activity,omitempty"`
	// BlockedOnSource is how many source blocks the source-first settle phase held
	// below the gate (epic 019): the UI renders it as "N blocks need settling /
	// source review" on a source_not_ready hold. Omitted when zero.
	BlockedOnSource int    `json:"blocked_on_source,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	FinishedAt      string `json:"finished_at,omitempty"`
}

func toConvergenceRunView(r *bstore.ConvergenceRun) convergenceRunView {
	v := convergenceRunView{
		ID:              r.ID,
		ProjectID:       r.ProjectID,
		Trigger:         r.Trigger,
		State:           r.State,
		Passes:          r.Passes,
		Locales:         r.Standing,
		FailingChecks:   r.FailingChecks,
		Error:           r.Error,
		StallReason:     r.StallReason,
		CurrentStage:    r.CurrentStage,
		CurrentLocale:   r.CurrentLocale,
		BlockedOnSource: r.BlockedOnSource,
	}
	if !r.CreatedAt.IsZero() {
		v.CreatedAt = r.CreatedAt.UTC().Format(time.RFC3339)
	}
	if r.FinishedAt != nil && !r.FinishedAt.IsZero() {
		v.FinishedAt = r.FinishedAt.UTC().Format(time.RFC3339)
	}
	if r.LastActivity != nil && !r.LastActivity.IsZero() {
		v.LastActivity = r.LastActivity.UTC().Format(time.RFC3339)
	}
	return v
}

// Convergence run scope (roadmap epic 019, theme B2): the explicit translation
// scope the pre-flight consent picks, so a large run is never started blind.
// The source gate is orthogonal to scope — source-held blocks never translate
// regardless — so "all" and "ready-only" both honor the gate; the difference is
// only whether the caller consented to the full estimate.
const (
	// ConvergenceScopeAll translates every pending locale over the ready source —
	// the full estimate the consent dialog priced. The default when no scope is
	// given (preserving the pre-019 on-push/CLI behavior).
	ConvergenceScopeAll = "all"
	// ConvergenceScopeReadyOnly is the same gated fan-out as "all"; it is the
	// explicit "translate only the ready source" consent label. Held source never
	// translates under either scope, so this is a semantic marker for the UI and
	// analytics, not a second code path.
	ConvergenceScopeReadyOnly = "ready-only"
	// ConvergenceScopeNone is transport-only: no translation run is started. The
	// consent dialog's [Transport only] choice maps here so "push, don't
	// translate" is a first-class, side-effect-free outcome (204, no run).
	ConvergenceScopeNone = "none"
)

// startConvergenceRunRequest is the POST body to start a run. Scope and
// Confirmed are the epic-019 pre-flight fields: a UI/CLI that showed the
// estimate sets them so a large run isn't started blind. An omitted Scope
// defaults to "all" (the pre-019 behavior), so existing callers are unaffected.
type startConvergenceRunRequest struct {
	Trigger string   `json:"trigger"`
	Locales []string `json:"locales,omitempty"`
	// Scope is the translation scope the consent picked: all | ready-only | none.
	// Empty defaults to "all". "none" is transport-only — no run is started.
	Scope string `json:"scope,omitempty"`
	// Confirmed records that the caller explicitly confirmed the estimate — carried
	// for analytics/audit. The client owns the gate; the server never starts a run
	// for Scope "none" but does not otherwise refuse an unconfirmed run.
	Confirmed bool `json:"confirmed,omitempty"`
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
	// Transport-only scope: the caller consented to push without translating, so
	// no run is started. Answer 204 so the client distinguishes it from a started
	// (201) or joined (200) run.
	if req.Scope == ConvergenceScopeNone {
		return c.NoContent(http.StatusNoContent)
	}
	trigger := req.Trigger
	if trigger == "" {
		trigger = "manual"
	}
	run, created, err := s.convergence.StartRun(c.Request().Context(), c.Param("id"), trigger, req.Locales)
	if err != nil {
		return serverErr(c, err)
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
		return serverErr(c, err)
	}
	views := make([]convergenceRunView, 0, len(runs))
	for _, r := range runs {
		views = append(views, toConvergenceRunView(r))
	}
	return c.JSON(http.StatusOK, views)
}

// runForProject fetches the :runID run and enforces that it belongs to the
// authorized project (the :id the access middleware validated). A run owned by
// another project reports not-found so the caller answers 404 — never 403, so
// run existence across projects is not leaked (F1: cross-project IDOR).
func (s *Server) runForProject(c echo.Context) (*bstore.ConvergenceRun, bool) {
	run, err := s.ConvergenceRunStore.GetRun(c.Request().Context(), c.Param("runID"))
	if err != nil || run == nil || run.ProjectID != c.Param("id") {
		return nil, false
	}
	return run, true
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
	run, ok := s.runForProject(c)
	if !ok {
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
	if s.convergence == nil || s.ConvergenceRunStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "convergence runs not configured"})
	}
	run, ok := s.runForProject(c)
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "run not found"})
	}
	// Cancel persists the terminal state even when no in-memory cancel func
	// exists (e.g. a run adopted from another replica or across a restart), so
	// the DB row never stays 'running' (F3). The bool reports whether a live
	// loop was signaled.
	signaled, err := s.convergence.Cancel(c.Request().Context(), run)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"canceled": true, "signaled": signaled})
}

// resumeSeq is the last event sequence a reconnecting SSE client already saw,
// so the stream resumes past it. It reads the standard Last-Event-ID request
// header (native EventSource auto-sends it on reconnect) and falls back to an
// explicit ?after=<seq> query param. An absent/invalid value means -1 (stream
// from the beginning).
func resumeSeq(c echo.Context) int {
	raw := c.Request().Header.Get("Last-Event-ID")
	if raw == "" {
		raw = c.QueryParam("after")
	}
	if raw == "" {
		return -1
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return -1
	}
	return n
}

// HandleConvergenceRunSSE streams a run's convergence.Event feed: it replays
// the persisted events (from the client's resume point, or the beginning),
// then follows live until the terminal done event (or the client disconnects).
// Each frame is `id: <seq>\ndata: <Event JSON>` — the exact event protocol the
// CLI renders a local run from, with the persisted seq as the resumable id.
// GET /…/projects/:id/convergence/runs/:runID/events
func (s *Server) HandleConvergenceRunSSE(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.convergence == nil || s.ConvergenceRunStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "convergence runs not configured"})
	}
	if _, ok := s.runForProject(c); !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "run not found"})
	}
	runID := c.Param("runID")

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	ctx := c.Request().Context()

	// Resume point: a reconnecting client resumes past the last event it saw
	// (Last-Event-ID header, or ?after=<seq>), so a reconnect never re-delivers
	// seen events — which is also what keeps a native-EventSource web consumer
	// from double-folding the stream.
	lastSeq := resumeSeq(c)

	// Subscribe to live frames BEFORE replaying the store so no event emitted
	// mid-replay is lost; dedupe by sequence.
	ch := s.convergence.hub.subscribe(runID)
	defer s.convergence.hub.unsubscribe(runID, ch)

	writeFrame := func(seq int, payload []byte) bool {
		if seq <= lastSeq {
			return false // already delivered (or before the resume point)
		}
		lastSeq = seq
		// The SSE id is the persisted, monotonic seq — the value a client echoes
		// back as Last-Event-ID to resume exactly where it left off.
		if _, err := fmt.Fprintf(c.Response(), "id: %d\ndata: %s\n\n", seq, payload); err != nil {
			return false
		}
		c.Response().Flush()
		return isTerminalEvent(payload)
	}

	// drainStore delivers any persisted events past lastSeq — the source of
	// truth. It is the recovery path for a frame the hub dropped (a full/slow
	// subscriber channel), so a subscriber that stays connected to a finished
	// run always receives the terminal done frame. Returns true once done is
	// delivered.
	drainStore := func() bool {
		seqs, payloads, err := s.ConvergenceRunStore.ListEvents(ctx, runID, lastSeq+1)
		if err != nil {
			return false
		}
		for i, payload := range payloads {
			if writeFrame(seqs[i], payload) {
				return true
			}
		}
		return false
	}

	// Replay everything persisted so far.
	if drainStore() {
		return nil
	}

	// Follow live, with a periodic store reconciliation that heals any dropped
	// hub frame and guarantees the terminal frame is eventually delivered.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case frame, ok := <-ch:
			if !ok {
				return nil
			}
			if writeFrame(frame.seq, frame.data) {
				return nil
			}
		case <-ticker.C:
			if drainStore() {
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
		return serverErr(c, err)
	}
	return c.NoContent(http.StatusOK)
}
