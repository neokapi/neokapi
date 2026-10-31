package sqlitestore

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// ListContextObjects implements platstore.ContextObjectStore.
func (s *SQLiteStore) ListContextObjects(ctx context.Context, projectID, dir string) ([]string, error) {
	if err := storeutil.CheckContextDir(dir); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT name FROM context_objects WHERE project_id = ? AND substr(name, 1, ?) = ?`,
		projectID, len(dir), dir)
	if err != nil {
		return nil, fmt.Errorf("list context objects under %s: %w", dir, err)
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("list context objects under %s: %w", dir, err)
		}
		if strings.HasPrefix(name, dir) {
			names = append(names, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list context objects under %s: %w", dir, err)
	}
	slices.Sort(names)
	return names, nil
}

// GetContextObject implements platstore.ContextObjectStore.
func (s *SQLiteStore) GetContextObject(ctx context.Context, projectID, name string) (*platstore.ContextObject, error) {
	if err := storeutil.CheckContextObjectName(name); err != nil {
		return nil, err
	}
	obj := platstore.ContextObject{Name: name}
	var at string
	err := s.db.QueryRowContext(ctx,
		`SELECT data, sha256, pushed_by, pushed_at FROM context_objects WHERE project_id = ? AND name = ?`,
		projectID, name).Scan(&obj.Data, &obj.SHA256, &obj.PushedBy, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", platstore.ErrNoContextObject, name)
	}
	if err != nil {
		return nil, fmt.Errorf("read context object %s: %w", name, err)
	}
	if obj.Data == nil {
		obj.Data = []byte{}
	}
	if obj.PushedAt, err = storeutil.ParseStoredTime("pushed_at", at); err != nil {
		return nil, fmt.Errorf("context object %s: %w", name, err)
	}
	return &obj, nil
}

// PutContextObjects implements platstore.ContextObjectStore. The first
// statement of the transaction is an insert, so the transaction takes the
// write lock before it reads, and a concurrent push waits on the busy timeout
// rather than failing on a stale snapshot.
func (s *SQLiteStore) PutContextObjects(ctx context.Context, projectID, pushedBy string, batch []platstore.ContextObject) error {
	for _, obj := range batch {
		if err := storeutil.CheckContextObject(obj.Name, obj.Data); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("put context objects: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	at := time.Now().UTC().Format(time.RFC3339Nano)
	for _, obj := range batch {
		data := obj.Data
		if data == nil {
			data = []byte{}
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO context_objects (project_id, name, data, sha256, pushed_by, pushed_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(project_id, name) DO NOTHING`,
			projectID, obj.Name, data, storeutil.ContextObjectDigest(data), pushedBy, at)
		if err != nil {
			return fmt.Errorf("put context object %s: %w", obj.Name, err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("put context object %s: %w", obj.Name, err)
		} else if n == 1 {
			continue
		}
		var held []byte
		if err := tx.QueryRowContext(ctx,
			`SELECT data FROM context_objects WHERE project_id = ? AND name = ?`,
			projectID, obj.Name).Scan(&held); err != nil {
			return fmt.Errorf("read held context object %s: %w", obj.Name, err)
		}
		if !bytes.Equal(held, data) {
			return &platstore.ContextObjectExistsError{Name: obj.Name}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("put context objects: %w", err)
	}
	return nil
}

var _ platstore.ContextObjectStore = (*SQLiteStore)(nil)
