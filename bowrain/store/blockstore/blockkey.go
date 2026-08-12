package blockstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
)

// The store files every overlay row under the block's own `blocks.id`: that is
// where the block round-trip writes a target and where hydration, the review
// queue, the decision ledger and coverage read it back. The blockstore.Store
// API hands its callers a different name for the same row — Block.Hash, the
// content key — and the server's own automation actions carry the row id. One
// resolution function, used by every overlay read and write, keeps the caller's
// name and the store's in step; without it a target written through this API
// lands on a row nothing joins to and the block comes back untranslated.
//
// A key that names no block in this store is refused rather than stored. An
// overlay is a layer ON a block, and a row nothing can ever join to is the
// silent-loss shape of #1846: the write succeeds, the read finds nothing, and
// the absence is indistinguishable from work not yet done.

// resolveBlockRow maps an overlay key onto the `blocks.id` its row is filed
// under. The row id wins over the content key, because naming a row directly is
// the stronger claim; content that several blocks share resolves to the lowest
// id, deterministically, so a write and the read that follows it always agree.
func (s *session) resolveBlockRow(key string) (string, error) {
	if key == "" {
		return "", errors.New("bowrain/blockstore: overlay key is empty")
	}
	if id, ok := s.cachedRow(key); ok {
		return id, nil
	}
	var (
		query string
		args  []any
	)
	if s.opts.Dialect == SQLiteDialect {
		query = `SELECT id FROM blocks WHERE project_id = ? AND (id = ? OR content_hash = ?)
			ORDER BY CASE WHEN id = ? THEN 0 ELSE 1 END, id LIMIT 1`
		args = []any{s.opts.ProjectID, key, key, key}
	} else {
		query = `SELECT id FROM blocks WHERE project_id = $1 AND (id = $2 OR content_hash = $2)
			ORDER BY CASE WHEN id = $2 THEN 0 ELSE 1 END, id LIMIT 1`
		args = []any{s.opts.ProjectID, key}
	}
	var id string
	if err := s.opts.DB.QueryRowContext(s.ctx, query, args...).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("bowrain/blockstore: overlay key %q names no block in project %s: %w",
				key, s.opts.ProjectID, blockstore.ErrNotFound)
		}
		return "", fmt.Errorf("bowrain/blockstore: resolve overlay key %q: %w", key, err)
	}
	s.cacheRow(key, id)
	return id, nil
}

// cachedRow and cacheRow guard the memo. The Store advertises Concurrent, so
// two goroutines writing overlays through one session must not meet in the map.
func (s *session) cachedRow(key string) (string, bool) {
	s.rowMu.Lock()
	defer s.rowMu.Unlock()
	id, ok := s.rowIDs[key]
	return id, ok
}

func (s *session) cacheRow(key, id string) {
	s.rowMu.Lock()
	defer s.rowMu.Unlock()
	if s.rowIDs == nil {
		s.rowIDs = map[string]string{}
	}
	s.rowIDs[key] = id
}

// blockKeysByRow returns the content key each row id is addressed by through
// the Store API, so a listed overlay carries the same BlockHash the block
// iterator reports for its block. A row whose block carries no content key maps
// to nothing and the listing falls back to the row id.
func (s *session) blockKeysByRow(rowIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(rowIDs))
	chunk := 5000
	if s.opts.Dialect == SQLiteDialect {
		chunk = 500
	}
	for start := 0; start < len(rowIDs); start += chunk {
		batch := rowIDs[start:min(start+chunk, len(rowIDs))]
		args := make([]any, 0, len(batch)+1)
		args = append(args, s.opts.ProjectID)
		marks := make([]string, len(batch))
		for i, id := range batch {
			args = append(args, id)
			if s.opts.Dialect == SQLiteDialect {
				marks[i] = "?"
			} else {
				marks[i] = fmt.Sprintf("$%d", i+2)
			}
		}
		project := "$1"
		if s.opts.Dialect == SQLiteDialect {
			project = "?"
		}
		query := `SELECT id, content_hash FROM blocks WHERE project_id = ` + project +
			` AND id IN (` + strings.Join(marks, ", ") + `)`
		rows, err := s.opts.DB.QueryContext(s.ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("bowrain/blockstore: read block keys: %w", err)
		}
		for rows.Next() {
			var id, hash string
			if err := rows.Scan(&id, &hash); err != nil {
				rows.Close()
				return nil, fmt.Errorf("bowrain/blockstore: scan block key: %w", err)
			}
			out[id] = hash
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("bowrain/blockstore: iterate block keys: %w", err)
		}
	}
	return out, nil
}
