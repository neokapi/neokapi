package server

import (
	"errors"
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

// serverErr reports a failure that is ours rather than the caller's.
//
// It *returns* the error instead of writing a response, which is the whole
// point: the central httpErrorHandler is what renders it, logging the raw text
// server-side against the request's reference ID and reporting it to Sentry
// while the client receives only the generic 5xx envelope. A handler that
// writes its own 500 with c.JSON never reaches that sanitising step and ships
// the driver's, the filesystem's or the connector's own words to whoever
// provoked the failure — which is exactly how a database error string, a
// column list or an absolute path leaves the building.
//
// Wrap with %w to add context. It reaches the log and nothing else, so it can
// be as specific as the operator needs:
//
//	return serverErr(c, fmt.Errorf("store block %s: %w", blockID, err))
//
// An open-circuit rejection is neither ours nor a 500, so it keeps rendering as
// the typed 503 with its Retry-After. httpErrorHandler makes the same check, so
// this is belt-and-braces — but it leaves serverErr correct on its own terms
// rather than by arrangement with a caller two layers up.
//
// A nil error becomes a 500 rather than a silent 200: reaching here at all
// means the handler believed it had failed, and answering "success" with an
// empty body would be the worse of the two wrong answers.
//
// TestNo5xxResponseCarriesInternalDetail holds the whole package to this.
func serverErr(c echo.Context, err error) error {
	if err == nil {
		return errors.New("serverErr called with a nil error")
	}
	if resp, ok := unavailableErr(c, err); ok {
		return resp
	}
	return err
}

// serverErrStatus is serverErr for a 5xx the handler means specifically — a 502
// because an upstream connector answered badly, a 503 because a store this
// workspace needs is not configured. A bare error would render as 500 and lose
// that distinction, so the status travels as an echo.HTTPError and the cause
// travels in its Internal field, where httpErrorHandler logs it and strips it
// from the body.
//
// Use serverErr for the ordinary case; reach for this only when the status is a
// deliberate statement about *where* the failure was, not merely that it
// happened.
func serverErrStatus(c echo.Context, status int, err error) error {
	if err == nil {
		return errors.New("serverErrStatus called with a nil error")
	}
	if resp, ok := unavailableErr(c, err); ok {
		return resp
	}
	return echo.NewHTTPError(status).SetInternal(err)
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
