// Package server — handlers_admin_slugs.go
//
// Admin endpoints for managing the workspace slug-rename grace period:
// listing active reservations and releasing one ahead of schedule. Used by
// the ctrl SaaS admin UI when an operator needs to free a name for reuse.
package server

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// HandleAdminListSlugReservations returns all active workspace slug
// reservations (most recent first), used by the ctrl admin UI.
//
// GET /api/admin/slug-reservations
func (s *Server) HandleAdminListSlugReservations(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}
	res, err := s.AuthStore.ListSlugReservations(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, res)
}

// adminReleaseSlugRequest is the body for POST /api/admin/slug-reservations/release.
type adminReleaseSlugRequest struct {
	Slug string `json:"slug"`
}

// HandleAdminReleaseSlugReservation deletes a single reservation, freeing
// the slug for immediate reuse. Idempotent on already-cleared reservations:
// returns 404 only if the slug was never reserved.
//
// POST /api/admin/slug-reservations/release
func (s *Server) HandleAdminReleaseSlugReservation(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}
	var req adminReleaseSlugRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	slug := strings.TrimSpace(strings.ToLower(req.Slug))
	if slug == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "slug is required"})
	}
	if err := s.AuthStore.ReleaseSlugReservation(c.Request().Context(), slug); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "released", "slug": slug})
}
