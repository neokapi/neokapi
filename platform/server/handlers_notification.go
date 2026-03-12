package server

import (
	"net/http"
	"strconv"

	bstore "github.com/gokapi/gokapi/bowrain/store"
	"github.com/labstack/echo/v4"
)

// HandleListNotifications returns notifications for the authenticated user.
func (s *Server) HandleListNotifications(c echo.Context) error {
	if s.NotificationStore == nil {
		return c.JSON(http.StatusOK, map[string]any{"notifications": []any{}, "unread_count": 0})
	}

	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
	}

	ctx := c.Request().Context()

	limit := 50
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	unreadOnly := c.QueryParam("unread") == "true"

	notifications, err := s.NotificationStore.List(ctx, userID, limit, unreadOnly)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if notifications == nil {
		notifications = []bstore.Notification{}
	}

	unreadCount, _ := s.NotificationStore.UnreadCount(ctx, userID)

	return c.JSON(http.StatusOK, map[string]any{
		"notifications": notifications,
		"unread_count":  unreadCount,
	})
}

// HandleMarkNotificationRead marks a single notification as read.
func (s *Server) HandleMarkNotificationRead(c echo.Context) error {
	if s.NotificationStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "notifications not configured"})
	}

	userID, _ := c.Get("user_id").(string)
	notificationID := c.Param("nid")

	if err := s.NotificationStore.MarkRead(c.Request().Context(), notificationID, userID); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// HandleMarkAllNotificationsRead marks all notifications as read for the user.
func (s *Server) HandleMarkAllNotificationsRead(c echo.Context) error {
	if s.NotificationStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "notifications not configured"})
	}

	userID, _ := c.Get("user_id").(string)

	if err := s.NotificationStore.MarkAllRead(c.Request().Context(), userID); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// HandleDeleteNotification removes a notification.
func (s *Server) HandleDeleteNotification(c echo.Context) error {
	if s.NotificationStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "notifications not configured"})
	}

	userID, _ := c.Get("user_id").(string)
	notificationID := c.Param("nid")

	if err := s.NotificationStore.Delete(c.Request().Context(), notificationID, userID); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
