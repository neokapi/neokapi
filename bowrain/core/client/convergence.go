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
	CreatedAt     time.Time                   `json:"created_at,omitzero"`
	FinishedAt    *time.Time                  `json:"finished_at,omitempty"`
}

// ConvergenceLocaleStanding is one locale's standing within a run — the final
// (or latest) per-locale rollup, matching the convergence.Event locale fields.
type ConvergenceLocaleStanding struct {
	Locale   string `json:"locale"`
	State    string `json:"state"` // shippable | parked | pending
	Units    int    `json:"units,omitempty"`
	Produced int    `json:"produced,omitempty"`
	ViaTM    int    `json:"viaTM,omitempty"`
	ViaAI    int    `json:"viaAI,omitempty"`
}

// StartConvergenceRunRequest is the POST body to start a run.
type StartConvergenceRunRequest struct {
	Trigger string   `json:"trigger"`           // cli | push | manual (defaults to manual server-side)
	Locales []string `json:"locales,omitempty"` // limit to these locales; empty = all pending
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
// onEvent for each convergence.Event, replaying the run's persisted events from
// the start then following live until the terminal EventDone (or ctx is done).
// The connection is long-lived, so it uses a dedicated no-timeout HTTP client
// keyed off ctx for cancellation rather than the client's per-request timeout.
func (c *BowrainClient) StreamConvergenceRunEvents(ctx context.Context, runID string, onEvent func(convergence.Event)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.convergencePrefix()+"/"+runID+"/events", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	c.applyAuth(req)

	streamClient := &http.Client{} // no timeout: SSE is long-lived, ctx cancels it
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("subscribe convergence run events: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("subscribe convergence run events (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Events carry a full run standing per line; give the scanner generous room
	// for a large pending-locale list.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue // SSE comments, event:/id: lines, and blank separators
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
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read convergence event stream: %w", err)
	}
	return nil
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
