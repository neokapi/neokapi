package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/bowrain/jobs"
	"github.com/gokapi/gokapi/core/model"
	apiclient "github.com/gokapi/gokapi/platform/client"
	platev "github.com/gokapi/gokapi/platform/event"
	"github.com/gokapi/gokapi/core/id"
	"github.com/gokapi/gokapi/platform/store"
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
	stream := c.Param("stream")
	if stream == "" {
		stream = "main"
	}

	// Auto-create non-main streams on first push.
	if stream != "main" && s.ContentStore != nil {
		if _, err := s.ContentStore.GetStream(ctx, projectID, stream); err != nil {
			baseCursor, _ := s.ContentStore.LatestCursor(ctx, projectID, "main")
			_ = s.ContentStore.CreateStream(ctx, &store.Stream{
				ProjectID:  projectID,
				Name:       stream,
				Parent:     "main",
				BaseCursor: baseCursor,
				Visibility: store.StreamPublic,
			})
		}
	}

	pushID := id.New()
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
			// Ensure the item exists in the ContentStore so it appears in the editor UI.
			if s.ContentStore != nil {
				_ = s.ContentStore.StoreItem(ctx, projectID, stream, &store.Item{
					Name:     itemName,
					Format:   detectFormatFromName(itemName),
					ItemType: "file",
				})
			}
		}
		totalStored += len(blocks)
	}

	cursor, err := s.Services.Project.LatestCursor(ctx, projectID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Publish push completed event if blocks were stored.
	if totalStored > 0 && s.EventBus != nil {
		var itemNames []string
		for name := range itemGroups {
			if name != "" {
				itemNames = append(itemNames, name)
			}
		}

		// Determine workspace slug from context.
		wsSlug := ""
		if ws, ok := c.Get("workspace_slug").(string); ok {
			wsSlug = ws
		}

		s.EventBus.Publish(platev.Event{
			Type:      platev.EventPushCompleted,
			Source:    "sync",
			ProjectID: projectID,
			Data: map[string]string{
				"items":          strings.Join(itemNames, ","),
				"push_id":        pushID,
				"workspace_slug": wsSlug,
			},
		})
	}

	return c.JSON(http.StatusOK, apiclient.SyncPushResponse{
		Stored:    totalStored,
		NewCursor: cursor,
		PushID:    pushID,
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

	stream := c.Param("stream")
	if stream == "" {
		stream = "main"
	}

	query := store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
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

// HandleSyncPushStatus returns the aggregated status of jobs triggered by a push.
// GET /api/v1/projects/:id/sync/status?push_id=xxx
func (s *Server) HandleSyncPushStatus(c echo.Context) error {
	if s.JobStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}

	pushID := c.QueryParam("push_id")
	if pushID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "push_id is required"})
	}

	jobList, err := s.JobStore.ListJobsByPushID(c.Request().Context(), pushID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	total := len(jobList)
	completed := 0
	failed := 0
	inProgress := 0

	for _, j := range jobList {
		switch j.Status {
		case jobs.StatusCompleted:
			completed++
		case jobs.StatusFailed:
			failed++
		case jobs.StatusProcessing, jobs.StatusQueued:
			inProgress++
		}
	}

	status := "completed"
	if inProgress > 0 {
		status = "in_progress"
	} else if failed > 0 && completed == 0 {
		status = "failed"
	}

	return c.JSON(http.StatusOK, map[string]any{
		"push_id":     pushID,
		"status":      status,
		"total":       total,
		"completed":   completed,
		"failed":      failed,
		"in_progress": inProgress,
	})
}

// detectFormatFromName infers a format identifier from the file extension.
func detectFormatFromName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		return "json"
	case ".md", ".mdx":
		return "markdown"
	case ".html", ".htm":
		return "html"
	case ".xml":
		return "xml"
	case ".yaml", ".yml":
		return "yaml"
	case ".po":
		return "po"
	case ".properties":
		return "properties"
	case ".csv":
		return "csv"
	case ".txt":
		return "plaintext"
	default:
		return filepath.Ext(name)
	}
}
