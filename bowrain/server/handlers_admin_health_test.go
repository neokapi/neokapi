package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestProbeWorkerHealth_NoURLReturnsNil(t *testing.T) {
	if wh := probeWorkerHealth(context.Background(), ""); wh != nil {
		t.Fatalf("expected nil when no worker URL configured, got %+v", wh)
	}
}

func TestProbeWorkerHealth_ParsesReadyz(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Errorf("expected /readyz, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready","components":{"database":{"status":"up"}}}`))
	}))
	defer srv.Close()

	wh := probeWorkerHealth(context.Background(), srv.URL)
	if wh == nil || !wh.Reachable {
		t.Fatalf("expected reachable worker health, got %+v", wh)
	}
	if wh.Status != "ready" {
		t.Fatalf("status = %q, want ready", wh.Status)
	}
	if wh.Components["database"].Status != "up" {
		t.Fatalf("database component = %+v, want up", wh.Components["database"])
	}
}

func TestProbeWorkerHealth_UnreachableIsReported(t *testing.T) {
	// Port 1 is not listening → connection refused.
	wh := probeWorkerHealth(context.Background(), "http://127.0.0.1:1")
	if wh == nil || wh.Reachable {
		t.Fatalf("expected unreachable worker health, got %+v", wh)
	}
	if wh.Error == "" {
		t.Fatal("expected an error message on unreachable worker")
	}
}

func TestHandleAdminHealth_ReturnsServerReadiness(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := s.HandleAdminHealth(c); err != nil {
		t.Fatalf("HandleAdminHealth: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body AdminHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Server.Components == nil {
		t.Fatal("expected server components in the response")
	}
	if body.Worker != nil {
		t.Fatalf("worker should be nil without BOWRAIN_WORKER_HEALTH_URL, got %+v", body.Worker)
	}
}
