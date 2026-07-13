package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/connector"
)

// connectorFeature maps a connector type to the plan feature it requires. Types
// absent from the map are available on every plan. Only entries whose feature is
// actually sold belong here: gating a type nobody advertises would invent a
// paywall, and leaving out one that is advertised gives it away (which is what
// happened to git).
var connectorFeature = map[string]billing.Feature{
	"git": billing.FeatureConnectorsGit,
}

// ConnectorAddRequest is the request for adding a connector.
type ConnectorAddRequest struct {
	Type   string            `json:"type"`
	Config map[string]string `json:"config"`
}

// FetchRequest is the request for fetching content from a connector.
type FetchRequest struct {
	ConnectorID string   `json:"connector_id"`
	ProjectID   string   `json:"project_id"`
	Paths       []string `json:"paths,omitempty"`
}

// PublishRequest is the request for publishing content to a connector.
type PublishRequest struct {
	ConnectorID string `json:"connector_id"`
	ProjectID   string `json:"project_id"`
	Message     string `json:"message,omitempty"`
}

func (s *Server) HandleListActiveConnectors(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	active := s.Services.Connector.ListActive(wsID)
	type connectorInfo struct {
		ID       string             `json:"id"`
		Name     string             `json:"name"`
		Category connector.Category `json:"category"`
	}
	result := make([]connectorInfo, len(active))
	for i, c := range active {
		result[i] = connectorInfo{
			ID:       c.ID(),
			Name:     c.Name(),
			Category: c.Category(),
		}
	}
	return c.JSON(http.StatusOK, result)
}

func (s *Server) HandleAddConnector(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}

	var req ConnectorAddRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	// Git connectors are a paid feature (AD-018), and the plan matrix has said so
	// since it was written — but nothing enforced it, so a Free workspace could
	// add one. The gate has to live here rather than on the route: every connector
	// type shares this endpoint, and the type only becomes known once the body is
	// bound. It runs before the service check so entitlement never depends on how
	// the server happens to be wired.
	if feature, ok := connectorFeature[req.Type]; ok {
		if err := billing.RequireFeature(c, feature, s.billingGuardEvent()); err != nil {
			return err
		}
	}

	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	conn, err := s.Services.Connector.AddConnector(wsID, req.Type, req.Config)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, map[string]string{
		"id":       conn.ID(),
		"name":     conn.Name(),
		"category": string(conn.Category()),
	})
}

func (s *Server) HandleRemoveConnector(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	if err := s.Services.Connector.RemoveConnector(wsID, c.Param("id")); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) HandleFetch(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req FetchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	items, err := s.Services.Connector.Fetch(c.Request().Context(), wsID, req.ConnectorID, req.ProjectID, connector.FetchOptions{
		Paths: req.Paths,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]int{"items_fetched": len(items)})
}

func (s *Server) HandlePublish(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req PublishRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	err := s.Services.Connector.Publish(c.Request().Context(), wsID, req.ConnectorID, req.ProjectID, connector.PublishOptions{
		Message: req.Message,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) HandleConnectorStatus(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	status, err := s.Services.Connector.ConnectorStatus(c.Request().Context(), wsID, c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, status)
}
