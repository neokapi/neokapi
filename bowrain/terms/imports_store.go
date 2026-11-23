package terms

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// ImportRecord is one terms import: the id the server minted for it, the
// file it last read, who created it and who last updated it, and when.
type ImportRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ErrImportNotFound is an import the workspace holds no record of.
var ErrImportNotFound = errors.New("no such import")

// ImportLedger keeps the record of each import, so the list a person picks
// an earlier import from is a bounded read and shows an import that wrote
// nothing.
type ImportLedger interface {
	// CreateImport records a new import.
	CreateImport(ctx context.Context, r ImportRecord) error
	// TouchImport records that by ran the import id again at at, under the
	// file name name. ErrImportNotFound when there is no such import.
	TouchImport(ctx context.Context, id, name, by string, at time.Time) (ImportRecord, error)
	// GetImport reads one import. ok is false when there is none.
	GetImport(ctx context.Context, id string) (ImportRecord, bool, error)
	// Imports lists up to limit imports, the latest updated first.
	Imports(ctx context.Context, limit int) ([]ImportRecord, error)
}

func (tb *PostgresStore) CreateImport(ctx context.Context, r ImportRecord) error {
	_, err := tb.run().ExecContext(ctx, `
		INSERT INTO tb_imports (workspace_id, id, name, created_by, created_at, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, '', $5)`, tb.workspaceID, r.ID, r.Name, r.CreatedBy, r.CreatedAt)
	if err != nil {
		return fmt.Errorf("record import: %w", err)
	}
	return nil
}

func (tb *PostgresStore) TouchImport(ctx context.Context, id, name, by string, at time.Time) (ImportRecord, error) {
	var r ImportRecord
	err := tb.run().QueryRowContext(ctx, `
		UPDATE tb_imports SET name = $3, updated_by = $4, updated_at = $5
		WHERE workspace_id = $1 AND id = $2
		RETURNING id, name, created_by, created_at, updated_by, updated_at`,
		tb.workspaceID, id, name, by, at).Scan(&r.ID, &r.Name, &r.CreatedBy, &r.CreatedAt, &r.UpdatedBy, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrImportNotFound
	}
	if err != nil {
		return r, fmt.Errorf("update import record: %w", err)
	}
	return r, nil
}

func (tb *PostgresStore) GetImport(ctx context.Context, id string) (ImportRecord, bool, error) {
	var r ImportRecord
	err := tb.run().QueryRowContext(ctx, `
		SELECT id, name, created_by, created_at, updated_by, updated_at
		FROM tb_imports WHERE workspace_id = $1 AND id = $2`, tb.workspaceID, id).
		Scan(&r.ID, &r.Name, &r.CreatedBy, &r.CreatedAt, &r.UpdatedBy, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, fmt.Errorf("read import record: %w", err)
	}
	return r, true, nil
}

func (tb *PostgresStore) Imports(ctx context.Context, limit int) ([]ImportRecord, error) {
	rows, err := tb.run().QueryContext(ctx, `
		SELECT id, name, created_by, created_at, updated_by, updated_at
		FROM tb_imports WHERE workspace_id = $1
		ORDER BY updated_at DESC, id LIMIT $2`, tb.workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list imports: %w", err)
	}
	defer rows.Close()
	out := []ImportRecord{}
	for rows.Next() {
		var r ImportRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.CreatedBy, &r.CreatedAt, &r.UpdatedBy, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan import record: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// memoryImports is the in-memory ImportLedger the revisioned wrapper keeps.
type memoryImports struct {
	mu      sync.Mutex
	records map[string]ImportRecord
}

func (m *memoryImports) CreateImport(_ context.Context, r ImportRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.records == nil {
		m.records = map[string]ImportRecord{}
	}
	if _, dup := m.records[r.ID]; dup {
		return fmt.Errorf("record import: %s exists", r.ID)
	}
	r.UpdatedBy, r.UpdatedAt = "", r.CreatedAt
	m.records[r.ID] = r
	return nil
}

func (m *memoryImports) TouchImport(_ context.Context, id, name, by string, at time.Time) (ImportRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return r, ErrImportNotFound
	}
	r.Name, r.UpdatedBy, r.UpdatedAt = name, by, at
	m.records[id] = r
	return r, nil
}

func (m *memoryImports) GetImport(_ context.Context, id string) (ImportRecord, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	return r, ok, nil
}

func (m *memoryImports) Imports(_ context.Context, limit int) ([]ImportRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ImportRecord, 0, len(m.records))
	for _, r := range m.records {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b ImportRecord) int {
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
