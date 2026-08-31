package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/apierror"
)

// requestID returns the per-request correlation ID set by
// observe.RequestIDMiddleware, or "" if absent.
func requestID(c echo.Context) string {
	return apierror.RequestID(c)
}

// httpErrorHandler is a single, consistent error handler for the whole API.
//
// It replaces Echo's default handler so that:
//   - Every error response uses the ErrorResponse envelope, so no handler emits
//     a bare {"error":…} or echo.NewHTTPError's {"message":…} shape.
//   - Every response carries a "reference" (the request ID) — the one ID a user
//     can quote to drill down to logs and the Sentry issue.
//   - 5xx responses do NOT leak internal error strings to the client; the raw
//     error is logged server-side (with the reference) instead.
//
// The Sentry capture hook is wired in Phase 2 (see captureServerError).
func (s *Server) httpErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	// Backstop for an open-circuit rejection that reached the chain unhandled.
	// It must not become a generic 500: nothing broke, a known-down dependency
	// was simply not called, and the client needs the typed code and Retry-After
	// to render that honestly.
	if resp, ok := unavailableErr(c, err); ok {
		if resp != nil {
			slog.WarnContext(c.Request().Context(), "request declined: dependency circuit open",
				"method", c.Request().Method, "path", c.Path(), "reference", requestID(c), "error", err)
		}
		return
	}

	ref := requestID(c)
	status := http.StatusInternalServerError
	code := ""
	message := "internal server error"
	details := ""

	if he, ok := errors.AsType[*echo.HTTPError](err); ok {
		status = he.Code
		if he.Message != nil {
			message = fmt.Sprintf("%v", he.Message)
		}
		if he.Internal != nil {
			details = he.Internal.Error()
		}
	} else {
		// A bare error: treat as 500 and keep the raw string server-side only.
		details = err.Error()
	}

	// Client errors (4xx) are the caller's fault and safe to echo back verbatim.
	// Server errors (5xx) are ours: log them with full context and return a
	// generic message so we never leak internals to the client.
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(c.Request().Context(), "request failed",
			"status", status,
			"method", c.Request().Method,
			"path", c.Path(),
			"reference", ref,
			"error", err,
		)
		s.captureServerError(c, err, status)
		message = "internal server error"
		details = "" // never leak internal detail on 5xx
	}

	// The additive human sentence: the shared copy table when the error value
	// is a known code, otherwise the (already client-safe) error text itself.
	// 5xx responses get a fixed sentence pointing at the reference instead of
	// internals.
	human := apierror.MessageFor(message, nil)
	if status >= http.StatusInternalServerError {
		human = "The server encountered an unexpected error while handling this request. Quote the reference when reporting it."
	}

	resp := ErrorResponse{
		Error:     message,
		Message:   human,
		Details:   details,
		Code:      code,
		Reference: ref,
	}

	var writeErr error
	if c.Request().Method == http.MethodHead {
		writeErr = c.NoContent(status)
	} else {
		writeErr = c.JSON(status, resp)
	}
	if writeErr != nil {
		slog.ErrorContext(c.Request().Context(), "failed to write error response",
			"reference", ref, "error", writeErr)
	}
}
