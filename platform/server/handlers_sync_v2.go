package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/storage"
	platauth "github.com/neokapi/neokapi/platform/auth"
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
	if req.ProjectID == "" {
		req.ProjectID = c.Param("id")
	}
	if req.Stream == "" {
		req.Stream = c.Param("stream")
	}
	if req.Stream == "" {
		req.Stream = "main"
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "content store not configured"})
	}

	diffEngine := bowsync.NewDiffEngine(s.ContentStore, nil) // TODO: wire Redis cache

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

	diffEngine := bowsync.NewDiffEngine(s.ContentStore, nil)

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
	if manifest.ProjectID == "" {
		manifest.ProjectID = c.Param("id")
	}
	if manifest.Stream == "" {
		manifest.Stream = c.Param("stream")
	}
	if manifest.Stream == "" {
		manifest.Stream = "main"
	}
	if manifest.ActorID == "" {
		manifest.ActorID, _ = c.Get("user_id").(string)
	}
	if manifest.WorkspaceSlug == "" {
		manifest.WorkspaceSlug, _ = c.Get("workspace_slug").(string)
	}

	// Validate all chunks exist in blob storage.
	for _, chunk := range manifest.Chunks {
		exists, err := s.BlobStore.Exists(c.Request().Context(), chunk.Hash)
		if err != nil || !exists {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("chunk %d (hash %s) not found in storage", chunk.Index, chunk.Hash),
			})
		}
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
			ItemName:      "__sync_push_v2__",
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

	// Store chunk via BlobStore.
	if chunkedStore, ok := s.BlobStore.(storage.ChunkedBlobStore); ok {
		if err := chunkedStore.StageChunk(c.Request().Context(), uploadID, chunkIndex, data); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	} else {
		// Fallback: store as individual blob with predictable key.
		key := fmt.Sprintf("chunks/%s/%04d", uploadID, chunkIndex)
		if _, err := s.BlobStore.Upload(c.Request().Context(), data, storage.UploadOptions{
			Filename: key,
		}); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
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
	r := http.MaxBytesReader(nil, c.Request().Body, maxBytes)
	defer r.Close()
	data := make([]byte, 0, 1024)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if err != nil {
			if err.Error() == "http: request body too large" {
				return nil, err
			}
			break
		}
	}
	return data, nil
}
