package server

import (
	"net/http"

	"github.com/labstack/echo/v4"

	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// HandleGetDigestSettings returns the digest settings for the current user.
func (s *Server) HandleGetDigestSettings(c echo.Context) error {
	userID := c.Get("user_id").(string)
	wsID := c.Param("ws")

	if s.DigestStore == nil {
		return c.JSON(http.StatusOK, bstore.DefaultDigestSettings(userID, wsID))
	}

	settings, err := s.DigestStore.GetSettings(c.Request().Context(), userID, wsID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get digest settings")
	}

	return c.JSON(http.StatusOK, settings)
}

// HandleUpdateDigestSettings updates the digest settings for the current user.
func (s *Server) HandleUpdateDigestSettings(c echo.Context) error {
	userID := c.Get("user_id").(string)
	wsID := c.Param("ws")

	if s.DigestStore == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "digest settings not available")
	}

	var req bstore.DigestSettings
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Validate frequency.
	switch req.Frequency {
	case bstore.DigestDaily, bstore.DigestWeekly, bstore.DigestOff:
		// valid
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "frequency must be daily, weekly, or off")
	}

	req.UserID = userID
	req.WorkspaceID = wsID

	if err := s.DigestStore.UpsertSettings(c.Request().Context(), &req); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save digest settings")
	}

	return c.JSON(http.StatusOK, &req)
}
