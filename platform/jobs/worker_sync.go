package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	platev "github.com/neokapi/neokapi/platform/event"
	"github.com/neokapi/neokapi/platform/store"
	"google.golang.org/protobuf/proto"

	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
)

// syncPushManifest matches the JSON manifest written by HandleSyncPushCommit.
type syncPushManifest struct {
	UploadID      string          `json:"upload_id"`
	ProjectID     string          `json:"project_id"`
	Stream        string          `json:"stream"`
	Chunks        []syncChunkRef  `json:"chunks"`
	Items         json.RawMessage `json:"items"`
	ActorID       string          `json:"actor_id"`
	WorkspaceSlug string          `json:"workspace_slug"`
	ConnectorID   string          `json:"connector_id"`
}

type syncChunkRef struct {
	Index       int    `json:"index"`
	ContentType string `json:"content_type"`
	Hash        string `json:"hash"`
	RecordCount int    `json:"record_count"`
	ByteSize    int64  `json:"byte_size"`
}

// processSyncPushJob handles the sync protocol push (AD-038).
// It reads the manifest, downloads chunks, deserializes protobuf,
// and stores content via the full model.
func processSyncPushJob(ctx context.Context, deps *WorkerDeps, job *TranslationJob) error {
	manifestKey := job.Model
	projectID := job.ProjectID
	pushID := job.PushID

	if deps.BlobStore == nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "blob store not configured")
		return fmt.Errorf("blob store not configured")
	}

	emitLog(deps, job.StepID, "info", "Processing sync push",
		map[string]string{"project": projectID, "push_id": pushID})

	// 1. Download and parse manifest.
	reader, err := deps.BlobStore.Download(ctx, manifestKey)
	if err != nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "manifest download failed")
		return fmt.Errorf("download manifest: %w", err)
	}
	manifestData, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "manifest read failed")
		return fmt.Errorf("read manifest: %w", err)
	}

	var manifest syncPushManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "invalid manifest")
		return fmt.Errorf("parse manifest: %w", err)
	}

	stream := manifest.Stream
	if stream == "" {
		stream = "main"
	}

	// Auto-create non-main streams.
	if stream != "main" {
		if _, err := deps.ContentStore.GetStream(ctx, projectID, stream); err != nil {
			baseCursor, _ := deps.ContentStore.LatestCursor(ctx, projectID, "main")
			_ = deps.ContentStore.CreateStream(ctx, &store.Stream{
				ProjectID:  projectID,
				Name:       stream,
				Parent:     "main",
				BaseCursor: baseCursor,
				Visibility: store.StreamPublic,
			})
		}
	}

	// Parse item metadata.
	var itemMetas []pb.SyncItemMeta
	if len(manifest.Items) > 0 {
		_ = json.Unmarshal(manifest.Items, &itemMetas)
	}
	itemMetaMap := map[string]*pb.SyncItemMeta{}
	for i := range itemMetas {
		itemMetaMap[itemMetas[i].Name] = &itemMetas[i]
	}

	// 2. Process each chunk.
	totalStored := 0
	var allItemNames []string

	for _, chunkRef := range manifest.Chunks {
		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Processing chunk %d (%s, %d records)", chunkRef.Index, chunkRef.ContentType, chunkRef.RecordCount),
			nil)

		// Download chunk.
		chunkReader, err := deps.BlobStore.Download(ctx, chunkRef.Hash)
		if err != nil {
			_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed,
				fmt.Sprintf("chunk %d download failed: %s", chunkRef.Index, err.Error()))
			return fmt.Errorf("download chunk %d: %w", chunkRef.Index, err)
		}
		chunkData, err := io.ReadAll(chunkReader)
		chunkReader.Close()
		if err != nil {
			_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "chunk read failed")
			return fmt.Errorf("read chunk %d: %w", chunkRef.Index, err)
		}

		// Attempt zstd decompression (compressed chunks start with zstd magic bytes).
		if deps.Decompressor != nil && len(chunkData) > 4 {
			if decompressed, err := deps.Decompressor.Decompress(chunkData); err == nil {
				chunkData = decompressed
			}
			// If decompression fails, assume uncompressed data and continue.
		}

		// Deserialize protobuf SyncChunk.
		var chunk pb.SyncChunk
		if err := proto.Unmarshal(chunkData, &chunk); err != nil {
			_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "invalid chunk data")
			return fmt.Errorf("unmarshal chunk %d: %w", chunkRef.Index, err)
		}

		// Route by content type.
		switch chunk.ContentType {
		case "blocks":
			stored, itemNames, err := processBlockChunk(ctx, deps, &chunk, projectID, stream, itemMetaMap)
			if err != nil {
				_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, err.Error())
				return err
			}
			totalStored += stored
			allItemNames = append(allItemNames, itemNames...)

		case "terms":
			log.Printf("sync: term processing not yet implemented (chunk %d)", chunkRef.Index)

		case "tm":
			log.Printf("sync: TM processing not yet implemented (chunk %d)", chunkRef.Index)

		case "media":
			log.Printf("sync: media processing not yet implemented (chunk %d)", chunkRef.Index)
		}
	}

	// Auto-set project default stream.
	if totalStored > 0 {
		proj, projErr := deps.ContentStore.GetProject(ctx, projectID)
		if projErr == nil && proj.DefaultStream == "" {
			proj.DefaultStream = stream
			_ = deps.ContentStore.UpdateProject(ctx, proj)
		}
	}

	// Mark completed and clean up.
	_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusCompleted, "")
	_ = deps.BlobStore.Delete(ctx, manifestKey)

	// Publish EventPushCompleted.
	if totalStored > 0 && deps.EventBus != nil {
		deps.EventBus.Publish(platev.Event{
			Type:      platev.EventPushCompleted,
			Source:    "sync-worker",
			ProjectID: projectID,
			Actor:     manifest.ActorID,
			Data: map[string]string{
				"items":          strings.Join(allItemNames, ","),
				"push_id":        pushID,
				"workspace_slug": manifest.WorkspaceSlug,
			},
		})
	}

	emitLog(deps, job.StepID, "info",
		fmt.Sprintf("Sync push completed: %d blocks across %d items", totalStored, len(allItemNames)),
		nil)

	return nil
}

// processBlockChunk converts SyncBlocks to model.Blocks and stores them.
func processBlockChunk(ctx context.Context, deps *WorkerDeps, chunk *pb.SyncChunk, projectID, stream string, itemMetas map[string]*pb.SyncItemMeta) (int, []string, error) {
	// Group blocks by item.
	itemGroups := map[string][]*model.Block{}
	for _, sb := range chunk.Blocks {
		b := bowsync.ProtoToBlock(sb)
		itemGroups[sb.ItemName] = append(itemGroups[sb.ItemName], b)
	}

	stored := 0
	var itemNames []string
	for itemName, blocks := range itemGroups {
		if itemName != "" {
			if err := deps.ContentStore.StoreBlocksForItem(ctx, projectID, stream, itemName, blocks); err != nil {
				return stored, itemNames, fmt.Errorf("store blocks for %s: %w", itemName, err)
			}
			// Ensure item exists with rich metadata.
			item := &store.Item{
				Name:     itemName,
				Format:   "json", // default
				ItemType: "file",
			}
			if meta, ok := itemMetas[itemName]; ok {
				if meta.Format != "" {
					item.Format = meta.Format
				}
				item.BlockIndex = meta.BlockIndexJson
				item.PreviewHTML = meta.PreviewHtml
				// TODO: resolve collection name to ID if meta.Collection is set
			}
			_ = deps.ContentStore.StoreItem(ctx, projectID, stream, item)
			itemNames = append(itemNames, itemName)
		} else {
			if err := deps.ContentStore.StoreBlocks(ctx, projectID, stream, blocks); err != nil {
				return stored, itemNames, fmt.Errorf("store blocks: %w", err)
			}
		}
		stored += len(blocks)
	}

	return stored, itemNames, nil
}
