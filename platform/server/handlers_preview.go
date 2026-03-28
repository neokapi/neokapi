package server

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/platform/store"
)

// HandleRenderDocumentPreview returns a full HTML preview for a file in a project.
// It uses stored PreviewHTML if available, falling back to generating a preview
// from the stored BlockIndex, and finally to a block-list preview built from the
// stored blocks themselves.
// GET /editor/projects/:pid/file-preview/*?locale=xx
func (s *Server) HandleRenderDocumentPreview(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	fname := fileParam(c)
	ctx := c.Request().Context()

	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	stream := streamParamWithProject(c, proj)

	item, err := s.ContentStore.GetItem(ctx, pid, stream, fname)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("item %q not found in project", fname)})
	}

	// 1. Best: reader-generated preview HTML (format-aware).
	if item.PreviewHTML != "" {
		return c.HTML(http.StatusOK, item.PreviewHTML)
	}

	// 2. Fallback: generate default preview from stored BlockIndex.
	// Skip the empty default "{}" that StoreItem sets — it produces
	// valid HTML boilerplate with no blocks, hiding the block-list fallback.
	if item.BlockIndex != "" && item.BlockIndex != "{}" {
		preview := editor.BuildPreviewFromBlockIndex(item.BlockIndex)
		if strings.Contains(preview, "<kat-block") {
			return c.HTML(http.StatusOK, preview)
		}
	}

	// 3. Last resort: build a block-list preview from stored blocks.
	storedBlocks, err := s.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: pid,
		Stream:    stream,
		ItemName:  fname,
	})
	if err == nil && len(storedBlocks) > 0 {
		return c.HTML(http.StatusOK, buildBlockListPreview(storedBlocks))
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

	pid := projectParam(c)
	bid := c.Param("bid")
	targetLocale := c.QueryParam("locale")
	stream := streamParam(c)
	ctx := c.Request().Context()

	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, bid)
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
	item, err := s.ContentStore.GetItem(ctx, pid, stream, sb.ItemName)
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

// buildBlockListPreview generates a simple block-list preview from stored blocks.
// Used as a last resort when neither PreviewHTML nor BlockIndex is available.
func buildBlockListPreview(blocks []*store.StoredBlock) string {
	var body strings.Builder
	body.WriteString(`<div style="font-family: monospace; font-size: 13px;">`)
	for _, sb := range blocks {
		if !sb.Block.Translatable {
			continue
		}
		text := html.EscapeString(sb.Block.SourceText())
		fmt.Fprintf(&body,
			`<p style="margin: 4px 0; padding: 4px 8px;"><kat-block id="%s">%s</kat-block></p>`+"\n",
			sb.Block.ID, text)
	}
	body.WriteString(`</div>`)
	return editor.PreviewBoilerplateStart() + body.String() + editor.PreviewBoilerplateEnd()
}
