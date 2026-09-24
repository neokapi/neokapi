package store

import (
	"context"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// Push application uses one transaction across content and voice stores.
// Chunks are downloaded, hash-verified and decoded before the transaction
// starts, limiting its duration to database work.
//
// Both stores share the worker's PgDB, so storage.PgDB.Transition covers
// collection reconciliation, voice updates, items, blocks, pruning and decisions.
// Any failure rolls back the entire push.
//
// Reconcile collections before storing items. An item referencing a missing
// collection would otherwise be assigned to the project's default collection.

var (
	_ platstore.PushApplyStore = (*PostgresStore)(nil)
	_ platstore.PushApplier    = pushApply{}
)

// pushApply is the PushApplier bound to one transaction. Every verb runs on
// that transaction, including the reads: a push decides an item's collection
// against items this same transition may have just stored, and asserts the
// decisions ledger against rows it is about to replace.
//
// Every verb that writes a stream's content (its items, blocks and decisions)
// first holds that stream's write lock exclusively (see lockStream), so the
// server's block writes on the stream take turns with the push. The collection
// verbs write no row a block write takes, and hold nothing.
type pushApply struct {
	s  *PostgresStore
	tx Runner
	// held is the set of stream write locks this transaction holds.
	held map[string]struct{}
}

func newPushApply(s *PostgresStore, tx Runner) pushApply {
	return pushApply{s: s, tx: tx, held: map[string]struct{}{}}
}

// hold takes a stream's write lock before the push's first write to it. The
// lock lasts until the transaction ends, so each stream's is taken once.
func (a pushApply) hold(ctx context.Context, projectID, stream string) error {
	key := streamLockKey(projectID, stream)
	if _, ok := a.held[key]; ok {
		return nil
	}
	if err := lockStream(ctx, a.tx, projectID, stream, true); err != nil {
		return err
	}
	a.held[key] = struct{}{}
	return nil
}

func (a pushApply) StoreItem(ctx context.Context, projectID, stream string, item *platstore.Item) error {
	if err := a.hold(ctx, projectID, stream); err != nil {
		return err
	}
	return storeItemTx(ctx, a.tx, projectID, stream, item)
}

func (a pushApply) StoreBlocks(ctx context.Context, projectID, stream string, blocks []*model.Block) error {
	if err := a.hold(ctx, projectID, stream); err != nil {
		return err
	}
	return storeBlocksTx(ctx, a.tx, projectID, stream, "", blocks, nil)
}

func (a pushApply) StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error {
	if err := a.hold(ctx, projectID, stream); err != nil {
		return err
	}
	return storeBlocksTx(ctx, a.tx, projectID, stream, itemName, blocks, nil)
}

func (a pushApply) SetBlockOrder(ctx context.Context, projectID, stream, itemName string, keys []string) error {
	if err := a.hold(ctx, projectID, stream); err != nil {
		return err
	}
	return setBlockOrderTx(ctx, a.tx, projectID, stream, itemName, keys)
}

func (a pushApply) PruneItemBlocks(ctx context.Context, projectID, stream, itemName string, keep []string) (int, error) {
	if err := a.hold(ctx, projectID, stream); err != nil {
		return 0, err
	}
	return pruneItemBlocksTx(ctx, a.tx, projectID, stream, itemName, keep)
}

func (a pushApply) UpsertUnitDecisions(ctx context.Context, projectID, stream string, decisions []venue.UnitDecision) (int, error) {
	if err := a.hold(ctx, projectID, stream); err != nil {
		return 0, err
	}
	return upsertUnitDecisionsTx(ctx, a.tx, projectID, stream, decisions)
}

func (a pushApply) ListUnitDecisions(ctx context.Context, projectID, stream string) ([]venue.UnitDecision, error) {
	return listUnitDecisionsTx(ctx, a.tx, projectID, stream)
}

func (a pushApply) RecordDraftBases(ctx context.Context, projectID, stream string, drafts []platstore.DraftBasis) error {
	if err := a.hold(ctx, projectID, stream); err != nil {
		return err
	}
	return recordDraftBasesTx(ctx, a.tx, projectID, stream, drafts)
}

func (a pushApply) ListCollections(ctx context.Context, projectID, stream string) ([]*platstore.Collection, error) {
	return a.s.listCollectionsTx(ctx, a.tx, projectID, stream)
}

func (a pushApply) CreateCollection(ctx context.Context, c *platstore.Collection) error {
	return a.s.createCollectionTx(ctx, a.tx, c)
}

func (a pushApply) UpdateCollection(ctx context.Context, c *platstore.Collection) error {
	return a.s.updateCollectionTx(ctx, a.tx, c)
}

func (a pushApply) RenameItem(ctx context.Context, projectID, stream, itemID, newName string) error {
	if err := a.hold(ctx, projectID, stream); err != nil {
		return err
	}
	return renameItemTx(ctx, a.tx, projectID, stream, itemID, newName)
}

func (a pushApply) DeleteItem(ctx context.Context, projectID, stream, itemName string) error {
	if err := a.hold(ctx, projectID, stream); err != nil {
		return err
	}
	return deleteItemTx(ctx, a.tx, projectID, stream, itemName)
}

func (a pushApply) GetItem(ctx context.Context, projectID, stream, itemName string) (*platstore.Item, error) {
	row := a.tx.QueryRowContext(ctx,
		`SELECT id, project_id, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at
		 FROM items WHERE project_id=$1 AND stream=$2 AND name=$3`,
		projectID, storeutil.DefaultStream(stream), itemName)
	return scanItemPg(row)
}

func (a pushApply) GetCollectionByName(ctx context.Context, projectID, name, stream string) (*platstore.Collection, error) {
	row := a.tx.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at
		 FROM collections WHERE project_id=$1 AND name=$2 AND (stream='' OR stream=$3)`,
		projectID, name, storeutil.DefaultStream(stream))
	return a.s.scanCollectionPg(row)
}

func (a pushApply) GetDefaultCollection(ctx context.Context, projectID string) (*platstore.Collection, error) {
	row := a.tx.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at
		 FROM collections WHERE project_id=$1 AND is_default=TRUE`, projectID)
	return a.s.scanCollectionPg(row)
}

// ApplyPush runs fn against one transaction and commits it. An error from fn
// rolls the whole transition back, so a push either lands or does not.
// Bind returns this store's push surface bound to tx, for a caller that owns
// the transaction and is putting more than one store in it.
func (s *PostgresStore) Bind(tx storage.Runner) platstore.PushApplier {
	return newPushApply(s, tx)
}

func (s *PostgresStore) ApplyPush(ctx context.Context, fn func(platstore.PushApplier) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin push: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(newPushApply(s, tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit push: %w", err)
	}
	return nil
}
