package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/bowrain/analytics"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
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

	// Resolve the project content memory (optional) and source language once, so ingest can
	// seed pushed target translations into the content memory for future recycling (theme
	// A2). A missing content memory or source language simply disables seeding — never an
	// error on the push path.
	var seedMemory memory.Store
	var sourceLocale model.LocaleID
	if deps.MemoryResolver != nil {
		slug := manifest.WorkspaceSlug
		if slug == "" {
			slug = "_anon"
		}
		if tm, terr := deps.MemoryResolver.GetMemory(slug); terr == nil {
			seedMemory = tm
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

		// The manifest's hash is a promise about the bytes, not just the name of
		// a place to find them. Re-derive it: whatever the store returned is
		// accepted only if it is in fact the content that was pushed, so a
		// manifest cannot make the worker parse bytes that were never uploaded
		// under that digest.
		if sum := sha256.Sum256(chunkData); hex.EncodeToString(sum[:]) != chunkRef.Hash {
			_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed,
				fmt.Sprintf("chunk %d content does not match its hash", chunkRef.Index))
			return fmt.Errorf("chunk %d: content does not match hash %s", chunkRef.Index, chunkRef.Hash)
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
			stored, itemNames, err := processBlockChunk(ctx, deps, &chunk, projectID, stream, itemMetaMap, seedMemory, sourceLocale)
			if err != nil {
				_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, err.Error())
				return err
			}
			totalStored += stored
			allItemNames = append(allItemNames, itemNames...)

		case "terms", "memory", "media":
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
		// The full item list still rides on "items" because downstream
		// consumers (per-locale automations in server/automation.go) fan out by
		// item name. The human-facing activity summary, however, is built from
		// the structured counts below — never from the joined item string,
		// which for a large push is an unreadable wall of paths.
		deps.EventBus.Publish(platev.Event{
			Type:      platev.EventPushCompleted,
			Source:    "sync-worker",
			ProjectID: projectID,
			Actor:     manifest.ActorID,
			Data: map[string]string{
				"items":          strings.Join(allItemNames, ","),
				"items_sample":   strings.Join(sampleItemNames(allItemNames, 3), ","),
				"files_count":    strconv.Itoa(len(allItemNames)),
				"blocks_count":   strconv.Itoa(totalStored),
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

// sampleItemNames returns at most n item names, for a compact preview stored
// on the push activity (e.g. the first few files) without carrying the whole
// list into the human-facing summary.
func sampleItemNames(names []string, n int) []string {
	if len(names) <= n {
		return names
	}
	return names[:n]
}

// processBlockChunk converts SyncBlocks to model.Blocks and stores them.
// Blocks with ExpectedHash set are checked for optimistic concurrency conflicts.
//
// tm (optional) is the project's server content memory: when a stored block arrives with an
// existing target translation, that pair is seeded into the content memory so a future
// convergence recycles it instead of paying AI (theme A2, "arrives with
// translations → recycles for free"). Seeding is idempotent (content-hash keyed
// entry IDs) and best-effort — a seed failure never fails the push.
func processBlockChunk(ctx context.Context, deps *WorkerDeps, chunk *pb.SyncChunk, projectID, stream string, itemMetas map[string]*pb.SyncItemMeta, tm memory.Store, sourceLocale model.LocaleID) (int, []string, error) {
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

		// Seed the project content memory from any target translations these blocks arrived
		// with, so a future convergence recycles them for free (theme A2).
		seedMemoryFromBlockTargets(ctx, tm, blocks, projectID, sourceLocale)
	}

	return stored, itemNames, nil
}

// seedMemoryFromBlockTargets adds every (source, target) pair carried by the blocks
// to the project content memory, across all target locales present. It is a thin fan-out
// over seedMemoryFromBlocks (one call per distinct target locale) and is a no-op
// when tm is nil or no block carries a populated target.
func seedMemoryFromBlockTargets(ctx context.Context, tm memory.Store, blocks []*model.Block, projectID string, sourceLocale model.LocaleID) {
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
		seedMemoryFromBlocks(ctx, tm, blocks, projectID, sourceLocale, loc, "push", "")
	}
}
