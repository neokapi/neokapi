package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HandleDeviceAuthStart starts the device authorization flow.
func (s *Server) HandleDeviceAuthStart(c echo.Context) error {
	// TODO: implement device auth start
	return c.JSON(http.StatusNotImplemented, ErrorResponse{Error: "not implemented"})
}

// HandleDeviceAuthPoll polls for a device authorization token.
func (s *Server) HandleDeviceAuthPoll(c echo.Context) error {
	// TODO: implement device auth poll
	return c.JSON(http.StatusNotImplemented, ErrorResponse{Error: "not implemented"})
}

// HandleAuthCallback handles OIDC redirect callback.
func (s *Server) HandleAuthCallback(c echo.Context) error {
	// TODO: implement OIDC callback
	return c.JSON(http.StatusNotImplemented, ErrorResponse{Error: "not implemented"})
}

// HandleAuthMe returns the current authenticated user.
func (s *Server) HandleAuthMe(c echo.Context) error {
	// TODO: implement me endpoint
	return c.JSON(http.StatusNotImplemented, ErrorResponse{Error: "not implemented"})
}

// HandleAuthLogout revokes the current token.
func (s *Server) HandleAuthLogout(c echo.Context) error {
	// TODO: implement logout
	return c.JSON(http.StatusNotImplemented, ErrorResponse{Error: "not implemented"})
}
