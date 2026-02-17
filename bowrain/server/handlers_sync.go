package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/bowrain/store"
	"github.com/labstack/echo/v4"
)

// SyncPushRequest is the request body for pushing source blocks.
type SyncPushRequest struct {
	Blocks []BlockInput `json:"blocks"`
}

// SyncPushResponse is the response for a sync push.
type SyncPushResponse struct {
	Stored    int   `json:"stored"`
	NewCursor int64 `json:"new_cursor"`
}

// SyncPullResponse is the response for a sync pull.
type SyncPullResponse struct {
	Changes   []store.ChangeEntry `json:"changes"`
	NewCursor int64               `json:"new_cursor"`
	HasMore   bool                `json:"has_more"`
}

// HandleSyncPush receives source blocks from a client and stores them.
func (s *Server) HandleSyncPush(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req SyncPushRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if len(req.Blocks) > store.MaxBlocksPerRequest {
		return c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
			Error: fmt.Sprintf("batch size %d exceeds limit %d", len(req.Blocks), store.MaxBlocksPerRequest),
		})
	}

	blocks := make([]*model.Block, len(req.Blocks))
	for i, bi := range req.Blocks {
		b := model.NewBlock(bi.ID, bi.Text)
		b.Name = bi.Name
		b.Type = bi.Type
		blocks[i] = b
	}

	projectID := c.Param("id")
	if err := s.Services.Project.StoreBlocks(c.Request().Context(), projectID, blocks); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	cursor, err := s.Services.Project.LatestCursor(c.Request().Context(), projectID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, SyncPushResponse{
		Stored:    len(blocks),
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

	// Support comma-separated locales; use the first one for filtering.
	locale := ""
	if locales := c.QueryParam("locales"); locales != "" {
		parts := strings.Split(locales, ",")
		if len(parts) > 0 {
			locale = strings.TrimSpace(parts[0])
		}
	}

	cs, err := s.Services.Project.GetChanges(c.Request().Context(), projectID, cursor, locale, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, SyncPullResponse{
		Changes:   cs.Changes,
		NewCursor: cs.NewCursor,
		HasMore:   cs.HasMore,
	})
}

// HandleGetChanges returns raw change log entries for a project.
func (s *Server) HandleGetChanges(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	cursor, _ := strconv.ParseInt(c.QueryParam("cursor"), 10, 64)
	locale := c.QueryParam("locale")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 100
	}

	cs, err := s.Services.Project.GetChanges(c.Request().Context(), projectID, cursor, locale, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, cs)
}
