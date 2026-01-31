package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/version"
	"github.com/labstack/echo/v4"
)

// HealthResponse is the response for the health check endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
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

// FlowInfo describes an available flow.
type FlowInfo struct {
	Name string `json:"name"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// ConvertResponse is the response for a successful conversion.
type ConvertResponse struct {
	Success      bool   `json:"success"`
	InputFormat  string `json:"input_format"`
	OutputFormat string `json:"output_format"`
	Size         int    `json:"size"`
}

// TranslateResponse is the response for a translation request.
type TranslateResponse struct {
	Success    bool   `json:"success"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
	Provider   string `json:"provider"`
}

// FlowExecuteResponse is the response for a flow execution.
type FlowExecuteResponse struct {
	Success  bool   `json:"success"`
	FlowName string `json:"flow_name"`
}

// handleHealth returns a simple health check.
func (s *Server) handleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, HealthResponse{
		Status:  "ok",
		Version: version.Version,
	})
}

// handleListFormats lists all registered formats with reader/writer availability.
func (s *Server) handleListFormats(c echo.Context) error {
	// Collect unique format names from both readers and writers.
	nameSet := make(map[string]struct{})
	for _, name := range s.formatRegistry.ReaderNames() {
		nameSet[name] = struct{}{}
	}
	for _, name := range s.formatRegistry.WriterNames() {
		nameSet[name] = struct{}{}
	}

	var formatList []FormatInfo
	for name := range nameSet {
		formatList = append(formatList, FormatInfo{
			Name:      name,
			HasReader: s.formatRegistry.HasReader(name),
			HasWriter: s.formatRegistry.HasWriter(name),
		})
	}

	// Sort for deterministic output.
	sort.Slice(formatList, func(i, j int) bool {
		return formatList[i].Name < formatList[j].Name
	})

	return c.JSON(http.StatusOK, formatList)
}

// handleListTools lists all registered tools.
func (s *Server) handleListTools(c echo.Context) error {
	names := s.toolRegistry.Names()
	sort.Strings(names)

	tools := make([]ToolInfo, len(names))
	for i, name := range names {
		tools[i] = ToolInfo{Name: name}
	}

	return c.JSON(http.StatusOK, tools)
}

// handleListFlows lists available flows.
func (s *Server) handleListFlows(c echo.Context) error {
	// No predefined flows in the base server; return empty list.
	return c.JSON(http.StatusOK, []FlowInfo{})
}

// handleConvert converts a file between formats.
func (s *Server) handleConvert(c echo.Context) error {
	inputFormat := c.FormValue("input_format")
	outputFormat := c.FormValue("output_format")
	sourceLang := c.FormValue("source_lang")
	targetLang := c.FormValue("target_lang")

	if inputFormat == "" || outputFormat == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "input_format and output_format are required",
		})
	}

	if !s.formatRegistry.HasReader(inputFormat) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "unsupported input format",
			Details: fmt.Sprintf("format %q has no registered reader", inputFormat),
		})
	}

	if !s.formatRegistry.HasWriter(outputFormat) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "unsupported output format",
			Details: fmt.Sprintf("format %q has no registered writer", outputFormat),
		})
	}

	// Get uploaded file.
	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "file upload required",
			Details: err.Error(),
		})
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "failed to open uploaded file",
			Details: err.Error(),
		})
	}
	defer src.Close()

	sourceLocale := model.LocaleID(sourceLang)
	if sourceLocale == "" {
		sourceLocale = model.LocaleEnglish
	}
	targetLocale := model.LocaleID(targetLang)

	ctx := context.Background()

	// Create reader and open document.
	reader, err := s.formatRegistry.NewReader(inputFormat)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "failed to create reader",
			Details: err.Error(),
		})
	}

	rawDoc := &model.RawDocument{
		URI:          file.Filename,
		SourceLocale: sourceLocale,
		TargetLocale: targetLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(src),
	}

	if err := reader.Open(ctx, rawDoc); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "failed to open document",
			Details: err.Error(),
		})
	}
	defer reader.Close()

	// Collect parts from reader.
	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "error reading document",
				Details: result.Error.Error(),
			})
		}
		parts = append(parts, result.Part)
	}

	// Write output.
	var output bytes.Buffer
	writer, err := s.formatRegistry.NewWriter(outputFormat)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "failed to create writer",
			Details: err.Error(),
		})
	}

	if err := writer.SetOutputWriter(&output); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "failed to set output writer",
			Details: err.Error(),
		})
	}

	locale := targetLocale
	if locale == "" {
		locale = sourceLocale
	}
	writer.SetLocale(locale)
	writer.SetEncoding("UTF-8")

	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "failed to write output",
			Details: err.Error(),
		})
	}
	writer.Close()

	return c.Blob(http.StatusOK, "application/octet-stream", output.Bytes())
}

// handleTranslate translates a file using an AI provider.
func (s *Server) handleTranslate(c echo.Context) error {
	providerName := c.FormValue("provider")
	sourceLang := c.FormValue("source_lang")
	targetLang := c.FormValue("target_lang")

	if sourceLang == "" || targetLang == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "source_lang and target_lang are required",
		})
	}

	if providerName == "" {
		providerName = "mock"
	}

	// File upload check.
	_, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "file upload required",
			Details: err.Error(),
		})
	}

	// In a full implementation, this would:
	// 1. Detect or use specified format
	// 2. Read the document
	// 3. Create an AI provider with the given api_key and model
	// 4. Run AITranslateTool over the parts
	// 5. Write the translated document
	//
	// For now, return a structured response indicating the request was accepted.
	return c.JSON(http.StatusOK, TranslateResponse{
		Success:    true,
		SourceLang: sourceLang,
		TargetLang: targetLang,
		Provider:   providerName,
	})
}

// handleFlowExecute executes a named flow.
func (s *Server) handleFlowExecute(c echo.Context) error {
	flowName := c.FormValue("flow_name")

	if flowName == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "flow_name is required",
		})
	}

	// File upload check.
	_, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "file upload required",
			Details: err.Error(),
		})
	}

	// In a full implementation, this would look up the named flow,
	// configure it with the provided parameters, and execute it.
	return c.JSON(http.StatusOK, FlowExecuteResponse{
		Success:  true,
		FlowName: flowName,
	})
}
