package server

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/apierror"
	"github.com/neokapi/neokapi/bowrain/resilience"
)

// apiErr writes the standard API error envelope (see bowrain/apierror):
// the stable code in "error" (unchanged wire contract), a human sentence in
// "message", the request correlation ID in "reference", and any structured
// detail fields merged at the top level.
//
// Handlers adopt this in place of ad-hoc c.JSON(status, ErrorResponse{…}) /
// map writes; the central httpErrorHandler remains the backstop for errors
// returned up the Echo chain (echo.NewHTTPError, panics).
func apiErr(c echo.Context, status int, code string, details ...map[string]any) error {
	return apierror.Write(c, status, code, details...)
}

// unavailableErr renders an open-circuit rejection as a typed 503 and reports
// whether it applied. It returns (nil, false) for every other error, so a
// handler's own error handling stays exactly as it was:
//
//	if err != nil {
//	    if resp, ok := unavailableErr(c, err); ok {
//	        return resp
//	    }
//	    …existing handling…
//	}
//
// This is deliberately not a 500. Nothing broke: a dependency is known to be
// down, we declined to call it, and the work is recoverable by waiting. The
// client gets a distinct code, a Retry-After, and enough detail to say so
// plainly instead of showing a failure the user cannot act on.
func unavailableErr(c echo.Context, err error) (error, bool) {
	u, ok := resilience.AsUnavailable(err)
	if !ok {
		return nil, false
	}
	retry := u.RetryAfterSeconds()
	c.Response().Header().Set("Retry-After", strconv.Itoa(retry))

	code := apierror.CodeDependencyUnavailable
	if u.Kind == resilience.KindAI {
		code = apierror.CodeAIUnavailable
	}
	return apiErr(c, http.StatusServiceUnavailable, code, map[string]any{
		"dependency":          u.Dependency,
		"retry_after_seconds": retry,
	}), true
}
