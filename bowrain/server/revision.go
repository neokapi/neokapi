package server

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
)

// A write to a record people edit names the revision it was based on, and the
// store refuses it when the record has changed since. These are the answers
// such a write can get besides success; every record with a revision answers
// the same way.

// ChangedResponse is a write refused because the record changed after the
// writer read it. Current is the record as it now stands, so the writer can
// show the person both versions.
type ChangedResponse struct {
	Code    string `json:"code"`
	Error   string `json:"error"`
	Current any    `json:"current"`
}

// refuseUnversioned answers a write that names no base revision: a client
// built before the record had one, which can only overwrite.
func refuseUnversioned(c *echo.Context, noun string) error {
	return c.JSON(http.StatusPreconditionRequired, ErrorResponse{
		Error: fmt.Sprintf("this version of the app cannot save %s; update it, then save again", noun),
	})
}

// refuseChanged answers a write based on a record that changed since it was
// read, with the record as it now stands. code is "<kind>_changed".
func refuseChanged(c *echo.Context, code, noun string, current any) error {
	return c.JSON(http.StatusConflict, ChangedResponse{
		Code:    code,
		Error:   fmt.Sprintf("someone else saved this %s after you opened it; the current version is attached", noun),
		Current: current,
	})
}

// refuseGone answers a write to a record deleted since it was read.
func refuseGone(c *echo.Context, noun string) error {
	return c.JSON(http.StatusNotFound, ErrorResponse{
		Error: fmt.Sprintf("this %s was deleted after it was opened", noun),
	})
}
