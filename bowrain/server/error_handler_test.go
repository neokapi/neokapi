package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newErrCtx(t *testing.T, method string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, "/api/v1/thing", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("request_id", "req-test-1")
	return c, rec
}

func TestHTTPErrorHandler_ServerErrorIsRedacted(t *testing.T) {
	s := &Server{}
	c, rec := newErrCtx(t, http.MethodGet)

	s.httpErrorHandler(errors.New("pq: password authentication failed for user \"bowrain\""), c)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "internal server error" {
		t.Fatalf("error = %q, want generic message (no leak)", body.Error)
	}
	if body.Details != "" {
		t.Fatalf("details = %q, want empty (internal detail must not leak on 5xx)", body.Details)
	}
	if body.Reference != "req-test-1" {
		t.Fatalf("reference = %q, want req-test-1", body.Reference)
	}
	if body.Message == "" {
		t.Fatalf("message missing: every error envelope carries a human sentence")
	}
	if strings.Contains(body.Message, "pq:") {
		t.Fatalf("message = %q leaks internals on 5xx", body.Message)
	}
	if got := rec.Header().Get("Content-Type"); got == "" {
		t.Fatalf("missing content-type")
	}
}

func TestHTTPErrorHandler_ClientErrorEchoesMessage(t *testing.T) {
	s := &Server{}
	c, rec := newErrCtx(t, http.MethodGet)

	s.httpErrorHandler(echo.NewHTTPError(http.StatusNotFound, "project not found"), c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "project not found" {
		t.Fatalf("error = %q, want echoed client message", body.Error)
	}
	if body.Reference != "req-test-1" {
		t.Fatalf("reference = %q, want req-test-1", body.Reference)
	}
	if body.Message == "" {
		t.Fatalf("message missing on 4xx envelope")
	}
}

func TestHTTPErrorHandler_HeadRequestNoBody(t *testing.T) {
	s := &Server{}
	c, rec := newErrCtx(t, http.MethodHead)

	s.httpErrorHandler(echo.NewHTTPError(http.StatusNotFound, "nope"), c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD response should have no body, got %q", rec.Body.String())
	}
}

func TestHTTPErrorHandler_MissingRequestID(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec) // no request_id set

	s.httpErrorHandler(errors.New("boom"), c)

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Reference != "" {
		t.Fatalf("reference = %q, want empty when no request_id", body.Reference)
	}
}
