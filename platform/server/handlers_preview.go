package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
)

// HandleRenderDocumentPreview returns a full HTML preview for a file in a project.
// It uses stored PreviewHTML if available, falling back to generating a preview
// from the stored BlockIndex.
// GET /editor/projects/:pid/file-preview/*?locale=xx
func (s *Server) HandleRenderDocumentPreview(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := c.Param("pid")
	fname := fileParam(c)
	ctx := c.Request().Context()

	_, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	item, err := s.ContentStore.GetItem(ctx, pid, "main", fname)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("item %q not found in project", fname)})
	}

	// 1. Best: reader-generated preview HTML (format-aware).
	if item.PreviewHTML != "" {
		return c.HTML(http.StatusOK, item.PreviewHTML)
	}

	// 2. Fallback: generate default preview from stored BlockIndex.
	if item.BlockIndex != "" {
		preview := editor.BuildPreviewFromBlockIndex(item.BlockIndex)
		return c.HTML(http.StatusOK, preview)
	}

	return c.HTML(http.StatusOK, "")
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

	sb, err := s.ContentStore.GetBlock(ctx, pid, "main", bid)
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
	item, err := s.ContentStore.GetItem(ctx, pid, "main", sb.ItemName)
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
