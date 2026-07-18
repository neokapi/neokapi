package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/core/version"
)

// AdminHealthResponse is the payload for the ctrl Health dashboard: the server's
// own readiness, the worker's readiness (when reachable), and deep-links out to
// the external observability tools so an operator can jump from a red tile to
// the underlying logs/traces.
type AdminHealthResponse struct {
	Server ReadinessResponse  `json:"server"`
	Worker *WorkerHealth      `json:"worker,omitempty"`
	Links  ObservabilityLinks `json:"links"`
}

// WorkerHealth mirrors the worker's /readyz payload, plus whether it was
// reachable at all (the worker is not publicly routable; the server probes it
// over the internal network when BOWRAIN_WORKER_HEALTH_URL is set).
type WorkerHealth struct {
	Reachable  bool                       `json:"reachable"`
	Status     string                     `json:"status,omitempty"`
	Components map[string]workerComponent `json:"components,omitempty"`
	Error      string                     `json:"error,omitempty"`
}

type workerComponent struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// ObservabilityLinks are operator deep-links, configured via env so the same
// build points at the right project per environment. Empty links are hidden.
type ObservabilityLinks struct {
	Sentry     string `json:"sentry,omitempty"`
	PostHog    string `json:"posthog,omitempty"`
	CloudWatch string `json:"cloudwatch,omitempty"`
}

// HandleAdminHealth aggregates server + worker readiness for the ctrl dashboard.
// Always 200 (the dashboard renders the per-component detail); an unhealthy
// server is conveyed in the body's status field, not the HTTP code, so the
// admin UI can always render.
func (s *Server) HandleAdminHealth(c echo.Context) error {
	ctx := c.Request().Context()
	components, status := s.readiness(ctx)

	resp := AdminHealthResponse{
		Server: ReadinessResponse{
			Status:     status,
			Version:    version.Version,
			Components: components,
		},
		Worker: probeWorkerHealth(ctx, os.Getenv("BOWRAIN_WORKER_HEALTH_URL")),
		Links: ObservabilityLinks{
			Sentry:     os.Getenv("OBSERVABILITY_SENTRY_URL"),
			PostHog:    os.Getenv("OBSERVABILITY_POSTHOG_URL"),
			CloudWatch: os.Getenv("OBSERVABILITY_CLOUDWATCH_URL"),
		},
	}
	return c.JSON(http.StatusOK, resp)
}

// probeWorkerHealth GETs the worker's /readyz over the internal network. Returns
// nil when no worker URL is configured (the dashboard then omits the worker
// panel). A probe failure is reported as reachable=false with the error, never
// fatal to the dashboard.
func probeWorkerHealth(ctx context.Context, baseURL string) *WorkerHealth {
	if baseURL == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/readyz", nil)
	if err != nil {
		return &WorkerHealth{Reachable: false, Error: err.Error()}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return &WorkerHealth{Reachable: false, Error: err.Error()}
	}
	defer func() { _ = res.Body.Close() }()

	var body struct {
		Status     string                     `json:"status"`
		Components map[string]workerComponent `json:"components"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return &WorkerHealth{Reachable: true, Error: "invalid worker response"}
	}
	return &WorkerHealth{
		Reachable:  true,
		Status:     body.Status,
		Components: body.Components,
	}
}
