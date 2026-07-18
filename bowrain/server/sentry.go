package server

import "github.com/labstack/echo/v4"

// captureServerError reports a 5xx to the error-tracking backend (Sentry).
//
// This is a no-op stub in Phase 1; Phase 2 wires the actual Sentry capture,
// tagging the event with the request ID so the client-visible "reference"
// resolves to the Sentry issue. Kept as a seam so the error handler needs no
// change when Sentry lands.
func (s *Server) captureServerError(c echo.Context, err error, status int) {
	_ = c
	_ = err
	_ = status
}
