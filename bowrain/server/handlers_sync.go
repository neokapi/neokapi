package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/storage/compression"
)

// HandleSyncPushInit handles the first step of a push: Merkle tree diff negotiation.
// POST /sync/push/init
func (s *Server) HandleSyncPushInit(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	var req struct {
		ProjectID    string            `json:"project_id"`
		Stream       string            `json:"stream"`
		ContentTypes []string          `json:"content_types"`
		ItemHashes   map[string]string `json:"item_hashes"`
		RootHash     string            `json:"root_hash"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	// Always authorize and operate on the path-scoped project. The permission
	// middleware resolved access against c.Param("id"); a client-supplied
	// project_id is ignored to prevent cross-project access (IDOR).
	req.ProjectID = c.Param("id")
	if req.Stream == "" {
		req.Stream = c.Param("stream")
	}
	if req.Stream == "" {
		req.Stream = "main"
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "content store not configured"})
	}

	diffEngine := bowsync.NewDiffEngine(s.ContentStore, s.SyncCache)

	// Fast path: root hash comparison.
	if req.RootHash != "" {
		unchanged, err := diffEngine.CheckRootHash(c.Request().Context(), req.ProjectID, req.Stream, req.RootHash)
		if err == nil && unchanged {
			return c.JSON(http.StatusOK, map[string]any{
				"upload_id": "",
				"status":    "unchanged",
			})
		}
	}

	// Full diff: compare item hashes.
	itemDiff, err := diffEngine.CompareItems(c.Request().Context(), req.ProjectID, req.Stream, req.ItemHashes)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	uploadID := id.New()

	return c.JSON(http.StatusOK, map[string]any{
		"upload_id":            uploadID,
		"status":               "diff_computed",
		"changed_items":        itemDiff.ChangedItems,
		"new_items":            itemDiff.NewItems,
		"deleted_items":        itemDiff.DeletedItems,
		"unchanged_item_count": itemDiff.UnchangedCount,
	})
}

// HandleSyncPushDiff handles block-level diff negotiation for a single item.
// POST /sync/push/diff
func (s *Server) HandleSyncPushDiff(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	var req struct {
		UploadID    string            `json:"upload_id"`
		ItemName    string            `json:"item_name"`
		BlockHashes map[string]string `json:"block_hashes"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	projectID := c.Param("id")
	stream := c.Param("stream")
	if stream == "" {
		stream = "main"
	}

	diffEngine := bowsync.NewDiffEngine(s.ContentStore, s.SyncCache)

	blockDiff, err := diffEngine.CompareBlocks(c.Request().Context(), projectID, stream, req.ItemName, req.BlockHashes)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Generate chunk upload URLs if blob store supports it.
	var chunkURLs []string
	transport := "proxy"
	if chunkedStore, ok := s.BlobStore.(storage.ChunkedBlobStore); ok {
		estimatedChunks := (len(blockDiff.Needed) / 500) + 1
		urls, err := chunkedStore.GenerateChunkUploadURLs(c.Request().Context(), req.UploadID, estimatedChunks, storage.SignOptions{})
		if err == nil && len(urls) > 0 {
			chunkURLs = urls
			transport = "direct"
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"needed":     blockDiff.Needed,
		"deleted":    blockDiff.Deleted,
		"conflicts":  blockDiff.Conflicts,
		"chunk_urls": chunkURLs,
		"transport":  transport,
	})
}

// HandleSyncPushCommit finalizes a push by validating the manifest and enqueuing the worker job.
// POST /sync/push/commit
func (s *Server) HandleSyncPushCommit(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	var manifest struct {
		UploadID      string          `json:"upload_id"`
		ProjectID     string          `json:"project_id"`
		Stream        string          `json:"stream"`
		Chunks        []chunkRef      `json:"chunks"`
		Items         json.RawMessage `json:"items"`
		ActorID       string          `json:"actor_id"`
		WorkspaceSlug string          `json:"workspace_slug"`
		ConnectorID   string          `json:"connector_id"`
	}
	if err := c.Bind(&manifest); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	// Force the project and actor to the authorized path/identity. The
	// permission middleware resolved access against c.Param("id") and the
	// authenticated user; a client-supplied project_id / actor_id / workspace
	// is not trusted (prevents writing into another tenant's project, IDOR).
	manifest.ProjectID = c.Param("id")
	if manifest.Stream == "" {
		manifest.Stream = c.Param("stream")
	}
	if manifest.Stream == "" {
		manifest.Stream = "main"
	}
	manifest.ActorID, _ = c.Get("user_id").(string)
	// Resolve the workspace slug from the path-scoped project rather than
	// trusting the client. Best-effort: the worker tolerates an empty slug.
	manifest.WorkspaceSlug = ""
	if s.ContentStore != nil {
		if proj, err := s.ContentStore.GetProject(c.Request().Context(), manifest.ProjectID); err == nil && proj != nil && proj.WorkspaceID != "" {
			if s.AuthStore != nil {
				if ws, err := s.AuthStore.GetWorkspace(c.Request().Context(), proj.WorkspaceID); err == nil && ws != nil {
					manifest.WorkspaceSlug = ws.Slug
				}
			}
		}
	}

	// Validate chunks exist. For ChunkedBlobStore (proxy uploads), chunks are
	// in the upload session, not content-addressed storage.
	if _, isChunked := s.BlobStore.(storage.ChunkedBlobStore); !isChunked {
		for _, chunk := range manifest.Chunks {
			exists, err := s.BlobStore.Exists(c.Request().Context(), chunk.Hash)
			if err != nil || !exists {
				return c.JSON(http.StatusBadRequest, ErrorResponse{
					Error: fmt.Sprintf("chunk %d (hash %s) not found in storage", chunk.Index, chunk.Hash),
				})
			}
		}
	}

	// Enforce upload budget.
	maxPushBytes := s.Config.MaxPushBytes
	if maxPushBytes <= 0 {
		maxPushBytes = 256 * 1024 * 1024 // default 256MB
	}
	var totalBytes int64
	for _, chunk := range manifest.Chunks {
		totalBytes += chunk.ByteSize
	}
	if totalBytes > maxPushBytes {
		return c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
			Error: fmt.Sprintf("upload budget exceeded: %d bytes > %d bytes max", totalBytes, maxPushBytes),
		})
	}

	pushID := id.New()

	// Serialize manifest for the worker.
	manifestJSON, _ := json.Marshal(manifest)

	// Store manifest as a blob for the worker to read.
	ref, err := s.BlobStore.Upload(c.Request().Context(), manifestJSON, storage.UploadOptions{
		ContentType: "application/json",
		Filename:    fmt.Sprintf("manifest-%s.json", pushID),
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to store manifest"})
	}

	// Enqueue the push job.
	if s.JobStore != nil && s.JobQueue != nil {
		job := &jobs.TranslationJob{
			ID:            pushID,
			WorkspaceSlug: manifest.WorkspaceSlug,
			ProjectID:     manifest.ProjectID,
			ItemName:      "__sync_push__",
			TargetLocale:  manifest.Stream,
			Model:         ref.Key, // manifest blob key
			PushID:        pushID,
			Status:        jobs.StatusQueued,
		}
		if err := s.JobStore.CreateJob(c.Request().Context(), job); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to create push job"})
		}
		if err := s.JobQueue.Enqueue(c.Request().Context(), pushID); err != nil {
			_ = s.JobStore.DeleteJob(c.Request().Context(), pushID)
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue push job"})
		}
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"push_id": pushID,
		"status":  "queued",
	})
}

// HandleSyncProxyChunkUpload handles chunk uploads for the proxy transport mode
// (local dev / self-hosted without Azure Blob SAS URLs).
// PUT /sync/push/chunks/:uploadId/:chunkIndex
func (s *Server) HandleSyncProxyChunkUpload(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	uploadID := c.Param("uploadId")
	chunkIndex := 0
	_, _ = fmt.Sscanf(c.Param("chunkIndex"), "%d", &chunkIndex)

	data, err := readBody(c, 2*1024*1024) // 2MB max per chunk
	if err != nil {
		return c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: "chunk too large"})
	}

	// Store chunk as a content-addressed blob. The worker later downloads each
	// chunk by its hash (from the commit manifest), so we need the chunk to be
	// accessible via BlobStore.Download(hash). Content-addressed Upload gives
	// us a stable key that matches the SHA-256 the client computes.
	if _, err := s.BlobStore.Upload(c.Request().Context(), data, storage.UploadOptions{
		ContentType: "application/octet-stream",
		Filename:    fmt.Sprintf("chunks/%s/%04d", uploadID, chunkIndex),
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

type chunkRef struct {
	Index       int    `json:"index"`
	ContentType string `json:"content_type"`
	Hash        string `json:"hash"`
	RecordCount int    `json:"record_count"`
	ByteSize    int64  `json:"byte_size"`
}

// readBody reads up to maxBytes from the request body.
func readBody(c echo.Context, maxBytes int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(c.Request().Body, maxBytes))
}

// HandleSyncPull returns full blocks, terms, and media for a project since the
// given cursor. The response is a RichPullResponse (Bowrain AD-009 Phase 7) with structured
// SyncBlock records instead of raw change log entries. When the client sends
// Accept-Encoding: zstd, the response is zstd-compressed.
func (s *Server) HandleSyncPull(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	ctx := c.Request().Context()
	projectID := c.Param("id")
	cursor, _ := strconv.ParseInt(c.QueryParam("cursor"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 1000
	}

	stream := c.Param("stream")
	if stream == "" {
		stream = "main"
	}

	var locales []string
	if raw := c.QueryParam("locales"); raw != "" {
		for l := range strings.SplitSeq(raw, ",") {
			if t := strings.TrimSpace(l); t != "" {
				locales = append(locales, t)
			}
		}
	}

	// Get change log entries to determine what changed.
	cs, err := s.Services.Project.GetChanges(ctx, projectID, cursor, locales, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	resp := apiclient.RichPullResponse{
		Cursor:  cs.NewCursor,
		HasMore: cs.HasMore,
	}

	if len(cs.Changes) > 0 {
		// Collect unique block IDs from the change log.
		blockIDSet := make(map[string]struct{})
		itemSet := make(map[string]struct{})
		for _, ch := range cs.Changes {
			if ch.ChangeType != "source_removed" {
				blockIDSet[ch.BlockID] = struct{}{}
			}
		}

		if len(blockIDSet) > 0 {
			blockIDs := make([]string, 0, len(blockIDSet))
			for id := range blockIDSet {
				blockIDs = append(blockIDs, id)
			}

			// Fetch full blocks from the store.
			query := store.BlockQuery{
				ProjectID: projectID,
				Stream:    stream,
				IDs:       blockIDs,
				Limit:     len(blockIDs),
			}
			stored, err := s.Services.Project.GetBlocks(ctx, query)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}

			resp.Blocks = make([]apiclient.SyncBlock, 0, len(stored))
			for _, sb := range stored {
				resp.Blocks = append(resp.Blocks, apiclient.StoredBlockToSyncBlock(sb))
				itemSet[sb.ItemName] = struct{}{}
			}
		}

		// Fetch media assets for affected items.
		if s.ContentStore != nil && len(itemSet) > 0 {
			for itemName := range itemSet {
				assets, err := s.ContentStore.ListAssets(ctx, projectID, stream, itemName)
				if err != nil {
					continue // best-effort: skip media on error
				}
				for _, a := range assets {
					resp.Media = append(resp.Media, apiclient.AssetToSyncMedia(a))
				}
			}
		}
	}

	return writePullResponse(c, resp)
}

// syncCompressorPool is a lazily-initialized zstd compression pool for pull responses.
var syncCompressorPool = compression.NewPool(nil)

// writePullResponse marshals the response as JSON and optionally compresses with zstd.
func writePullResponse(c echo.Context, resp apiclient.RichPullResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "marshal response"})
	}

	// Compress with zstd if the client accepts it.
	if strings.Contains(c.Request().Header.Get("Accept-Encoding"), "zstd") {
		compressed, err := syncCompressorPool.Compress(data)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "compress response"})
		}
		c.Response().Header().Set("Content-Encoding", "zstd")
		c.Response().Header().Set("Content-Type", "application/json")
		return c.Blob(http.StatusOK, "application/json", compressed)
	}

	return c.JSONBlob(http.StatusOK, data)
}

// HandleSyncGetBlocks returns blocks with full structured content for a specific item.
// Returns []SyncBlock with segments, spans, annotations, and metadata.
func (s *Server) HandleSyncGetBlocks(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
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

	result := make([]apiclient.SyncBlock, len(blocks))
	for i, sb := range blocks {
		result[i] = apiclient.StoredBlockToSyncBlock(sb)
	}

	return c.JSON(http.StatusOK, result)
}

// HandleSyncPushStatus returns the aggregated status of jobs triggered by a push.
// GET /api/v1/projects/:id/sync/status?push_id=xxx
func (s *Server) HandleSyncPushStatus(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
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
