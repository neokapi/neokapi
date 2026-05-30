package server

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	bevent "github.com/neokapi/neokapi/bowrain/event"
)

// HandleListAuditLog returns audit log entries for a project.
// GET /projects/:id/audit-log?type=&limit=&offset=
func (s *Server) HandleListAuditLog(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermAuditRead); err != nil {
		return err
	}
	if s.AuditLogger == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "audit log not configured"})
	}

	projectID := c.Param("id")
	eventType := c.QueryParam("type")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	if limit <= 0 {
		limit = 50
	}

	entries, err := s.AuditLogger.ListAuditLog(c.Request().Context(), projectID, eventType, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	if entries == nil {
		entries = []bevent.AuditEntry{}
	}

	return c.JSON(http.StatusOK, entries)
}

// HandleListWorkspaceAuditLog returns audit log entries across all projects in a workspace.
// GET /audit-log?type=&actor=&search=&project=&limit=&offset=
func (s *Server) HandleListWorkspaceAuditLog(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermAuditRead); err != nil {
		return err
	}
	if s.AuditLogger == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "audit log not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	if limit <= 0 {
		limit = 50
	}

	q := bevent.AuditQuery{
		WorkspaceID:  wsID,
		ProjectID:    c.QueryParam("project"),
		EventType:    c.QueryParam("type"),
		Actor:        c.QueryParam("actor"),
		ResourceType: c.QueryParam("resource_type"),
		Effect:       c.QueryParam("effect"),
		Search:       c.QueryParam("search"),
		Limit:        limit,
		Offset:       offset,
	}

	entries, err := s.AuditLogger.QueryAuditLog(c.Request().Context(), q)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	if entries == nil {
		entries = []bevent.AuditEntry{}
	}

	return c.JSON(http.StatusOK, entries)
}

// HandleVerifyWorkspaceAuditChain verifies the tamper-evidence of the
// workspace's security audit chain and returns the result. Requires PermAuditRead.
// GET /:ws/audit-log/verify
func (s *Server) HandleVerifyWorkspaceAuditChain(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermAuditRead); err != nil {
		return err
	}
	if s.AuditLogger == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "audit log not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	v, err := s.AuditLogger.VerifyChain(c.Request().Context(), wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, v)
}
