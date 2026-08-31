package observe

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("metrics"))
	})
}

func TestMetricsGate_TokenMode(t *testing.T) {
	h := MetricsAccessMiddlewareStd("s3cr3t", okHandler())

	// Correct bearer → allowed.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t")
	req.RemoteAddr = "203.0.113.9:5000" // public IP must not matter in token mode
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: code = %d, want 200", rec.Code)
	}

	// Wrong bearer → 404 (anti-enumeration).
	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer nope")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad token: code = %d, want 404", rec.Code)
	}
}

func TestMetricsGate_IPModeAllowsPrivateRefusesPublic(t *testing.T) {
	h := MetricsAccessMiddlewareStd("", okHandler())

	// Loopback scraper → allowed.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:4000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback: code = %d, want 200", rec.Code)
	}

	// Private (in-cluster) scraper → allowed.
	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "10.0.4.7:4000"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("private ip: code = %d, want 200", rec.Code)
	}

	// Public IP → refused with 404.
	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "203.0.113.9:4000"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public ip: code = %d, want 404", rec.Code)
	}
}
