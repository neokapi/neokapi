package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/bowrain/analytics"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
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
	// ExpectedRef is the compare-and-swap assertion the client sent: the
	// governance components it last observed. The commit handler checks it too,
	// so a waiting client is told at once — but the handler answers before the
	// work is queued, and this is where the write actually happens, so the
	// assertion is re-made here against the state it is about to change.
	ExpectedRef ref.Ref `json:"expected_ref"`
	// BlockPropertyKeys is what the producer declared its readers record. It
	// scopes deletion: a property key outside this set was written by someone
	// this push knows nothing about, and survives it. See keepDeclaredProperties.
	BlockPropertyKeys []string `json:"block_property_keys"`
	// Scope is the set of paths this push is authoritative over: the recipe's
	// globs (with its exclusions negated), or the resolved paths of a scoped
	// push. Within it the declared tree is exactly what the project holds;
	// outside it nothing is touched. A push declaring no scope removes no
	// item — an older producer makes no claim about what is missing.
	Scope venue.Scope `json:"scope"`

	// Tree is what the producer read: per item, the block keys it holds now and
	// the content hash of each, in document order.
	//
	// It carries both halves of what a payload cannot say. The keys say which
	// blocks an item still has, so the rest can go. The content hashes say what
	// a document IS, so a file that moved is recognised as the same document
	// and keeps the approvals hanging off it rather than being read as one file
	// deleted and another created.
	Tree venue.Tree `json:"tree"`
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
	if bs, canBranch := deps.ContentStore.(store.StreamBranchStore); canBranch && stream != "main" {
		if _, err := deps.ContentStore.GetStream(ctx, projectID, stream); err != nil {
			baseCursor, _ := deps.ContentStore.LatestCursor(ctx, projectID, "main")
			if cerr := bs.CreateStream(ctx, &store.Stream{
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

	// The context content type reconciles inside the transition below, and
	// before the chunks are stored within it: the collections are the structure
	// the items are stored into, so an item naming a collection that has not
	// been created yet would fall to the project's default collection and be
	// governed by it until the next push re-binds it. See the binding in
	// processBlockChunk.
	contextEntries, err := parseContextEntries(manifest.Contexts)
	if err != nil {
		markJobFailed(ctx, deps, job.ID, "invalid context entries")
		return err
	}
	workspaceID := ""
	if projectRow != nil {
		workspaceID = projectRow.WorkspaceID
	}
	if len(contextEntries) > 0 {
		if err := assertContextRef(ctx, deps, projectID, stream, manifest.ExpectedRef); err != nil {
			markJobFailed(ctx, deps, job.ID, err.Error())
			return err
		}
	}

	// Resolve the project content memory (optional) and source language once, so
	// the decisions this push carries can promote the wording they approve into
	// the memory once they commit. The push's own targets are not admitted: a
	// decision is the only door in. A missing content memory or source language
	// simply disables promotion — never an error on the push path.
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

	// 2. Stage every chunk. Nothing is written here: the chunks are
	// downloaded, verified against the hashes the manifest promised, decoded,
	// and checked against the state the push started from.
	staged, err := stagePush(ctx, deps, job, &manifest, projectID, stream, itemMetaMap)
	if err != nil {
		return err
	}

	// Which document each declared entry IS, resolved against what the project
	// holds now. It reads pre-push state alongside the staging checks, and the
	// transition acts on it.
	plan, err := planIdentity(ctx, deps, projectID, stream, manifest.Scope, manifest.Tree)
	if err != nil {
		markJobFailed(ctx, deps, job.ID, err.Error())
		return err
	}

	// A malformed decisions payload fails the push — decisions are authored
	// state, and dropping them silently is the exact failure shape this protocol
	// keeps having to confess. A payload that decodes to nothing, on the other
	// hand, is not a decisions write at all: everything downstream keys on the
	// decoded records rather than the raw bytes, which is what stops a JSON
	// `null` — four bytes of nothing — from asserting a component this push
	// never touches.
	var decisions []venue.UnitDecision
	if len(manifest.Decisions) > 0 {
		if err := json.Unmarshal(manifest.Decisions, &decisions); err != nil {
			markJobFailed(ctx, deps, job.ID, "invalid decisions payload")
			return fmt.Errorf("parse manifest decisions: %w", err)
		}
	}

	// The platform is authoritative for review governance. Everything the
	// gate needs is resolved here, before the transition opens: which rows
	// the payload names and the rung each holds, who last wrote each target by
	// hand, the workspace policy, and the pusher's review permission for each
	// language a verdict or a withdrawn sign-off names. A push that carries
	// neither resolves no permission at all.
	gov, gerr := newPushGovernor(ctx, deps, projectID, stream, workspaceID, manifest.ActorID, staged, decisions)
	if gerr != nil {
		markJobFailed(ctx, deps, job.ID, gerr.Error())
		return gerr
	}

	// 3. Apply the whole transition on one transaction. Blocks, items, the
	// prune and the decisions ledger land together or not at all; a worker that
	// dies partway leaves the project exactly as the push found it.
	applier, ok := deps.ContentStore.(store.PushApplyStore)
	if !ok {
		err := errors.New("sync: this content store cannot apply a push atomically")
		markJobFailed(ctx, deps, job.ID, err.Error())
		return err
	}
	var outcome *pushOutcome
	var contextResult *contextReconcileResult
	apply := func(tx store.PushApplier, voice coreprofile.Store) error {
		// Collections first, and on this transaction: an item naming a
		// collection that does not exist yet falls to the project's default
		// collection, and a reconcile that outlived a failed push would leave
		// the project governed by a declaration that never landed.
		var rerr error
		contextResult, rerr = reconcileContext(ctx, tx, voice,
			projectID, stream, workspaceID, manifest.ActorID, contextEntries)
		if rerr != nil {
			return rerr
		}
		var aerr error
		outcome, aerr = applyStagedPush(ctx, tx, deps, job, projectID, stream,
			staged, plan, manifest.Tree, decisions, manifest.ExpectedRef, gov)
		return aerr
	}

	if deps.PushTransition != nil {
		if err := deps.PushTransition(ctx, apply); err != nil {
			markJobFailed(ctx, deps, job.ID, err.Error())
			return err
		}
	} else if err := applier.ApplyPush(ctx, func(tx store.PushApplier) error {
		// No shared-pool transition: the voice store cannot join this
		// transaction, so it is written through the pool and a rollback leaves
		// it. Content and collections still land together.
		return apply(tx, deps.VoiceStore)
	}); err != nil {
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
	totalStored := outcome.Stored
	allItemNames := outcome.ItemNames
	if outcome.Decisions > 0 {
		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Recorded %d decision(s) in the ledger", outcome.Decisions), nil)
	}

	// What the review gate refused, said in every place it has to be said:
	// the venue's log, the audit trail, the push's own run log, and the job
	// row the producer reads back through the status endpoint.
	gov.logRefusals(ctx, projectID, stream)
	gov.emitAudit(ctx, deps, projectID, stream)
	gov.recordSoDViolations(deps, projectID)
	if summary := gov.summary(); summary != "" {
		emitLog(deps, job.StepID, "info", summary, nil)
	}
	recordPushGovernance(ctx, deps, job.ID, outcome.Governance)
	if outcome.Renamed > 0 {
		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Followed %d file(s) to a new path, keeping their translations and approvals", outcome.Renamed), nil)
	}
	if outcome.Removed > 0 {
		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Removed %d file(s) the source no longer has", outcome.Removed), nil)
	}

	// The corpus follows the decisions, once they have committed: approvals
	// promote their wording into the content memory, rejections evict it. The
	// content memory is a different store, so it cannot join the transition —
	// which is why it runs after it rather than inside it, and why the full
	// decision set is passed rather than only the changed records. Promotion is
	// idempotent, and a replay must be able to heal a memory that was reset
	// independently of the ledger.
	if len(decisions) > 0 {
		if promotedN, evictedN := PromoteDecisionsToMemory(ctx, deps.ContentStore, seedMemory,
			projectID, stream, sourceLocale, decisions); promotedN+evictedN > 0 {
			emitLog(deps, job.StepID, "info",
				fmt.Sprintf("Content memory followed the decisions: %d promoted, %d evicted", promotedN, evictedN), nil)
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
	// content is already stored, and the next push will try again. A
	// context-only push counts — it landed this project's collections on this
	// stream, which is exactly what the field names.
	if totalStored > 0 || contextResult.total() > 0 {
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

	// Reclaim the manifest and every chunk blob it referenced. Ingested content
	// now lives in the store, so the staging blobs are spent; without this sweep
	// they are never reclaimed (only the manifest was, before), and an
	// authenticated client grows blob storage without bound at 2 MiB/chunk. A
	// failed delete orphans a blob rather than losing content, so it warns
	// rather than failing the job. The bucket-lifecycle backstop (a TTL rule on
	// the staging prefix) lives in bowrain-infra.
	if err := deps.BlobStore.Delete(ctx, manifestKey); err != nil {
		slog.Warn("could not delete sync push manifest blob",
			"job_id", job.ID, "manifest_key", manifestKey, "error", err)
	}
	for _, chunkRef := range manifest.Chunks {
		if err := deps.BlobStore.Delete(ctx, chunkRef.Hash); err != nil {
			slog.Warn("could not delete sync push chunk blob",
				"job_id", job.ID, "chunk_hash", chunkRef.Hash, "error", err)
		}
	}

	// Publish EventPushCompleted.
	//
	// A push that lands only context — a recipe declaring a new collection,
	// moving a point, renaming a channel — stores no blocks, and gating the
	// event on blocks alone left every such push silent. The consumers that
	// care are exactly the ones a context change is FOR: the workspace context
	// graph, and the channel-slug equivalence the workspace observes across its
	// projects. Both then waited for a block-carrying push to come along.
	if (totalStored > 0 || contextResult.total() > 0) && deps.EventBus != nil {
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

// itemSourcePath is the file an item's content was lifted out of, when every
// block it holds agrees on one.
//
// Most items are their own source: a Markdown page's name is the page. The
// exception is a generated catalog. A KBF bundle extracted from `App.tsx` is
// stored as `…/App.kbf.json`, and that name is the item's identity — the
// recipe's glob claimed it, a push addresses it, the target file is keyed by it.
// It is the courier, not the letter, and a file list showing only couriers reads
// as a list of JSON.
//
// The letter is on the blocks. A reader that lifts content out of a source file
// records that file on every block it produces, so the bundle already knows what
// it is a bundle OF. Reading it here is what lets a surface *show* the item as
// its source without anything being renamed — and without fetching a whole
// document to learn one path.
//
// Disagreement records nothing. A bundle holding two source files has no single
// source, and naming it after whichever block sorted first would be a guess
// wearing the same field as a fact.
func itemSourcePath(blocks []*model.Block) string {
	src := ""
	for _, b := range blocks {
		if b == nil {
			continue
		}
		file := b.Properties["file"]
		if file == "" {
			continue
		}
		if src == "" {
			src = file
			continue
		}
		if src != file {
			return ""
		}
	}
	return src
}

// pruneDeclaredItems removes, from each item this push described, the blocks the
// source no longer has.
//
// It is the half of the protocol that lets a push say a string is GONE. What a
// push carries is what CHANGED, and a deleted paragraph or a removed `t()` call
// changes nothing it can send — it simply stops being sent. So the store, which
// upserts what arrives, kept the block for good: still counted in the item's
// totals, still listed in its content, still queued for review, still dragging
// the coverage a ship gate reads. The producer declares what each item it read
// now holds (core/venue.ItemBlockKeys); this is where the rest goes.
//
// Silence is not deletion — the rule keepUndeclaredProperties follows one level
// down. An item absent from the declaration was not read by this push (a scoped
// `kapi push <path>`, an older producer that sends no declaration at all) and is
// left entirely alone. An item present with an empty set IS an answer: its last
// translatable string was deleted, which is the case this exists for.
//
// A failure here fails the push. Inside one transition the prune is not a
// separate, lesser write it could be skipped in favour of: the blocks this push
// carried and the blocks it removed are one statement about what the source now
// holds, and landing half of it is what this design exists to stop. The client
// retries and declares the same thing again.
func pruneDeclaredItems(
	ctx context.Context,
	tx store.PushApplier,
	deps *WorkerDeps,
	job *TranslationJob,
	projectID, stream string,
	itemBlocks map[string][]string,
) (int, error) {
	if len(itemBlocks) == 0 {
		return 0, nil
	}
	names := make([]string, 0, len(itemBlocks))
	for itemName := range itemBlocks {
		if itemName != "" {
			names = append(names, itemName)
		}
	}
	sort.Strings(names)

	pruned, items := 0, 0
	for _, itemName := range names {
		n, err := tx.PruneItemBlocks(ctx, projectID, stream, itemName, itemBlocks[itemName])
		if err != nil {
			return 0, fmt.Errorf("prune item %s of the blocks its source no longer has: %w", itemName, err)
		}
		if n > 0 {
			pruned += n
			items++
		}
	}
	if pruned > 0 {
		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Removed %d block(s) across %d item(s) the source no longer has", pruned, items), nil)
	}
	return pruned, nil
}

// keepUndeclaredProperties carries forward the block properties a stored row
// holds that this push declared no knowledge of.
//
// A fleet is not one version. A CI runner pinned to an older kapi, or a laptop
// that has not upgraded, reads the same files with a reader that records less —
// and because the sync cache is not committed, a fresh checkout declares every
// item, so the older producer really does push the whole corpus. Storing what it
// sent as the whole truth would let it delete a field it has never heard of, and
// two vintages would take turns adding and removing it.
//
// Silence is not deletion; disagreement is. A push declares the keys its readers
// emit (core/venue.BlockPropertyKeys), computed over every block it read rather
// than the ones it sent — so a value genuinely removed at the source is removed
// here, while a key this producer never emits survives untouched. A push that
// declares nothing knows nothing: it adds and updates, and removes nothing.
//
// Derived locators are excluded. An advisory property says where a block was
// found, so preserving one from a producer that no longer records it leaves a
// locator pointing at nothing — worse than its absence.
//
// Best-effort: a push must not fail because the prior state could not be read.
func keepUndeclaredProperties(
	ctx context.Context,
	deps *WorkerDeps,
	projectID, stream, itemName string,
	arriving []*model.Block,
	declared []string,
) {
	authoritative := map[string]bool{}
	for _, key := range declared {
		authoritative[key] = true
	}
	// The keys the payload carries are authoritative whatever was declared: a
	// block that arrived with a property plainly knows about it.
	for _, b := range arriving {
		for key := range b.Properties {
			authoritative[key] = true
		}
	}

	storedRows, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: stream, ItemName: itemName,
	})
	if err != nil || len(storedRows) == 0 {
		return
	}
	// Joined both ways, like the conflict check above: a block that came out of
	// this store names its row directly, one read from a local file carries its
	// durable identity in the structural name the store filed as source_id.
	byRowID := map[string]*venue.StoredBlock{}
	byKey := map[string]*venue.StoredBlock{}
	for _, row := range storedRows {
		byRowID[row.ID] = row
		if row.SourceID != "" {
			byKey[row.SourceID] = row
		}
	}

	for _, b := range arriving {
		row, found := byRowID[b.ID]
		if !found && b.Name != "" {
			row, found = byKey[b.Name]
		}
		if !found || row.Block == nil {
			continue
		}
		for key, value := range row.Block.Properties {
			if authoritative[key] || model.IsAdvisoryProperty(key) {
				continue
			}
			if b.Properties == nil {
				b.Properties = map[string]string{}
			}
			b.Properties[key] = value
		}
	}
}
