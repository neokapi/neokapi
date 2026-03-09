package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gokapi/gokapi/core/editor"
	"github.com/gokapi/gokapi/core/model"
	"github.com/labstack/echo/v4"
)

// HandleRenderDocumentPreview returns a full HTML preview for a file in a project.
// GET /editor/projects/:pid/file-preview/*?locale=xx
func (s *Server) HandleRenderDocumentPreview(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := c.Param("pid")
	fname := fileParam(c)
	ctx := c.Request().Context()

	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	item, err := s.ContentStore.GetItem(ctx, pid, fname)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("item %q not found in project", fname)})
	}

	if len(item.SourceBytes) == 0 {
		return c.HTML(http.StatusOK, "")
	}

	reader, err := s.FormatRegistry.NewReader(item.Format)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("no reader for %q: %s", item.Format, err)})
	}

	doc := &model.RawDocument{
		URI:          fname,
		SourceLocale: proj.SourceLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(item.SourceBytes)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("parse source: %s", err)})
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("read source: %s", result.Error)})
		}
		parts = append(parts, result.Part)
	}
	reader.Close()

	preview := editor.BuildPreview(parts, item.SourceBytes, item.Format, proj.SourceLocale)
	return c.HTML(http.StatusOK, preview)
}

// HandleRenderBlockHTML returns the rendered HTML for a single block.
// If a locale query param is provided and a translation exists, the target text
// is returned; otherwise the source HTML (or plain source text) is returned.
// GET /editor/projects/:pid/blocks/:bid/html?locale=xx
func (s *Server) HandleRenderBlockHTML(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := c.Param("pid")
	bid := c.Param("bid")
	targetLocale := c.QueryParam("locale")
	ctx := c.Request().Context()

	sb, err := s.ContentStore.GetBlock(ctx, pid, bid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("block %q not found", bid)})
	}

	// Return target translation if available.
	if targetLocale != "" {
		if text := sb.Block.TargetText(model.LocaleID(targetLocale)); text != "" {
			return c.HTML(http.StatusOK, text)
		}
	}

	// Try the block index for HTML-enriched source.
	item, err := s.ContentStore.GetItem(ctx, pid, sb.ItemName)
	if err == nil && item.BlockIndex != "" {
		var blockIndex editor.BlockIndex
		if err := json.Unmarshal([]byte(item.BlockIndex), &blockIndex); err == nil {
			b := blockIndex.BlockByID(bid)
			if b != nil && b.SourceHTML != "" {
				return c.HTML(http.StatusOK, b.SourceHTML)
			}
		}
	}

	return c.HTML(http.StatusOK, sb.Block.SourceText())
}
