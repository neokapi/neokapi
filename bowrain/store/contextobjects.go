package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// ListContextObjects implements platstore.ContextObjectStore.
func (s *PostgresStore) ListContextObjects(ctx context.Context, projectID, dir string) ([]string, error) {
	if err := storeutil.CheckContextDir(dir); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT name FROM context_objects WHERE project_id = $1 AND starts_with(name, $2)`,
		projectID, dir)
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
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list context objects under %s: %w", dir, err)
	}
	// Byte order, which the database's collation does not promise.
	slices.Sort(names)
	return names, nil
}

// GetContextObject implements platstore.ContextObjectStore.
func (s *PostgresStore) GetContextObject(ctx context.Context, projectID, name string) (*platstore.ContextObject, error) {
	if err := storeutil.CheckContextObjectName(name); err != nil {
		return nil, err
	}
	obj := platstore.ContextObject{Name: name}
	err := s.db.QueryRowContext(ctx,
		`SELECT data, sha256, pushed_by, pushed_at FROM context_objects WHERE project_id = $1 AND name = $2`,
		projectID, name).Scan(&obj.Data, &obj.SHA256, &obj.PushedBy, &obj.PushedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", platstore.ErrNoContextObject, name)
	}
	if err != nil {
		return nil, fmt.Errorf("read context object %s: %w", name, err)
	}
	obj.PushedAt = obj.PushedAt.UTC()
	return &obj, nil
}

// PutContextObjects implements platstore.ContextObjectStore. Each object is
// inserted unless its name is held; a held name's bytes are then compared. Two
// pushes creating one name at once serialize on the primary key: the second
// insert waits for the first to commit and finds its row.
func (s *PostgresStore) PutContextObjects(ctx context.Context, projectID, pushedBy string, batch []platstore.ContextObject) error {
	for _, obj := range batch {
		if err := storeutil.CheckContextObject(obj.Name, obj.Data); err != nil {
			return err
		}
	}
	// Names are written in byte order, so two batches that share names
	// take their row locks in one order and cannot deadlock.
	batch = slices.SortedFunc(slices.Values(batch), func(a, b platstore.ContextObject) int {
		return strings.Compare(a.Name, b.Name)
	})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("put context objects: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, obj := range batch {
		data := obj.Data
		if data == nil {
			data = []byte{}
		}
		digest := storeutil.ContextObjectDigest(data)
		res, err := tx.ExecContext(ctx,
			`INSERT INTO context_objects (project_id, name, data, sha256, pushed_by, pushed_at)
			 VALUES ($1, $2, $3, $4, $5, NOW())
			 ON CONFLICT (project_id, name) DO NOTHING`,
			projectID, obj.Name, data, digest, pushedBy)
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
			`SELECT data FROM context_objects WHERE project_id = $1 AND name = $2`,
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

// DigestContextObjects implements platstore.ContextObjectStore.
func (s *PostgresStore) DigestContextObjects(ctx context.Context, projectID, dir string) (map[string]string, error) {
	if err := storeutil.CheckContextDir(dir); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, sha256 FROM context_objects WHERE project_id = $1 AND starts_with(name, $2)`,
		projectID, dir)
	if err != nil {
		return nil, fmt.Errorf("digest context objects under %s: %w", dir, err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, digest string
		if err := rows.Scan(&name, &digest); err != nil {
			return nil, fmt.Errorf("digest context objects under %s: %w", dir, err)
		}
		out[name] = digest
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("digest context objects under %s: %w", dir, err)
	}
	return out, nil
}

// ContextKey implements platstore.ContextObjectStore.
func (s *PostgresStore) ContextKey(ctx context.Context, projectID string) (string, error) {
	var key string
	err := s.db.QueryRowContext(ctx, `SELECT context_key FROM projects WHERE id = $1`, projectID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", platstore.ErrNoContextProject, projectID)
	}
	if err != nil {
		return "", fmt.Errorf("read the context key of project %s: %w", projectID, err)
	}
	return key, nil
}

// BindContextKey implements platstore.ContextObjectStore. The update binds
// only an unbound project, so of two first pushes naming different keys the
// one that commits first binds and the other reads its key back.
func (s *PostgresStore) BindContextKey(ctx context.Context, projectID, key string) (string, error) {
	if key == "" {
		return "", errors.New("bind a context key: the key is empty")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE projects SET context_key = $2 WHERE id = $1 AND context_key = ''`, projectID, key); err != nil {
		return "", fmt.Errorf("bind the context key of project %s: %w", projectID, err)
	}
	return s.ContextKey(ctx, projectID)
}

var _ platstore.ContextObjectStore = (*PostgresStore)(nil)
