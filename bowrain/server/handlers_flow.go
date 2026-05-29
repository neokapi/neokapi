package server

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/id"
)

// Flow definitions (Bowrain AD-013).
//
// Flows are server-side, project-scoped pipeline graphs (reader → tool(s) →
// writer). Automation `run_flow` actions reference a flow by id. Built-in
// flows (flow.BuiltInFlows) are always available and merged into the listing;
// project flows are persisted in the FlowDefStore and override or extend the
// built-in set.
//
// Flows are connector-agnostic: they apply to content from any connector
// (Kapi is one source among many). The flow graph never names a connector.

// HandleListFlowDefinitions returns built-in flows merged with the project's
// stored flow definitions.
func (s *Server) HandleListFlowDefinitions(c echo.Context) error {
	projectID := c.Param("id")

	defs := make([]flow.FlowDefinition, 0, 8)
	defs = append(defs, flow.BuiltInFlows()...)

	if s.FlowDefStore != nil {
		stored, err := s.FlowDefStore.List(c.Request().Context(), projectID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
		defs = append(defs, stored...)
	}

	return c.JSON(http.StatusOK, defs)
}

// HandleGetFlowDefinition returns a single flow definition (built-in or
// project-stored) by id.
func (s *Server) HandleGetFlowDefinition(c echo.Context) error {
	projectID := c.Param("id")
	flowID := c.Param("flowId")

	for _, def := range flow.BuiltInFlows() {
		if def.ID == flowID {
			return c.JSON(http.StatusOK, def)
		}
	}

	if s.FlowDefStore == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "flow not found"})
	}
	def, err := s.FlowDefStore.Get(c.Request().Context(), projectID, flowID)
	if errors.Is(err, bstore.ErrFlowDefNotFound) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "flow not found"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, def)
}

// HandleCreateFlowDefinition creates a new project flow definition.
func (s *Server) HandleCreateFlowDefinition(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAutomation); err != nil {
		return err
	}
	projectID := c.Param("id")
	if s.FlowDefStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "flow store not configured"})
	}

	var def flow.FlowDefinition
	if err := c.Bind(&def); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if def.ID == "" {
		def.ID = id.New()
	}
	if isBuiltInFlowID(def.ID) {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: "id collides with a built-in flow; choose another id"})
	}
	def.Source = "project"
	if err := def.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if err := s.FlowDefStore.Upsert(c.Request().Context(), projectID, &def); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	stored, err := s.FlowDefStore.Get(c.Request().Context(), projectID, def.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, stored)
}

// HandleUpdateFlowDefinition replaces an existing project flow definition.
func (s *Server) HandleUpdateFlowDefinition(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAutomation); err != nil {
		return err
	}
	projectID := c.Param("id")
	flowID := c.Param("flowId")
	if s.FlowDefStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "flow store not configured"})
	}
	if isBuiltInFlowID(flowID) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: "cannot modify a built-in flow"})
	}

	var def flow.FlowDefinition
	if err := c.Bind(&def); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	def.ID = flowID // path wins over body
	def.Source = "project"
	if err := def.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if err := s.FlowDefStore.Upsert(c.Request().Context(), projectID, &def); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	stored, err := s.FlowDefStore.Get(c.Request().Context(), projectID, flowID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, stored)
}

// HandleDeleteFlowDefinition removes a project flow definition.
func (s *Server) HandleDeleteFlowDefinition(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAutomation); err != nil {
		return err
	}
	projectID := c.Param("id")
	flowID := c.Param("flowId")
	if s.FlowDefStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "flow store not configured"})
	}
	if isBuiltInFlowID(flowID) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: "cannot delete a built-in flow"})
	}
	if err := s.FlowDefStore.Delete(c.Request().Context(), projectID, flowID); err != nil {
		if errors.Is(err, bstore.ErrFlowDefNotFound) {
			return c.JSON(http.StatusNotFound, ErrorResponse{Error: "flow not found"})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

func isBuiltInFlowID(flowID string) bool {
	for _, def := range flow.BuiltInFlows() {
		if def.ID == flowID {
			return true
		}
	}
	return false
}
