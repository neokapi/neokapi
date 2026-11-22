package server

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// flushResponse sends what the handler has written so far to the client, for
// streamed responses (server-sent events).
func flushResponse(c *echo.Context) {
	_ = http.NewResponseController(c.Response()).Flush()
}

// upgradeWriter is the http.ResponseWriter beneath Echo's response. A
// WebSocket upgrade writes its 101 and takes the connection over through it,
// outside Echo's record of the response.
func upgradeWriter(c *echo.Context) http.ResponseWriter {
	if r, err := echo.UnwrapResponse(c.Response()); err == nil {
		return r.ResponseWriter
	}
	return c.Response()
}

// responseStatusOf is the status the handler has set on the response, 200
// when it has set none.
func responseStatusOf(c *echo.Context) int {
	if r, err := echo.UnwrapResponse(c.Response()); err == nil {
		return r.Status
	}
	return http.StatusOK
}
