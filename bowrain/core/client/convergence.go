package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/convergence"
)

// This file is the client half of the server-side convergence-run protocol
// (strategy 2026-07-kapi-up doc 03): a connected project's `kapi up` pushes,
// starts a run here, streams its convergence.Event feed over SSE (the one
// protocol every venue speaks), and pulls the produced targets. The run
// entity + SSE endpoint live on the server (bowrain/server); this is the
// transport for them.

// ConvergenceRun is the server's view of one convergence run — the JSON the
// REST endpoints return. It mirrors the run entity persisted server-side; the
// per-locale standing rolls up the same convergence.Event stream the run
// emitted.
type ConvergenceRun struct {
	ID            string                      `json:"id"`
	ProjectID     string                      `json:"project_id"`
	Trigger       string                      `json:"trigger"` // cli | push | manual
	State         string                      `json:"state"`   // running | converged | parked | canceled | failed
	Passes        int                         `json:"passes"`
	Locales       []ConvergenceLocaleStanding `json:"locales,omitempty"`
	FailingChecks int                         `json:"failing_checks,omitempty"`
	Error         string                      `json:"error,omitempty"` // set when State is failed/canceled
	// StallReason is the machine-readable cause a run did not converge
	// (needs_credits | source_not_ready | needs_ai_key | …), so the CLI/UI offers
	// the right next action (epic 019). Empty on a converged run.
	StallReason string `json:"stall_reason,omitempty"`
	// CurrentStage/CurrentLocale surface the run's live loop position
	// (settle-source | translate | …); BlockedOnSource is how many source blocks
	// the settle phase held below the gate ("settle your source first").
	CurrentStage    string     `json:"current_stage,omitempty"`
	CurrentLocale   string     `json:"current_locale,omitempty"`
	BlockedOnSource int        `json:"blocked_on_source,omitempty"`
	CreatedAt       time.Time  `json:"created_at,omitzero"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

// ConvergenceLocaleStanding is one locale's standing within a run — the final
// (or latest) per-locale rollup. It is the framework's standing type: the
// server persists what core/convergence.Standing folds out of the event
// stream, and this is the same shape coming back over the wire.
type ConvergenceLocaleStanding = convergence.LocaleStanding

// StartConvergenceRunRequest is the POST body to start a run.
type StartConvergenceRunRequest struct {
	Trigger string   `json:"trigger"`           // cli | push | manual (defaults to manual server-side)
	Locales []string `json:"locales,omitempty"` // limit to these locales; empty = all pending
	// Scope is the epic-019 pre-flight consent scope: all | ready-only | none.
	// Empty defaults to "all". "none" is transport-only (no run started).
	Scope string `json:"scope,omitempty"`
	// Confirmed records explicit estimate confirmation (analytics/audit).
	Confirmed bool `json:"confirmed,omitempty"`
}

// ConvergenceEstimate is the provider-free pre-flight the server computes before
// a run (epic 019, theme B): source readiness first, then the per-locale TM/AI
// split and credit cost for the ready source, then the workspace balance. It
// mirrors the server's convergenceEstimateView.
type ConvergenceEstimate struct {
	Source  ConvergenceSourceReadiness  `json:"source"`
	Locales []ConvergenceEstimateLocale `json:"locales,omitempty"`
	Totals  ConvergenceEstimateTotals   `json:"totals"`
	Credits *ConvergenceEstimateCredits `json:"credits,omitempty"`
	Note    string                      `json:"note,omitempty"`
}

// ConvergenceSourceReadiness is the source-first split: ready vs. held on the
// gate.
type ConvergenceSourceReadiness struct {
	Gate  string `json:"gate"`
	Total int    `json:"total"`
	Ready int    `json:"ready"`
	Held  int    `json:"held"`
}

// ConvergenceEstimateLocale is one locale's estimated work over the ready
// source.
type ConvergenceEstimateLocale struct {
	Locale        string `json:"locale"`
	Pending       int    `json:"pending"`
	ViaTM         int    `json:"via_tm"`
	ViaAI         int    `json:"via_ai"`
	TokenEstimate int    `json:"token_estimate"`
}

// ConvergenceEstimateTotals rolls the per-locale work up across locales.
type ConvergenceEstimateTotals struct {
	Pending       int `json:"pending"`
	ViaTM         int `json:"via_tm"`
	ViaAI         int `json:"via_ai"`
	TokenEstimate int `json:"token_estimate"`
}

// ConvergenceEstimateCredits is the credit/$ side: the AI remainder's cost, the
// workspace balance, and how much of the AI work it covers.
type ConvergenceEstimateCredits struct {
	EstimatedCredits int64   `json:"estimated_credits"`
	EstimatedUSD     float64 `json:"estimated_usd"`
	Balance          int64   `json:"balance"`
	CoversAllAI      bool    `json:"covers_all_ai"`
	CoversAIUnits    int     `json:"covers_ai_units"`
}

// EstimateConvergence fetches the provider-free pre-flight estimate for the
// project's next run. It starts no run and calls no AI provider.
func (c *BowrainClient) EstimateConvergence(ctx context.Context) (*ConvergenceEstimate, error) {
	if c == nil {
		return nil, errors.New("bowrain: project is not connected to a server")
	}
	url := c.projectPrefix() + "/convergence/estimate"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("estimate convergence: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("estimate convergence (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var est ConvergenceEstimate
	if err := json.NewDecoder(resp.Body).Decode(&est); err != nil {
		return nil, fmt.Errorf("decode estimate: %w", err)
	}
	return &est, nil
}

// convergencePrefix is the project-scoped base for run endpoints.
func (c *BowrainClient) convergencePrefix() string {
	return c.projectPrefix() + "/convergence/runs"
}

// StartConvergenceRun asks the server to start (or reuse) a convergence run for
// the project. When a run is already in flight the server returns that run
// (HTTP 200) rather than starting a second; a fresh run returns 201.
func (c *BowrainClient) StartConvergenceRun(ctx context.Context, req StartConvergenceRunRequest) (*ConvergenceRun, error) {
	if c == nil {
		return nil, errors.New("bowrain: project is not connected to a server")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.convergencePrefix(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doRequest(httpReq)
	if err != nil {
		return nil, fmt.Errorf("start convergence run: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("start convergence run (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var run ConvergenceRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return nil, fmt.Errorf("decode convergence run: %w", err)
	}
	return &run, nil
}

// ListConvergenceRuns returns the project's recent runs, newest first. limit
// <= 0 uses the server default.
func (c *BowrainClient) ListConvergenceRuns(ctx context.Context, limit int) ([]ConvergenceRun, error) {
	u := c.convergencePrefix()
	if limit > 0 {
		u += "?limit=" + strconv.Itoa(limit)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("list convergence runs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list convergence runs (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var runs []ConvergenceRun
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		return nil, fmt.Errorf("decode convergence runs: %w", err)
	}
	return runs, nil
}

// GetConvergenceRun fetches one run by ID.
func (c *BowrainClient) GetConvergenceRun(ctx context.Context, runID string) (*ConvergenceRun, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.convergencePrefix()+"/"+runID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("get convergence run: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get convergence run (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var run ConvergenceRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return nil, fmt.Errorf("decode convergence run: %w", err)
	}
	return &run, nil
}

// CancelConvergenceRun requests cancellation of an in-flight run.
func (c *BowrainClient) CancelConvergenceRun(ctx context.Context, runID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.convergencePrefix()+"/"+runID+"/cancel", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := c.doRequest(req)
	if err != nil {
		return fmt.Errorf("cancel convergence run: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel convergence run (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// StreamConvergenceRunEvents subscribes to a run's SSE event stream and calls
// onEvent for each convergence.Event until the terminal EventDone. A run is
// long-lived, so the underlying connection can drop mid-run (proxy idle
// timeout, LB reset, brief network blip) with a CLEAN EOF and no terminal
// frame — treating that as success would make `kapi up` pull partial results
// and report a still-running run as finished. So this reconnects on a
// premature close, resuming from the last seen SSE id (Last-Event-ID) so the
// server replays only events after it and no event is delivered twice; it
// returns only on the terminal frame, a hard error, an HTTP error, or ctx
// cancellation.
func (c *BowrainClient) StreamConvergenceRunEvents(ctx context.Context, runID string, onEvent func(convergence.Event)) error {
	var lastID string
	backoff := 500 * time.Millisecond
	const maxBackoff = 5 * time.Second
	for {
		done, id, err := c.streamOnce(ctx, runID, lastID, onEvent)
		if done {
			return nil // saw the terminal EventDone
		}
		if id != "" {
			lastID = id
		}
		if err != nil {
			// ctx cancellation / deadline is the caller's signal, not a retry.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// A hard error (HTTP status, request build) is terminal; a mere
			// connection drop returns err==nil with done==false and is retried.
			return err
		}
		// Clean EOF before the terminal frame: the run is still going. Wait,
		// then resume from lastID.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
		}
	}
}

// streamOnce reads one SSE connection to exhaustion. It returns done=true when
// the terminal EventDone arrived, the last SSE id it saw (for resume), and a
// non-nil err only for a hard/terminal failure — a clean EOF with no terminal
// frame returns (false, lastID, nil) so the caller reconnects and resumes.
func (c *BowrainClient) streamOnce(ctx context.Context, runID, lastID string, onEvent func(convergence.Event)) (bool, string, error) {
	url := c.convergencePrefix() + "/" + runID + "/events"
	if lastID != "" {
		url += "?after=" + lastID
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, lastID, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	c.applyAuth(req)

	streamClient := &http.Client{} // no timeout: SSE is long-lived, ctx cancels it
	resp, err := streamClient.Do(req)
	if err != nil {
		// A transport error mid-run is a droppable connection, not a hard
		// failure — reconnect and resume (unless ctx was cancelled).
		if ctx.Err() != nil {
			return false, lastID, ctx.Err()
		}
		return false, lastID, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return false, lastID, fmt.Errorf("subscribe convergence run events (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Events carry a full run standing per line; give the scanner generous room
	// for a large pending-locale list.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if id, ok := strings.CutPrefix(line, "id:"); ok {
			lastID = strings.TrimSpace(id)
			continue
		}
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue // SSE comments, event: lines, and blank separators
		}
		data = strings.TrimSpace(data)
		if data == "" {
			continue
		}
		var ev convergence.Event
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue // tolerate a malformed frame rather than abort the stream
		}
		if onEvent != nil {
			onEvent(ev)
		}
		if ev.Type == convergence.EventDone {
			return true, lastID, nil
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return false, lastID, ctx.Err()
		}
		return false, lastID, nil // read error mid-stream → reconnect and resume
	}
	return false, lastID, nil // clean EOF before terminal → reconnect and resume
}

// SetConvergePolicy updates the project's server-side convergence policy
// (on-push | manual) — the recipe's server.converge value, sent to the server
// so a push knows whether to converge on its own clock. It PATCHes the project
// settings; a server that predates the field ignores it (best-effort).
func (c *BowrainClient) SetConvergePolicy(ctx context.Context, policy string) error {
	body, err := json.Marshal(map[string]string{"converge_policy": policy})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.projectPrefix()+"/settings", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return fmt.Errorf("set converge policy: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("set converge policy (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}
