package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// HandleGetNotificationPreferences returns notification preferences for the current user.
func (s *Server) HandleGetNotificationPreferences(c echo.Context) error {
	if s.PreferenceStore == nil {
		return c.JSON(http.StatusOK, map[string]any{"preferences": bstore.DefaultPreferences("", "")})
	}

	ws := c.Param("ws")
	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
	}

	prefs, err := s.PreferenceStore.List(c.Request().Context(), userID, ws)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"preferences": prefs})
}

// HandleUpdateNotificationPreferences updates notification preferences for the current user.
func (s *Server) HandleUpdateNotificationPreferences(c echo.Context) error {
	if s.PreferenceStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "preferences not configured"})
	}

	ws := c.Param("ws")
	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
	}

	var req struct {
		Preferences []struct {
			Category string `json:"category"`
			Channels struct {
				Web     bool `json:"web"`
				Email   bool `json:"email"`
				Push    bool `json:"push"`
				Desktop bool `json:"desktop"`
			} `json:"channels"`
		} `json:"preferences"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	prefs := make([]bstore.NotificationPreference, 0, len(req.Preferences))
	for _, p := range req.Preferences {
		prefs = append(prefs, bstore.NotificationPreference{
			UserID:      userID,
			WorkspaceID: ws,
			Category:    bstore.NotificationCategory(p.Category),
			Web:         p.Channels.Web,
			Email:       p.Channels.Email,
			Push:        p.Channels.Push,
			Desktop:     p.Channels.Desktop,
		})
	}

	if err := s.PreferenceStore.BulkUpsert(c.Request().Context(), prefs); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}
