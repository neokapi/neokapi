package server

import (
	"net/http"

	"github.com/gokapi/gokapi/core/connector"
	"github.com/labstack/echo/v4"
)

// ConnectorAddRequest is the request for adding a connector.
type ConnectorAddRequest struct {
	Type   string            `json:"type"`
	Config map[string]string `json:"config"`
}

// PullRequest is the request for pulling content.
type PullRequest struct {
	ConnectorID string   `json:"connector_id"`
	ProjectID   string   `json:"project_id"`
	Paths       []string `json:"paths,omitempty"`
}

// PushRequest is the request for pushing content.
type PushRequest struct {
	ConnectorID string `json:"connector_id"`
	ProjectID   string `json:"project_id"`
	Message     string `json:"message,omitempty"`
}

func (s *Server) HandleListConnectorTypes(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	types := s.Services.Connector.ListConnectorTypes()
	return c.JSON(http.StatusOK, types)
}

func (s *Server) HandleListActiveConnectors(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	active := s.Services.Connector.ListActive()
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
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req ConnectorAddRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	conn, err := s.Services.Connector.AddConnector(req.Type, req.Config)
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
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	if err := s.Services.Connector.RemoveConnector(c.Param("id")); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) HandlePull(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req PullRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	items, err := s.Services.Connector.Pull(c.Request().Context(), req.ConnectorID, req.ProjectID, connector.PullOptions{
		Paths: req.Paths,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]int{"items_pulled": len(items)})
}

func (s *Server) HandlePush(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req PushRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	err := s.Services.Connector.Push(c.Request().Context(), req.ConnectorID, req.ProjectID, connector.PushOptions{
		Message: req.Message,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) HandleSyncStatus(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	status, err := s.Services.Connector.SyncStatus(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, status)
}
