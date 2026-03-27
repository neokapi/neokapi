package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/platform/auth"

	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/id"
	corestorage "github.com/neokapi/neokapi/core/storage"
	apiclient "github.com/neokapi/neokapi/platform/client"
	"github.com/neokapi/neokapi/platform/store"
)

// HandleSyncPush receives source blocks from a client, writes them to blob
// storage, and enqueues a background job for processing (AD-037).
// Returns 202 Accepted with a push_id for status polling.
func (s *Server) HandleSyncPush(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	if s.BlobStore == nil || s.JobStore == nil || s.JobQueue == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "async push not configured (blob store or job queue missing)"})
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

	projectID := c.Param("id")
	userID, _ := c.Get("user_id").(string)
	pushID := id.New()

	stream := c.Param("stream")
	if stream == "" {
		stream = "main"
	}

	return s.handleAsyncPush(c, &req, projectID, pushID, stream, userID)
}

// syncPushPayload wraps the push request with context needed by the worker.
type syncPushPayload struct {
	Request apiclient.SyncPushRequest `json:"request"`
	UserID  string                    `json:"user_id"`
	WsSlug  string                    `json:"ws_slug"`
}

// handleAsyncPush writes the push payload to blob storage and enqueues a job
// for background processing (AD-037).
func (s *Server) handleAsyncPush(c echo.Context, req *apiclient.SyncPushRequest, projectID, pushID, stream, userID string) error {
	wsSlug := ""
	if ws, ok := c.Get("workspace_slug").(string); ok {
		wsSlug = ws
	}
	payload := syncPushPayload{
		Request: *req,
		UserID:  userID,
		WsSlug:  wsSlug,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to serialize push payload"})
	}

	ref, err := s.BlobStore.Upload(c.Request().Context(), data, corestorage.UploadOptions{
		ContentType: "application/json",
		Filename:    fmt.Sprintf("push-%s.json", pushID),
	})
	blobKey := ref.Key
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to store push payload"})
	}

	// Create a sync-push translation job. The worker identifies this as a
	// sync push by ItemName="__sync_push__". The blob key is stored in Model
	// (unused for sync pushes). Stream is stored in TargetLocale.
	job := &jobs.TranslationJob{
		ID:            pushID,
		WorkspaceSlug: func() string { ws, _ := c.Get("workspace_slug").(string); return ws }(),
		ProjectID:     projectID,
		ItemName:      "__sync_push__",
		TargetLocale:  stream,
		Model:         blobKey, // blob storage key for the push payload
		PushID:        pushID,
		Status:        jobs.StatusQueued,
	}
	if err := s.JobStore.CreateJob(c.Request().Context(), job); err != nil {
		// Clean up blob on failure.
		_ = s.BlobStore.Delete(c.Request().Context(), blobKey)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to create push job"})
	}
	if err := s.JobQueue.Enqueue(c.Request().Context(), pushID); err != nil {
		_ = s.JobStore.DeleteJob(c.Request().Context(), pushID)
		_ = s.BlobStore.Delete(c.Request().Context(), blobKey)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue push job"})
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"push_id": pushID,
		"status":  "queued",
		"message": "Push accepted for background processing",
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

	limit := 1000
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > store.DefaultBlockLimit {
		limit = store.DefaultBlockLimit
	}
	offset := 0
	if o := c.QueryParam("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	query := store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
		ItemName:  itemName,
		Limit:     limit,
		Offset:    offset,
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
