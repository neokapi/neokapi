package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	fw "github.com/neokapi/neokapi/memory"
)

// An entry a person edits carries a revision the store replaces on every write
// of its text. A person's edit names the revision it was based on, and the
// store refuses it when the entry has changed since. Writers that add to an
// entry without changing its text (an origin, a property) leave the revision
// alone, so they never refuse a person's edit made in between.

// ErrEntryNotFound is an entry id with no entry in the workspace.
var ErrEntryNotFound = errors.New("content-memory entry not found")

// OriginPerson is the origin source a person's edit records on the entry it
// changes. An entry that holds one was edited by a person, and nothing that
// removes its own origins removes the entry.
const OriginPerson = "user"

// Revisions returns the revision of each entry ids names that the workspace
// holds.
func (tm *PostgresStore) Revisions(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, tm.workspaceID)
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	rows, err := tm.db.QueryContext(ctx,
		`SELECT id, revision FROM tm_entries WHERE workspace_id = $1 AND id IN (`+strings.Join(placeholders, ",")+`)`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("read revisions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, rev string
		if err := rows.Scan(&id, &rev); err != nil {
			return nil, fmt.Errorf("read revisions: %w", err)
		}
		out[id] = rev
	}
	return out, rows.Err()
}

// Update writes a person's edit of entry when the stored entry is still at
// revision base, and returns the entry's new revision. It replaces the
// variants the edit carries and the entry's entities, keeps every origin, and
// records editor as the person who made the edit. The check and the write are
// one statement, and the whole edit is one transaction. An entry that has
// changed since is storage.ErrStale; one that is gone is ErrEntryNotFound.
func (tm *PostgresStore) Update(ctx context.Context, entry fw.Entry, base string, editor fw.Origin) (string, error) {
	if entry.ID == "" {
		return "", errors.New("entry ID is required")
	}
	if len(entry.Variants) == 0 {
		return "", errors.New("entry must have at least one variant")
	}
	fw.NormalizeEntryLocales(&entry)
	propsJSON := []byte("{}")
	if len(entry.Properties) > 0 {
		b, err := json.Marshal(entry.Properties)
		if err != nil {
			return "", fmt.Errorf("marshal properties: %w", err)
		}
		propsJSON = b
	}
	now := time.Now()
	next := storage.NewRevision()
	editor.Source = OriginPerson

	err := tm.db.Transition(ctx, func(r storage.Runner) error {
		res, err := r.ExecContext(ctx, `
			UPDATE tm_entries
			SET hint_src_lang = $3, properties = $4::jsonb, note = $5, updated_at = $6, revision = $7
			WHERE workspace_id = $1 AND id = $2 AND revision = $8`,
			tm.workspaceID, entry.ID, string(entry.HintSrcLang), string(propsJSON), entry.Note, now, next, base)
		if err != nil {
			return fmt.Errorf("update entry: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("update entry: %w", err)
		}
		if n == 0 {
			var one int
			err := r.QueryRowContext(ctx,
				`SELECT 1 FROM tm_entries WHERE workspace_id = $1 AND id = $2`,
				tm.workspaceID, entry.ID).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrEntryNotFound
			}
			if err != nil {
				return fmt.Errorf("read entry %s: %w", entry.ID, err)
			}
			return storage.ErrStale
		}
		if err := tm.writeChildren(ctx, r, entry, now, false); err != nil {
			return err
		}
		return tm.putOrigin(ctx, r, entry.ID, editor, now)
	})
	if err != nil {
		return "", err
	}
	return next, nil
}

// AddIfAbsent stores entry when the workspace holds no entry under its id, and
// reports whether it did. It never changes an entry that exists.
func (tm *PostgresStore) AddIfAbsent(ctx context.Context, entry fw.Entry) (bool, error) {
	if entry.ID == "" {
		return false, errors.New("entry ID is required")
	}
	if len(entry.Variants) == 0 {
		return false, errors.New("entry must have at least one variant")
	}
	fw.NormalizeEntryLocales(&entry)
	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}
	propsJSON := []byte("{}")
	if len(entry.Properties) > 0 {
		b, err := json.Marshal(entry.Properties)
		if err != nil {
			return false, fmt.Errorf("marshal properties: %w", err)
		}
		propsJSON = b
	}
	created := false
	err := tm.db.Transition(ctx, func(r storage.Runner) error {
		res, err := r.ExecContext(ctx, `
			INSERT INTO tm_entries
				(workspace_id, id, project_id, stream, hint_src_lang, properties, note, point, unit, created_at, updated_at, revision)
			VALUES ($1, $2, $3, '', $4, $5::jsonb, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (workspace_id, id) DO NOTHING`,
			tm.workspaceID, entry.ID, entry.ProjectID, string(entry.HintSrcLang), string(propsJSON),
			entry.Note, entry.Point, entry.Unit, entry.CreatedAt, entry.UpdatedAt, storage.NewRevision())
		if err != nil {
			return fmt.Errorf("insert entry: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("insert entry: %w", err)
		}
		if n == 0 {
			return nil
		}
		created = true
		return tm.writeChildren(ctx, r, entry, now, true)
	})
	return created, err
}

// AddOrigin records one more origin on the entry id holds, with the properties
// that come with it, in one transaction. It replaces an origin with the same
// source and key, and leaves the entry's text, its revision and its other
// origins and properties as they are. It reports false when the workspace
// holds no entry id. It is the Bowrain counterpart of the local store's.
func (tm *PostgresStore) AddOrigin(ctx context.Context, id string, origin fw.Origin, properties map[string]string) (bool, error) {
	found := false
	err := tm.db.Transition(ctx, func(r storage.Runner) error {
		var propsJSON []byte
		err := r.QueryRowContext(ctx,
			`SELECT properties FROM tm_entries WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
			tm.workspaceID, id).Scan(&propsJSON)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read entry %s: %w", id, err)
		}
		found = true
		now := time.Now()
		if len(properties) > 0 {
			merged := map[string]string{}
			if len(propsJSON) > 0 {
				if err := json.Unmarshal(propsJSON, &merged); err != nil {
					return fmt.Errorf("read the properties of entry %s: %w", id, err)
				}
			}
			maps.Copy(merged, properties)
			b, err := json.Marshal(merged)
			if err != nil {
				return fmt.Errorf("marshal properties: %w", err)
			}
			if _, err := r.ExecContext(ctx,
				`UPDATE tm_entries SET properties = $3::jsonb, updated_at = $4 WHERE workspace_id = $1 AND id = $2`,
				tm.workspaceID, id, string(b), now); err != nil {
				return fmt.Errorf("update entry %s: %w", id, err)
			}
		}
		return tm.putOrigin(ctx, r, id, origin, now)
	})
	return found, err
}

// RemoveOrigin removes the origin with source and key from the entry id
// holds, and removes the entry with it when no origin is left. An entry a
// person edited holds the person's origin, so it stays. It reports whether
// the entry was removed.
func (tm *PostgresStore) RemoveOrigin(ctx context.Context, id, source, key string) (bool, error) {
	removed := false
	err := tm.db.Transition(ctx, func(r storage.Runner) error {
		var one int
		err := r.QueryRowContext(ctx,
			`SELECT 1 FROM tm_entries WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
			tm.workspaceID, id).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read entry %s: %w", id, err)
		}
		if _, err := r.ExecContext(ctx,
			`DELETE FROM tm_entry_origins WHERE workspace_id = $1 AND entry_id = $2 AND source = $3 AND key = $4`,
			tm.workspaceID, id, source, key); err != nil {
			return fmt.Errorf("remove origin: %w", err)
		}
		var left int
		if err := r.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM tm_entry_origins WHERE workspace_id = $1 AND entry_id = $2`,
			tm.workspaceID, id).Scan(&left); err != nil {
			return fmt.Errorf("count origins: %w", err)
		}
		if left > 0 {
			return nil
		}
		if _, err := r.ExecContext(ctx,
			`DELETE FROM tm_entries WHERE workspace_id = $1 AND id = $2`, tm.workspaceID, id); err != nil {
			return fmt.Errorf("delete entry: %w", err)
		}
		removed = true
		return nil
	})
	return removed, err
}

// putOrigin replaces the origin with o's source and key on entry id, or adds
// it after the others.
func (tm *PostgresStore) putOrigin(ctx context.Context, r storage.Runner, id string, o fw.Origin, now time.Time) error {
	if _, err := r.ExecContext(ctx,
		`DELETE FROM tm_entry_origins WHERE workspace_id = $1 AND entry_id = $2 AND source = $3 AND key = $4`,
		tm.workspaceID, id, o.Source, o.Key); err != nil {
		return fmt.Errorf("replace origin: %w", err)
	}
	var next int
	if err := r.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(ordinal) + 1, 0) FROM tm_entry_origins WHERE workspace_id = $1 AND entry_id = $2`,
		tm.workspaceID, id).Scan(&next); err != nil {
		return fmt.Errorf("read origins of entry %s: %w", id, err)
	}
	addedAt := o.AddedAt
	if addedAt.IsZero() {
		addedAt = now
	}
	if _, err := r.ExecContext(ctx, `INSERT INTO tm_entry_origins
		(workspace_id, entry_id, ordinal, source, key, reference, added_at, added_by, session_id, context_fp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		tm.workspaceID, id, next, o.Source, o.Key, o.Reference,
		addedAt, o.AddedBy, o.SessionID, o.ContextFingerprint); err != nil {
		return fmt.Errorf("insert origin: %w", err)
	}
	return nil
}
