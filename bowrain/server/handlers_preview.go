package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
)

// HandleRenderDocumentPreview returns a full HTML preview for a file in a project.
// It uses stored PreviewHTML if available, falling back to generating a preview
// from the stored BlockIndex, and finally to a block-list preview built from the
// stored blocks themselves.
// GET /editor/projects/:pid/file-preview/*?locale=xx
//
// The response is document-derived HTML and is served sandboxed. See sandboxCSP:
// this route sits on the authenticated /:ws group, so it shares an origin with
// the application, and the markup it returns is reproduced verbatim from an
// uploaded document rather than escaped — which is the feature, not an
// oversight.
func (s *Server) HandleRenderDocumentPreview(c echo.Context) error {
	// Set before any branch: every response from this route is sandboxed, so
	// the guarantee cannot be lost by an early return added later.
	applySandboxCSP(c)

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
//
// Served sandboxed for the same reason as the document preview, and with one
// more: the target text is whatever a translator submitted, so this response
// can carry markup that no document ever contained.
func (s *Server) HandleRenderBlockHTML(c echo.Context) error {
	// Set before any branch, as in HandleRenderDocumentPreview.
	applySandboxCSP(c)

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
	parts := make([]*model.Part, 0, len(blocks))
	for _, sb := range blocks {
		if !sb.Block.Translatable {
			continue
		}
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: sb.Block})
	}
	return editor.BuildGenericPreview(parts)
}
