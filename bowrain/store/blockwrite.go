package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

var _ platstore.BlockWriteStore = (*PostgresStore)(nil)

// itemBlocksSQL reads one item's blocks in document order. The key filter, when
// there is one, matches a block by its durable key, its name or its row id: the
// three a change set may address it by. %s is either nothing or the filter and,
// for a held read, FOR UPDATE.
const itemBlocksSQL = `SELECT b.id, b.project_id, b.item_name, b.source_id, b.name, b.type, b.mime_type, b.translatable,
		b.content_hash, b.context_hash, b.source_json, b.properties, b.overlays, b.stored_at, b.updated_at
	 FROM blocks b
	 WHERE b.project_id = $1 AND b.stream = $2 AND b.item_name = $3%s
	 ORDER BY ` + BlockListOrder + `%s`

// keyFilter is the clause that narrows an item read to the blocks keys names.
const keyFilter = ` AND (b.source_id = ANY($4) OR b.name = ANY($4) OR b.id = ANY($4))`

// ItemBlocks reads one item's blocks in document order. See
// store.BlockWriteStore.
func (s *PostgresStore) ItemBlocks(ctx context.Context, projectID, stream, itemName string, keys []string) ([]*venue.StoredBlock, error) {
	stream = storeutil.DefaultStream(stream)
	return readItemBlocks(ctx, s.db.DB, projectID, stream, itemName, keys, false)
}

// readItemBlocks reads an item's blocks on q, holding their rows when hold is
// set.
func readItemBlocks(ctx context.Context, q Querier, projectID, stream, itemName string, keys []string, hold bool) ([]*venue.StoredBlock, error) {
	filter, lock := "", ""
	args := []any{projectID, stream, itemName}
	if len(keys) > 0 {
		filter = keyFilter
		args = append(args, keys)
	}
	if hold {
		lock = " FOR UPDATE OF b"
	}
	rows, err := q.QueryContext(ctx, fmt.Sprintf(itemBlocksSQL, filter, lock), args...)
	if err != nil {
		return nil, fmt.Errorf("read the blocks of %s: %w", itemName, err)
	}
	defer rows.Close()
	var out []*venue.StoredBlock
	for rows.Next() {
		sb, err := scanStoredBlockPg(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the blocks of %s: %w", itemName, err)
	}
	if err := HydrateOverlays(ctx, q, "pg", projectID, stream, out); err != nil {
		return nil, err
	}
	return out, nil
}

// BeginBlockWrite starts a write to one stream. See store.BlockWriteStore.
func (s *PostgresStore) BeginBlockWrite(ctx context.Context, projectID, stream string) (platstore.BlockWrite, error) {
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	// A block write shares the stream's lock, taken before any row (see
	// lockStream).
	if err := lockStream(ctx, tx, projectID, stream, false); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return &pgBlockWrite{tx: tx, projectID: projectID, stream: stream, held: map[string]map[string]bool{}}, nil
}

// pgBlockWrite is a BlockWrite on one PostgreSQL transaction.
type pgBlockWrite struct {
	tx        *sql.Tx
	projectID string
	stream    string
	held      map[string]map[string]bool // row id → variants of its targets
	done      bool
}

// targetVariants is the set of variants a block holds a target in. A held
// row's set is what Store compares the stored block against, so a target the
// change removed loses its row.
func targetVariants(b *model.Block) map[string]bool {
	out := map[string]bool{}
	src := b.EditionKeyOf(model.EditionKey{})
	for key := range b.EachEdition {
		if key != src {
			out[VariantKeyText(key)] = true
		}
	}
	return out
}

func (w *pgBlockWrite) Hold(ctx context.Context, itemName string, keys []string) ([]*venue.StoredBlock, error) {
	rows, err := readItemBlocks(ctx, w.tx, w.projectID, w.stream, itemName, keys, true)
	if err != nil {
		return nil, err
	}
	for _, sb := range rows {
		w.held[sb.Block.ID] = targetVariants(sb.Block)
	}
	return rows, nil
}

func (w *pgBlockWrite) Store(ctx context.Context, blocks []*venue.StoredBlock) error {
	// A write-back: each block lands on the row Hold read, which still holds
	// the content hash it read because the row is held. It keeps the row's
	// stored context hash, which is the producer's (see WriteBackBlocks).
	wb, bs := storeutil.NewWriteBack(blocks)
	if len(bs) == 0 {
		return nil
	}
	if err := storeBlocksTx(ctx, w.tx, w.projectID, w.stream, "", bs, wb); err != nil {
		return err
	}
	if skipped := wb.Skipped(); len(skipped) > 0 {
		return fmt.Errorf("block %s is no longer the row the write holds", skipped[0])
	}
	// The write above upserts the targets each block carries. A target the
	// held row carried and the block does not was removed by the change, and
	// its row goes too.
	now := time.Now().UTC()
	for _, b := range bs {
		was, ok := w.held[b.ID]
		if !ok {
			continue
		}
		is := targetVariants(b)
		for variant := range was {
			if !is[variant] {
				if err := removeTargetTx(ctx, w.tx, w.projectID, w.stream, b.ID, variant, now); err != nil {
					return err
				}
			}
		}
		w.held[b.ID] = is
	}
	return nil
}

// removeTargetTx deletes one target of a block and records the removal in the
// block's history, attributed like any other change of the transaction, and in
// the stream's change log.
func removeTargetTx(ctx context.Context, tx Runner, projectID, stream, blockID, variant string, now time.Time) error {
	res, err := tx.ExecContext(ctx,
		`DELETE FROM translations WHERE project_id=$1 AND stream=$2 AND block_id=$3 AND locale=$4`,
		projectID, stream, blockID, variant)
	if err != nil {
		return fmt.Errorf("remove target %s of block %s: %w", variant, blockID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	cc := ChangeContextFromContext(ctx)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO block_history
			(project_id, stream, block_id, locale, change_type, text, coded_text, origin, author, actor_role, edit_reason, correlation_id, created_at)
		 VALUES ($1, $2, $3, $4, 'target_removed', '', '', '', $5, $6, $7, $8, $9)`,
		projectID, stream, blockID, variant, cc.Actor, cc.ActorRole, cc.Reason, cc.CorrelationID, now); err != nil {
		return fmt.Errorf("record the removal of target %s of block %s: %w", variant, blockID, err)
	}
	if err := logChange(ctx, tx, projectID, stream, blockID, "target_removed", variant, ""); err != nil {
		return fmt.Errorf("log the removal of target %s of block %s: %w", variant, blockID, err)
	}
	return nil
}

func (w *pgBlockWrite) Commit() error {
	if w.done {
		return nil
	}
	w.done = true
	if err := w.tx.Commit(); err != nil {
		return fmt.Errorf("commit block write: %w", err)
	}
	return nil
}

func (w *pgBlockWrite) Rollback() error {
	if w.done {
		return nil
	}
	w.done = true
	return w.tx.Rollback()
}
