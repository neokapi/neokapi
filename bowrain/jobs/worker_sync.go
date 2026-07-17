package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/bowrain/analytics"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"google.golang.org/protobuf/proto"

	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	bowsync "github.com/neokapi/neokapi/bowrain/core/sync"
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

// processSyncPushJob handles the sync protocol push (Bowrain AD-009).
// It reads the manifest, downloads chunks, deserializes protobuf,
// and stores content via the full model.
func processSyncPushJob(ctx context.Context, deps *WorkerDeps, job *TranslationJob) error {
	manifestKey := job.Model
	projectID := job.ProjectID
	pushID := job.PushID

	if deps.BlobStore == nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "blob store not configured")
		return errors.New("blob store not configured")
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

	// Resolve the project TM (optional) and source language once, so ingest can
	// seed pushed target translations into the TM for future recycling (theme
	// A2). A missing TM or source language simply disables seeding — never an
	// error on the push path.
	var seedTM sievepen.TMStore
	var sourceLocale model.LocaleID
	if deps.TMResolver != nil {
		slug := manifest.WorkspaceSlug
		if slug == "" {
			slug = "_anon"
		}
		if tm, terr := deps.TMResolver.GetTM(slug); terr == nil {
			seedTM = tm
		}
		if proj, perr := deps.ContentStore.GetProject(ctx, projectID); perr == nil {
			sourceLocale = proj.DefaultSourceLanguage
		}
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
			stored, itemNames, err := processBlockChunk(ctx, deps, &chunk, projectID, stream, itemMetaMap, seedTM, sourceLocale)
			if err != nil {
				_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, err.Error())
				return err
			}
			totalStored += stored
			allItemNames = append(allItemNames, itemNames...)

		case "terms", "tm", "media":
			// These content types are not yet implemented. Rather than silently
			// dropping the payload and marking the job Completed (which would lose
			// data for any client that emits them), fail the job explicitly.
			err := fmt.Errorf("sync: content type %q (chunk %d) is not yet supported", chunk.ContentType, chunkRef.Index)
			_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, err.Error())
			return err

		default:
			err := fmt.Errorf("sync: unknown content type %q (chunk %d)", chunk.ContentType, chunkRef.Index)
			_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, err.Error())
			return err
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

	// Fire-and-forget analytics after the push succeeded (epic 018 workstream
	// D). Counts are bucketed; item names and content never leave the server.
	if deps.Tracker != nil {
		actor := manifest.ActorID
		if actor == "" {
			actor = "server"
		}
		props := analytics.Props("", projectID)
		props["item_count"] = len(allItemNames)
		props["block_count_bucket"] = analytics.CountBucket(totalStored)
		if manifest.WorkspaceSlug != "" {
			props["workspace_slug"] = manifest.WorkspaceSlug
		}
		deps.Tracker.CaptureEvent(actor, analytics.EventContentPushed, props)
	}

	emitLog(deps, job.StepID, "info",
		fmt.Sprintf("Sync push completed: %d blocks across %d items", totalStored, len(allItemNames)),
		nil)

	return nil
}

// processBlockChunk converts SyncBlocks to model.Blocks and stores them.
// Blocks with ExpectedHash set are checked for optimistic concurrency conflicts.
//
// tm (optional) is the project's server TM: when a stored block arrives with an
// existing target translation, that pair is seeded into the TM so a future
// convergence recycles it instead of paying AI (theme A2, "arrives with
// translations → recycles for free"). Seeding is idempotent (content-hash keyed
// entry IDs) and best-effort — a seed failure never fails the push.
func processBlockChunk(ctx context.Context, deps *WorkerDeps, chunk *pb.SyncChunk, projectID, stream string, itemMetas map[string]*pb.SyncItemMeta, tm sievepen.TMStore, sourceLocale model.LocaleID) (int, []string, error) {
	// Check expected_hash conflict detection (optimistic concurrency).
	for _, sb := range chunk.Blocks {
		if sb.ExpectedHash == "" {
			continue
		}
		existing, err := deps.ContentStore.GetBlock(ctx, projectID, stream, sb.Id)
		if err != nil {
			continue // Block doesn't exist yet — no conflict.
		}
		if existing.ContentHash != sb.ExpectedHash {
			return 0, nil, fmt.Errorf("conflict on block %s in %s: expected hash %s but current is %s",
				sb.Id, sb.ItemName, sb.ExpectedHash, existing.ContentHash)
		}
	}

	// Group blocks by item.
	itemGroups := map[string][]*model.Block{}
	for _, sb := range chunk.Blocks {
		b, err := bowsync.ProtoToBlock(sb)
		if err != nil {
			return 0, nil, fmt.Errorf("decode block %s in %s: %w", sb.Id, sb.ItemName, err)
		}
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
				if meta.Collection != "" {
					coll, err := deps.ContentStore.GetCollectionByName(ctx, projectID, meta.Collection, stream)
					if err == nil && coll != nil {
						item.CollectionID = coll.ID
					}
				}
			}
			_ = deps.ContentStore.StoreItem(ctx, projectID, stream, item)
			itemNames = append(itemNames, itemName)
		} else {
			if err := deps.ContentStore.StoreBlocks(ctx, projectID, stream, blocks); err != nil {
				return stored, itemNames, fmt.Errorf("store blocks: %w", err)
			}
		}
		stored += len(blocks)

		// Seed the project TM from any target translations these blocks arrived
		// with, so a future convergence recycles them for free (theme A2).
		seedTMFromBlockTargets(ctx, tm, blocks, projectID, sourceLocale)
	}

	return stored, itemNames, nil
}

// seedTMFromBlockTargets adds every (source, target) pair carried by the blocks
// to the project TM, across all target locales present. It is a thin fan-out
// over seedTMFromBlocks (one call per distinct target locale) and is a no-op
// when tm is nil or no block carries a populated target.
func seedTMFromBlockTargets(ctx context.Context, tm sievepen.TMStore, blocks []*model.Block, projectID string, sourceLocale model.LocaleID) {
	if tm == nil || sourceLocale.IsEmpty() {
		return
	}
	// Collect the distinct target locales carried by these blocks.
	locales := map[model.LocaleID]struct{}{}
	for _, b := range blocks {
		if b == nil {
			continue
		}
		for key := range b.Targets {
			if key.Locale != "" && key.Locale != sourceLocale {
				locales[key.Locale] = struct{}{}
			}
		}
	}
	for loc := range locales {
		seedTMFromBlocks(ctx, tm, blocks, projectID, sourceLocale, loc, "push", "")
	}
}
