package server

import (
	"net/http"
	"sort"

	"github.com/gokapi/gokapi/version"
	"github.com/labstack/echo/v4"
)

// HealthResponse is the response for the health check endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// ConfigResponse reports the server's operating mode.
type ConfigResponse struct {
	Mode    string `json:"mode"` // "local" or "server"
	Version string `json:"version"`
}

// FormatInfo describes a registered format.
type FormatInfo struct {
	Name      string `json:"name"`
	HasReader bool   `json:"has_reader"`
	HasWriter bool   `json:"has_writer"`
}

// ToolInfo describes a registered tool.
type ToolInfo struct {
	Name string `json:"name"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// HandleHealth returns a simple health check.
func (s *Server) HandleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, HealthResponse{
		Status:  "ok",
		Version: version.Version,
	})
}

// HandleConfig returns the server configuration mode.
func (s *Server) HandleConfig(c echo.Context) error {
	mode := "server"
	if s.Config.LocalMode {
		mode = "local"
	}
	return c.JSON(http.StatusOK, ConfigResponse{
		Mode:    mode,
		Version: version.Version,
	})
}

// HandleListFormats lists all registered formats with reader/writer availability.
func (s *Server) HandleListFormats(c echo.Context) error {
	// Collect unique format names from both readers and writers.
	nameSet := make(map[string]struct{})
	for _, name := range s.FormatRegistry.ReaderNames() {
		nameSet[name] = struct{}{}
	}
	for _, name := range s.FormatRegistry.WriterNames() {
		nameSet[name] = struct{}{}
	}

	var formatList []FormatInfo
	for name := range nameSet {
		formatList = append(formatList, FormatInfo{
			Name:      name,
			HasReader: s.FormatRegistry.HasReader(name),
			HasWriter: s.FormatRegistry.HasWriter(name),
		})
	}

	// Sort for deterministic output.
	sort.Slice(formatList, func(i, j int) bool {
		return formatList[i].Name < formatList[j].Name
	})

	return c.JSON(http.StatusOK, formatList)
}

// HandleListTools lists all registered tools.
func (s *Server) HandleListTools(c echo.Context) error {
	names := s.ToolRegistry.Names()
	sort.Strings(names)

	tools := make([]ToolInfo, len(names))
	for i, name := range names {
		tools[i] = ToolInfo{Name: name}
	}

	return c.JSON(http.StatusOK, tools)
}
