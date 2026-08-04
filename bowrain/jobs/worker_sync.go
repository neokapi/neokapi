package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	// Contexts is the context content type: the collections the recipe
	// declares. It rides in the manifest so the structure lands in the same
	// transaction as the items stored into it (see worker_context.go).
	Contexts json.RawMessage `json:"contexts"`
	// Decisions is the decisions content type: the client's committed decision
	// record (core/state), upserted idempotently into the unit_decisions
	// ledger after the chunks are stored — so decisions arriving with the
	// content they judge can resolve their rows and project their status.
	Decisions json.RawMessage `json:"decisions"`
}

type syncChunkRef struct {
	Index       int    `json:"index"`
	ContentType string `json:"content_type"`
	Hash        string `json:"hash"`
	RecordCount int    `json:"record_count"`
	ByteSize    int64  `json:"byte_size"`
}

// markJobFailed records why a sync job failed, on the way to returning the
// error that actually stops it.
//
// The status write is genuinely best-effort — the caller is already returning
// the real error, and there is nothing useful to do if the store is also
// unreachable — but "best-effort" is not "unobserved". A job left in-flight
// because this write failed looks, to anyone reading the job table, exactly
// like a job that is still running, so the failure is logged rather than
// discarded.
func markJobFailed(ctx context.Context, deps *WorkerDeps, jobID, reason string) {
	if err := deps.JobStore.UpdateJobStatus(ctx, jobID, StatusFailed, reason); err != nil {
		slog.Error("could not record sync job failure", "job_id", jobID, "reason", reason, "error", err)
	}
}

// processSyncPushJob handles the sync protocol push (Bowrain AD-009).
// It reads the manifest, downloads chunks, deserializes protobuf,
// and stores content via the full model.
func processSyncPushJob(ctx context.Context, deps *WorkerDeps, job *TranslationJob) error {
	manifestKey := job.Model
	projectID := job.ProjectID
	pushID := job.PushID

	if deps.BlobStore == nil {
		markJobFailed(ctx, deps, job.ID, "blob store not configured")
		return errors.New("blob store not configured")
	}

	emitLog(deps, job.StepID, "info", "Processing sync push",
		map[string]string{"project": projectID, "push_id": pushID})

	// 1. Download and parse manifest.
	reader, err := deps.BlobStore.Download(ctx, manifestKey)
	if err != nil {
		markJobFailed(ctx, deps, job.ID, "manifest download failed")
		return fmt.Errorf("download manifest: %w", err)
	}
	manifestData, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		markJobFailed(ctx, deps, job.ID, "manifest read failed")
		return fmt.Errorf("read manifest: %w", err)
	}

	var manifest syncPushManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		markJobFailed(ctx, deps, job.ID, "invalid manifest")
		return fmt.Errorf("parse manifest: %w", err)
	}

	stream := manifest.Stream
	if stream == "" {
		stream = "main"
	}

	// Auto-create non-main streams. A stream that already exists is the common
	// case here — GetStream failing does not distinguish "absent" from
	// "unreachable" — so a create that loses that race is not an error. One
	// that fails for any other reason leaves the chunks below writing into a
	// stream that does not exist, which is worth a line in the log.
	if stream != "main" {
		if _, err := deps.ContentStore.GetStream(ctx, projectID, stream); err != nil {
			baseCursor, _ := deps.ContentStore.LatestCursor(ctx, projectID, "main")
			if cerr := deps.ContentStore.CreateStream(ctx, &store.Stream{
				ProjectID:  projectID,
				Name:       stream,
				Parent:     "main",
				BaseCursor: baseCursor,
				Visibility: store.StreamPublic,
			}); cerr != nil {
				slog.Warn("could not auto-create stream for sync push",
					"project_id", projectID, "stream", stream, "error", cerr)
			}
		}
	}

	// Parse item metadata. This carries each item's format, block index,
	// preview and collection, so parsing it as far as the first bad byte and
	// carrying on would store the items with default metadata and nothing to
	// say the rest was ever there. Fail the push instead.
	var itemMetas []pb.SyncItemMeta
	if len(manifest.Items) > 0 {
		if err := json.Unmarshal(manifest.Items, &itemMetas); err != nil {
			markJobFailed(ctx, deps, job.ID, "invalid item metadata")
			return fmt.Errorf("parse manifest item metadata: %w", err)
		}
	}
	itemMetaMap := map[string]*pb.SyncItemMeta{}
	for i := range itemMetas {
		itemMetaMap[itemMetas[i].Name] = &itemMetas[i]
	}

	// Read the project once: its source language seeds the content memory below,
	// and its workspace is the brand hub the context reconcile binds voices in.
	// A project that cannot be read leaves both unresolved, which degrades each
	// to its own no-op rather than failing a push whose content is fine.
	var projectRow *store.Project
	if p, perr := deps.ContentStore.GetProject(ctx, projectID); perr == nil {
		projectRow = p
	}

	// The context content type reconciles BEFORE the chunks are stored: the
	// collections are the structure the items are stored into, so an item
	// naming a collection that has not been created yet would be stored
	// ungrouped and stay that way until the next push.
	contextEntries, err := parseContextEntries(manifest.Contexts)
	if err != nil {
		markJobFailed(ctx, deps, job.ID, "invalid context entries")
		return err
	}
	workspaceID := ""
	if projectRow != nil {
		workspaceID = projectRow.WorkspaceID
	}
	contextResult, err := reconcileContext(ctx, deps, projectID, stream, workspaceID, manifest.ActorID, contextEntries)
	if err != nil {
		markJobFailed(ctx, deps, job.ID, err.Error())
		return err
	}
	if contextResult.total() > 0 {
		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Reconciled %d collection(s): %d created, %d updated, %d claimed, %d unchanged",
				contextResult.total(), len(contextResult.Created), len(contextResult.Updated),
				len(contextResult.Claimed), len(contextResult.Unchanged)),
			nil)
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
		if projectRow != nil {
			sourceLocale = projectRow.DefaultSourceLanguage
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
			markJobFailed(ctx, deps, job.ID,
				fmt.Sprintf("chunk %d download failed: %s", chunkRef.Index, err.Error()))
			return fmt.Errorf("download chunk %d: %w", chunkRef.Index, err)
		}
		chunkData, err := io.ReadAll(chunkReader)
		chunkReader.Close()
		if err != nil {
			markJobFailed(ctx, deps, job.ID, "chunk read failed")
			return fmt.Errorf("read chunk %d: %w", chunkRef.Index, err)
		}

		// The manifest's hash is a promise about the bytes, not just the name of
		// a place to find them. Re-derive it: whatever the store returned is
		// accepted only if it is in fact the content that was pushed, so a
		// manifest cannot make the worker parse bytes that were never uploaded
		// under that digest.
		if sum := sha256.Sum256(chunkData); hex.EncodeToString(sum[:]) != chunkRef.Hash {
			markJobFailed(ctx, deps, job.ID,
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
			markJobFailed(ctx, deps, job.ID, "invalid chunk data")
			return fmt.Errorf("unmarshal chunk %d: %w", chunkRef.Index, err)
		}

		// Route by content type.
		switch chunk.ContentType {
		case "blocks":
			stored, itemNames, err := processBlockChunk(ctx, deps, &chunk, projectID, stream, itemMetaMap, seedMemory, sourceLocale)
			if err != nil {
				markJobFailed(ctx, deps, job.ID, err.Error())
				return err
			}
			totalStored += stored
			allItemNames = append(allItemNames, itemNames...)

		case "terms", "memory", "media":
			// These content types are not yet implemented. Rather than silently
			// dropping the payload and marking the job Completed (which would lose
			// data for any client that emits them), fail the job explicitly.
			err := fmt.Errorf("sync: content type %q (chunk %d) is not yet supported", chunk.ContentType, chunkRef.Index)
			markJobFailed(ctx, deps, job.ID, err.Error())
			return err

		default:
			err := fmt.Errorf("sync: unknown content type %q (chunk %d)", chunk.ContentType, chunkRef.Index)
			markJobFailed(ctx, deps, job.ID, err.Error())
			return err
		}
	}

	// The decisions content type ingests AFTER the chunks, so decisions that
	// arrived with the content they judge can resolve their rows and project
	// their status. A malformed payload fails the push — decisions are
	// authored state, and dropping them silently is the exact failure shape
	// this protocol keeps having to confess. A store without the ledger
	// capability skips with a log line rather than failing content it can
	// otherwise apply.
	if len(manifest.Decisions) > 0 {
		var decisions []store.UnitDecision
		if err := json.Unmarshal(manifest.Decisions, &decisions); err != nil {
			markJobFailed(ctx, deps, job.ID, "invalid decisions payload")
			return fmt.Errorf("parse manifest decisions: %w", err)
		}
		if ds, ok := deps.ContentStore.(store.DecisionStore); ok {
			applied, derr := ds.UpsertUnitDecisions(ctx, projectID, stream, decisions)
			if derr != nil {
				markJobFailed(ctx, deps, job.ID, derr.Error())
				return derr
			}
			if applied > 0 {
				emitLog(deps, job.StepID, "info",
					fmt.Sprintf("Recorded %d decision(s) in the ledger", applied), nil)
			}
		} else {
			slog.Warn("decisions arrived but the content store keeps no ledger; skipped",
				"project_id", projectID, "decisions", len(decisions))
		}
	}

	// The stored content just diverged from whatever the diff cache holds, so
	// drop the project's cached hashes before the job reads as done. Skipping
	// this is not a staleness nicety: the next push inside the cache TTL is
	// diffed against pre-apply hashes, concludes its changed blocks are already
	// on the server, and silently uploads nothing.
	if totalStored > 0 && deps.SyncCache != nil {
		deps.SyncCache.InvalidateProject(ctx, projectID)
	}

	// Auto-set project default stream. Convenience, not correctness: the
	// content is already stored, and the next push will try again.
	if totalStored > 0 {
		proj, projErr := deps.ContentStore.GetProject(ctx, projectID)
		if projErr == nil && proj.DefaultStream == "" {
			proj.DefaultStream = stream
			if err := deps.ContentStore.UpdateProject(ctx, proj); err != nil {
				slog.Warn("could not set project default stream",
					"project_id", projectID, "stream", stream, "error", err)
			}
		}
	}

	// Mark completed. The content is stored either way, but a job whose
	// completion never lands stays in-flight forever and is indistinguishable
	// from one still running — so this one the caller does hear about.
	if err := deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusCompleted, ""); err != nil {
		return fmt.Errorf("mark sync push job completed: %w", err)
	}

	// Clean up the manifest. Failing to delete it orphans a blob rather than
	// losing content, so it does not fail the job — but unobserved orphans
	// accumulate, and nobody goes looking for a blob they were never told about.
	if err := deps.BlobStore.Delete(ctx, manifestKey); err != nil {
		slog.Warn("could not delete sync push manifest blob",
			"job_id", job.ID, "manifest_key", manifestKey, "error", err)
	}

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
				"stream":       stream,
				"items":        strings.Join(allItemNames, ","),
				"items_sample": strings.Join(sampleItemNames(allItemNames, 3), ","),
				"files_count":  strconv.Itoa(len(allItemNames)),
				"blocks_count": strconv.Itoa(totalStored),
				"push_id":      pushID,
				// Collections the recipe stopped declaring. They ride on the
				// event rather than being acted on, which is the whole of the
				// server's answer to a removed collection: say so, keep the
				// content.
				"undeclared_collections": strings.Join(contextResult.Undeclared, ","),
				"workspace_slug":         manifest.WorkspaceSlug,
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
	// Check expected_hash conflict detection (optimistic concurrency). A block
	// that came out of this store names its row directly (sb.Id is the stored
	// row id — what a pull handed the client); one read from a local file
	// carries its durable identity in sb.Name (source_id, the structural name).
	// Both joins are checked: matching only the row id went permanently silent
	// when the store moved to durable keying, and matching only the durable key
	// would orphan every hash a pull-based editor holds. One stored-block load
	// per item.
	type expectedRef struct{ byRowID, byKey, hash string }
	conflictItems := map[string][]expectedRef{}
	for _, sb := range chunk.Blocks {
		if sb.ExpectedHash == "" {
			continue
		}
		conflictItems[sb.ItemName] = append(conflictItems[sb.ItemName],
			expectedRef{byRowID: sb.Id, byKey: sb.Name, hash: sb.ExpectedHash})
	}
	for itemName, expected := range conflictItems {
		storedRows, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
			ProjectID: projectID, Stream: stream, ItemName: itemName,
		})
		if err != nil {
			continue // Item doesn't exist yet — no conflict.
		}
		byRowID := map[string]string{}
		byKey := map[string]string{}
		for _, row := range storedRows {
			byRowID[row.ID] = row.ContentHash
			if row.SourceID != "" {
				byKey[row.SourceID] = row.ContentHash
			}
		}
		for _, exp := range expected {
			current, found := byRowID[exp.byRowID]
			if !found && exp.byKey != "" {
				current, found = byKey[exp.byKey]
			}
			if !found {
				continue // Block doesn't exist yet — no conflict.
			}
			if current != exp.hash {
				return 0, nil, fmt.Errorf("conflict on block %s in %s: expected hash %s but current is %s",
					exp.byRowID, itemName, exp.hash, current)
			}
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
			if err := deps.ContentStore.StoreItem(ctx, projectID, stream, item); err != nil {
				return stored, itemNames, fmt.Errorf("store item %s: %w", itemName, err)
			}
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
