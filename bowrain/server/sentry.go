package server

import (
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/observe"
)

// captureServerError reports a 5xx to Sentry, tagged with the request ID so the
// client-visible "reference" resolves to the Sentry issue. No-op when Sentry is
// not configured (empty DSN). Called from the unified HTTPErrorHandler, which
// fires both for returned 5xx and for panics recovered by middleware.Recover.
func (s *Server) captureServerError(c echo.Context, err error, status int) {
	if !observe.SentryEnabled() {
		return
	}
	observe.CaptureError(err, requestID(c), map[string]string{
		"route":  c.Path(),
		"method": c.Request().Method,
		"status": strconv.Itoa(status),
	})
}
