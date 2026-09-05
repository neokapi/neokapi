package server

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/core/registry"
)

// HandleGetToolSchema serves a tool's option schema from the tool registry:
// the document a flow step's options form renders. The registry's own JSON
// is returned when it holds one, so every extension field (ui:groups,
// ui:widget, x-step, $defs) reaches the client as the tool declared it.
//
// A tool the registry does not know, or one registered without a schema, is
// a 404. The route is public, like /info, whose tool list marks the tools
// that carry a schema (hasSchema).
func (s *Server) HandleGetToolSchema(c echo.Context) error {
	sch := s.ToolRegistry.Schema(registry.ToolID(c.Param("name")))
	if sch == nil {
		return apiErr(c, http.StatusNotFound, "tool schema not found")
	}
	data := sch.RawJSON
	if len(data) == 0 {
		var err error
		if data, err = json.Marshal(sch); err != nil {
			return serverErr(c, err)
		}
	}
	return c.JSONBlob(http.StatusOK, data)
}
