package server

import (
	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/apierror"
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
