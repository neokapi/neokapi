package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"sort"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
	"google.golang.org/protobuf/proto"
)

// A push lands in two movements: staging, which reads and decides nothing is
// written; and applying, which writes and reads only what it has written.
//
// Staging downloads every chunk, verifies it against the hash the manifest
// promised, decodes it, checks the optimistic-concurrency hashes and merges
// forward the block properties this producer does not declare. All of that is
// a read of the state the push started from — which is the state those checks
// mean, so making them all against one snapshot is more correct than the
// chunk-at-a-time version, where a later chunk's conflict check could see an
// earlier chunk's writes and compare a client's expectation against content
// the same push had just produced.
//
// Applying then runs on one transaction (store.PushApplyStore), so the whole
// transition lands or none of it does.

// stagedGroup is one item's blocks, decoded and checked, waiting to be written.
// An empty ItemName is the item-less group: blocks that belong to no file, which
// the desktop editor writes when it saves a target.
type stagedGroup struct {
	ItemName string
	Blocks   []*model.Block
	Meta     *pb.SyncItemMeta
}

// stagePush turns the manifest's chunks into groups ready to write, or fails
// having written nothing. Every failure marks the job failed with the reason,
// because the caller cannot say more than this function already knows.
func stagePush(
	ctx context.Context,
	deps *WorkerDeps,
	job *TranslationJob,
	manifest *syncPushManifest,
	projectID, stream string,
	itemMetas map[string]*pb.SyncItemMeta,
) ([]stagedGroup, error) {
	var staged []stagedGroup

	for _, chunkRef := range manifest.Chunks {
		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Reading chunk %d (%s, %d records)", chunkRef.Index, chunkRef.ContentType, chunkRef.RecordCount),
			nil)

		chunkReader, err := deps.BlobStore.Download(ctx, chunkRef.Hash)
		if err != nil {
			markJobFailed(ctx, deps, job.ID,
				fmt.Sprintf("chunk %d download failed: %s", chunkRef.Index, err.Error()))
			return nil, fmt.Errorf("download chunk %d: %w", chunkRef.Index, err)
		}
		chunkData, err := io.ReadAll(chunkReader)
		chunkReader.Close()
		if err != nil {
			markJobFailed(ctx, deps, job.ID, "chunk read failed")
			return nil, fmt.Errorf("read chunk %d: %w", chunkRef.Index, err)
		}

		// The manifest's hash is a promise about the bytes, not just the name of
		// a place to find them. Re-derive it: whatever the store returned is
		// accepted only if it is in fact the content that was pushed, so a
		// manifest cannot make the worker parse bytes that were never uploaded
		// under that digest.
		if sum := sha256.Sum256(chunkData); hex.EncodeToString(sum[:]) != chunkRef.Hash {
			markJobFailed(ctx, deps, job.ID,
				fmt.Sprintf("chunk %d content does not match its hash", chunkRef.Index))
			return nil, fmt.Errorf("chunk %d: content does not match hash %s", chunkRef.Index, chunkRef.Hash)
		}

		// Attempt zstd decompression (compressed chunks start with zstd magic bytes).
		if deps.Decompressor != nil && len(chunkData) > 4 {
			if decompressed, derr := deps.Decompressor.Decompress(chunkData); derr == nil {
				chunkData = decompressed
			}
			// If decompression fails, assume uncompressed data and continue.
		}

		var chunk pb.SyncChunk
		if err := proto.Unmarshal(chunkData, &chunk); err != nil {
			markJobFailed(ctx, deps, job.ID, "invalid chunk data")
			return nil, fmt.Errorf("unmarshal chunk %d: %w", chunkRef.Index, err)
		}

		switch chunk.ContentType {
		case "blocks":
			groups, err := stageBlockChunk(ctx, deps, &chunk, projectID, stream, itemMetas, manifest.BlockPropertyKeys)
			if err != nil {
				markJobFailed(ctx, deps, job.ID, err.Error())
				return nil, err
			}
			staged = append(staged, groups...)

		case "terms", "memory", "media":
			// These content types are not yet implemented. Rather than silently
			// dropping the payload and marking the job Completed (which would lose
			// data for any client that emits them), fail the job explicitly.
			err := fmt.Errorf("sync: content type %q (chunk %d) is not yet supported", chunk.ContentType, chunkRef.Index)
			markJobFailed(ctx, deps, job.ID, err.Error())
			return nil, err

		default:
			err := fmt.Errorf("sync: unknown content type %q (chunk %d)", chunk.ContentType, chunkRef.Index)
			markJobFailed(ctx, deps, job.ID, err.Error())
			return nil, err
		}
	}

	return staged, nil
}

// stageBlockChunk decodes one blocks chunk into per-item groups, having checked
// every expectation the payload carries about the state it was built against.
func stageBlockChunk(
	ctx context.Context,
	deps *WorkerDeps,
	chunk *pb.SyncChunk,
	projectID, stream string,
	itemMetas map[string]*pb.SyncItemMeta,
	declaredProperties []string,
) ([]stagedGroup, error) {
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
				return nil, fmt.Errorf("conflict on block %s in %s: expected hash %s but current is %s",
					exp.byRowID, itemName, exp.hash, current)
			}
		}
	}

	// Group blocks by item.
	itemGroups := map[string][]*model.Block{}
	for _, sb := range chunk.Blocks {
		b, err := venue.ProtoToBlock(sb)
		if err != nil {
			return nil, fmt.Errorf("decode block %s in %s: %w", sb.Id, sb.ItemName, err)
		}
		itemGroups[sb.ItemName] = append(itemGroups[sb.ItemName], b)
	}

	// Sorted, so the order blocks are written in is the same on every run. Map
	// iteration order is not a detail here: it decides the order of rows in the
	// change log a person reads back.
	names := make([]string, 0, len(itemGroups))
	for name := range itemGroups {
		names = append(names, name)
	}
	sort.Strings(names)

	groups := make([]stagedGroup, 0, len(names))
	for _, itemName := range names {
		blocks := itemGroups[itemName]
		if itemName != "" {
			keepUndeclaredProperties(ctx, deps, projectID, stream, itemName, blocks, declaredProperties)
		}
		groups = append(groups, stagedGroup{
			ItemName: itemName,
			Blocks:   blocks,
			Meta:     itemMetas[itemName],
		})
	}
	return groups, nil
}

// applyStagedPush writes one push's whole transition. Every call on `tx` lands
// in the same transaction, so an error anywhere leaves the project exactly as
// the push found it.
func applyStagedPush(
	ctx context.Context,
	tx store.PushApplier,
	deps *WorkerDeps,
	job *TranslationJob,
	projectID, stream string,
	staged []stagedGroup,
	plan *identityPlan,
	declared venue.Tree,
	decisions []venue.UnitDecision,
	expected ref.Ref,
	gov *pushGovernor,
) (*pushOutcome, error) {
	out := &pushOutcome{}

	// Review governance, before anything is written. Every rung above
	// translated this payload claims, and every sign-off it takes back, is put
	// to the platform's own gate, and what fails it is settled here rather
	// than stored and undone later. The content is untouched: a push moves
	// content, and only the verdict on it is the platform's to withhold or to
	// keep.
	gov.vetTargets(staged)

	// Identity first, before any content lands. A file that moved keeps the
	// item that carries its approvals, so its blocks are written to that item
	// rather than minting a second one at the new path.
	if err := plan.applyRenames(ctx, tx, projectID, stream); err != nil {
		return nil, err
	}
	out.Renamed = len(plan.Renames)

	for _, g := range staged {
		if g.ItemName == "" {
			if err := tx.StoreBlocks(ctx, projectID, stream, g.Blocks); err != nil {
				return nil, fmt.Errorf("store blocks: %w", err)
			}
			out.Stored += len(g.Blocks)
			continue
		}

		if err := tx.StoreBlocksForItem(ctx, projectID, stream, g.ItemName, g.Blocks); err != nil {
			return nil, fmt.Errorf("store blocks for %s: %w", g.ItemName, err)
		}
		item := stagedItemRow(ctx, tx, projectID, stream, g)
		if err := tx.StoreItem(ctx, projectID, stream, item); err != nil {
			return nil, fmt.Errorf("store item %s: %w", g.ItemName, err)
		}
		out.Stored += len(g.Blocks)
		out.ItemNames = append(out.ItemNames, g.ItemName)

		// Pushed targets do NOT seed the content memory. Transport moves
		// content; it does not approve it. The corpus has one door in —
		// wording a decision approved (PromoteDecisionsToMemory) — so nothing
		// a push carried can be offered back as approved wording merely for
		// having traveled.
	}

	// Removals land after every chunk's blocks, and inside the same
	// transaction. A push is chunked, so no single chunk knows an item's whole
	// content: pruning against one would delete everything the others carried.
	if _, err := pruneDeclaredItems(ctx, tx, deps, job, projectID, stream, declared.BlockKeys()); err != nil {
		return nil, err
	}

	// And then the items the declaration does not mention at all. Last, so an
	// item this push emptied is removed for what the declaration says rather
	// than for the order two statements ran in.
	removed, err := plan.applyRemovals(ctx, tx, projectID, stream)
	if err != nil {
		return nil, err
	}
	out.Removed = removed

	// The decisions ledger settles last, so decisions that arrived with the
	// content they judge can resolve their rows and project their status. The
	// expected-ref assertion reads through the same transaction as the write it
	// guards, which is what makes it a compare-and-swap rather than a look
	// followed by a hope.
	if len(decisions) > 0 {
		// One read of the ledger answers both questions asked of it: whether
		// the governance this push asserted still stands, and which of the
		// verdicts it carries the platform already holds.
		var held []venue.UnitDecision
		if expected.Decisions != "" || gov.judging() {
			var herr error
			if held, herr = tx.ListUnitDecisions(ctx, projectID, stream); herr != nil {
				return nil, fmt.Errorf("read the decision ledger: %w", herr)
			}
		}
		if err := assertDecisionsHeld(expected, held); err != nil {
			return nil, err
		}
		// A verdict the pusher may not make is kept as the basis it carries
		// and nothing more, so the venue records that the translation exists
		// without recording an approval nobody was entitled to give.
		decisions = gov.vetDecisions(held, decisions)
		applied, derr := tx.UpsertUnitDecisions(ctx, projectID, stream, decisions)
		if derr != nil {
			return nil, derr
		}
		out.Decisions = applied
	}

	// A permission this venue could not resolve is not a refusal. Rolling the
	// transition back leaves the producer holding the content and the
	// verdicts, to send again once the venue can answer; landing it would
	// discard an approval the pusher may well have been entitled to make.
	if err := gov.err(); err != nil {
		return nil, err
	}
	out.Governance = gov.report()
	return out, nil
}

// pushOutcome is what a landed transition did, for the logs, events and
// analytics that run after it committed.
type pushOutcome struct {
	Stored    int
	ItemNames []string
	Removed   int
	Renamed   int
	Decisions int

	// Governance is what the platform's review gate did not accept: the
	// verdicts this push carried that the pusher was not entitled to make.
	// It travels back to the producer, which is the only way `kapi push` can
	// say what landed and what did not.
	Governance venue.PushGovernance
}

// stagedItemRow is the item row a group writes, with its collection binding
// resolved against this transition's own writes.
func stagedItemRow(ctx context.Context, tx store.PushApplier, projectID, stream string, g stagedGroup) *store.Item {
	item := &store.Item{
		Name:     g.ItemName,
		Format:   "json", // default
		ItemType: "file",
	}
	if src := itemSourcePath(g.Blocks); src != "" {
		item.Properties = map[string]string{store.ItemPropSourcePath: src}
	}
	if g.Meta == nil {
		return item
	}
	if g.Meta.Format != "" {
		item.Format = g.Meta.Format
	}
	item.BlockIndex = g.Meta.BlockIndexJson
	item.PreviewHTML = g.Meta.PreviewHtml
	if g.Meta.Collection == "" {
		return item
	}

	// The collection this item belongs to was reconciled before the chunks
	// were stored, precisely so this lookup finds it. When it does not, the
	// item does NOT stay outside every collection: an item stored with no
	// collection id falls to the project's default collection, which every
	// project created through the API has (#1786). So the named collection's
	// coordinates, voice and gates do not apply — and the default collection's
	// bindings do, to content that was never authored under them.
	//
	// The fallback is resolved here rather than left to the store, so the
	// warning names the collection the item actually landed in instead of
	// describing an outcome the store does not produce. Only a project with no
	// default collection leaves the item genuinely ungrouped.
	//
	// The fallback applies to an item with no binding yet. An item this push
	// already found filed under a collection keeps that binding: a lookup that
	// failed once is not a decision to regroup content, and a re-push that
	// resolved the default here would move the item into it (#1840).
	coll, err := tx.GetCollectionByName(ctx, projectID, g.Meta.Collection, stream)
	if err == nil && coll != nil {
		item.CollectionID = coll.ID
		return item
	}

	existing, exErr := tx.GetItem(ctx, projectID, stream, g.ItemName)
	if exErr == nil && existing != nil && existing.CollectionID != "" {
		item.CollectionID = existing.CollectionID
		slog.Warn("pushed item could not be bound to the collection its meta names; it keeps the collection it is already filed under, so that collection's governance goes on applying to it and the named one's does not",
			"project_id", projectID, "stream", stream,
			"item", g.ItemName, "collection", g.Meta.Collection,
			"kept_collection_id", existing.CollectionID, "error", err)
		return item
	}

	fallback, fbErr := tx.GetDefaultCollection(ctx, projectID)
	if fbErr == nil && fallback != nil {
		item.CollectionID = fallback.ID
		slog.Warn("pushed item could not be bound to its collection; it falls to the project's default collection, so the named collection's governance does not apply to it and the default collection's does",
			"project_id", projectID, "stream", stream,
			"item", g.ItemName, "collection", g.Meta.Collection,
			"default_collection", fallback.Name, "error", err)
		return item
	}

	slog.Warn("pushed item could not be bound to its collection and the project has no default collection; it is stored ungrouped, so no collection's governance applies to it",
		"project_id", projectID, "stream", stream,
		"item", g.ItemName, "collection", g.Meta.Collection,
		"error", err, "default_collection_error", fbErr)
	return item
}
