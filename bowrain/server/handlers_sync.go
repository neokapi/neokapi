package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/platform/store"
	"github.com/gokapi/gokapi/core/model"
	apiclient "github.com/gokapi/gokapi/platform/client"
	"github.com/labstack/echo/v4"
)

// HandleSyncPush receives source blocks from a client and stores them.
func (s *Server) HandleSyncPush(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req apiclient.SyncPushRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if len(req.Blocks) > store.MaxBlocksPerRequest {
		return c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
			Error: fmt.Sprintf("batch size %d exceeds limit %d", len(req.Blocks), store.MaxBlocksPerRequest),
		})
	}

	// Group blocks by item_name for per-item storage.
	itemGroups := map[string][]*model.Block{}
	for _, bi := range req.Blocks {
		b := model.NewBlock(bi.ID, bi.Text)
		b.Name = bi.Name
		b.Type = bi.Type
		itemGroups[bi.ItemName] = append(itemGroups[bi.ItemName], b)
	}

	projectID := c.Param("id")
	ctx := c.Request().Context()
	totalStored := 0
	for itemName, blocks := range itemGroups {
		if itemName == "" {
			if err := s.Services.Project.StoreBlocks(ctx, projectID, blocks); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		} else {
			if err := s.Services.Project.StoreBlocksForItem(ctx, projectID, itemName, blocks); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		}
		totalStored += len(blocks)
	}

	cursor, err := s.Services.Project.LatestCursor(ctx, projectID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, apiclient.SyncPushResponse{
		Stored:    totalStored,
		NewCursor: cursor,
	})
}

// HandleSyncPull returns change log entries for a project, optionally scoped by locale.
func (s *Server) HandleSyncPull(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	cursor, _ := strconv.ParseInt(c.QueryParam("cursor"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 100
	}

	var locales []string
	if raw := c.QueryParam("locales"); raw != "" {
		for _, l := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(l); t != "" {
				locales = append(locales, t)
			}
		}
	}

	cs, err := s.Services.Project.GetChanges(c.Request().Context(), projectID, cursor, locales, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, cs)
}

// HandleGetChanges returns raw change log entries for a project.
func (s *Server) HandleGetChanges(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	cursor, _ := strconv.ParseInt(c.QueryParam("cursor"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 100
	}

	var locales []string
	if raw := c.QueryParam("locales"); raw != "" {
		for _, l := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(l); t != "" {
				locales = append(locales, t)
			}
		}
	} else if single := c.QueryParam("locale"); single != "" {
		locales = []string{single}
	}

	cs, err := s.Services.Project.GetChanges(c.Request().Context(), projectID, cursor, locales, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, cs)
}

// HandleSyncGetBlocks returns blocks with their translations for a specific item.
func (s *Server) HandleSyncGetBlocks(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	itemName := c.QueryParam("item_name")

	query := store.BlockQuery{
		ProjectID: projectID,
		ItemName:  itemName,
	}

	blocks, err := s.Services.Project.GetBlocks(c.Request().Context(), query)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Convert stored blocks to the wire format with targets.
	result := make([]apiclient.BlockContent, len(blocks))
	for i, b := range blocks {
		targets := make(map[string]string)
		for locale := range b.Block.Targets {
			targets[string(locale)] = b.Block.TargetText(locale)
		}
		result[i] = apiclient.BlockContent{
			ID:       b.Block.ID,
			Name:     b.Block.Name,
			ItemName: b.ItemName,
			Source:   b.Block.SourceText(),
			Targets:  targets,
		}
	}

	return c.JSON(http.StatusOK, result)
}
