package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gokapi/gokapi/bowrain/storage"
	"github.com/gokapi/gokapi/core/model"
	platstore "github.com/gokapi/gokapi/platform/store"
	"github.com/google/uuid"
)

// PostgresStore implements ContentStore using PostgreSQL.
type PostgresStore struct {
	db *storage.PgDB
}

// NewPostgresStore opens a PostgreSQL-backed ContentStore.
func NewPostgresStore(connStr string) (*PostgresStore, error) {
	db, err := storage.OpenPostgres(connStr)
	if err != nil {
		return nil, fmt.Errorf("open store database: %w", err)
	}
	if err := storage.MigratePostgresNS(db, "store_schema_migrations", storeMigrationsPg); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate store schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

// NewPostgresStoreFromDB wraps an existing PgDB for content store use.
func NewPostgresStoreFromDB(db *storage.PgDB) (*PostgresStore, error) {
	if err := storage.MigratePostgresNS(db, "store_schema_migrations", storeMigrationsPg); err != nil {
		return nil, fmt.Errorf("migrate store schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

// Close closes the underlying database.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

// ---------------------------------------------------------------------------
// Project CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateProject(ctx context.Context, p *platstore.Project) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	locales := joinLocales(p.TargetLocales)
	propsJSON, err := json.Marshal(p.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO projects (id, name, source_locale, target_locales, properties, workspace_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.ID, p.Name, string(p.SourceLocale), locales, string(propsJSON),
		p.WorkspaceID, now, now)
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetProject(ctx context.Context, id string) (*platstore.Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, source_locale, target_locales, properties, workspace_id, created_at, updated_at
		 FROM projects WHERE id = $1`, id)
	return scanProjectPg(row)
}

func (s *PostgresStore) ListProjects(ctx context.Context) ([]*platstore.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, source_locale, target_locales, properties, workspace_id, created_at, updated_at
		 FROM projects ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	result := make([]*platstore.Project, 0)
	for rows.Next() {
		p, err := scanProjectPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateProject(ctx context.Context, p *platstore.Project) error {
	p.UpdatedAt = time.Now().UTC()
	locales := joinLocales(p.TargetLocales)
	propsJSON, err := json.Marshal(p.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET name=$1, source_locale=$2, target_locales=$3, properties=$4, workspace_id=$5, updated_at=$6
		 WHERE id=$7`,
		p.Name, string(p.SourceLocale), locales, string(propsJSON),
		p.WorkspaceID, p.UpdatedAt, p.ID)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %s not found", p.ID)
	}
	return nil
}

func (s *PostgresStore) DeleteProject(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %s not found", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Item management
// ---------------------------------------------------------------------------

func (s *PostgresStore) StoreItem(ctx context.Context, projectID string, item *platstore.Item) error {
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now

	propsJSON, err := json.Marshal(item.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	if item.BlockIndex == "" {
		item.BlockIndex = "{}"
	}
	if item.ItemType == "" {
		item.ItemType = "file"
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO items (project_id, name, format, item_type, source_bytes, block_index, properties, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT(project_id, name) DO UPDATE SET
			format=EXCLUDED.format, item_type=EXCLUDED.item_type,
			source_bytes=EXCLUDED.source_bytes, block_index=EXCLUDED.block_index,
			properties=EXCLUDED.properties, updated_at=EXCLUDED.updated_at`,
		projectID, item.Name, item.Format, item.ItemType, item.SourceBytes,
		item.BlockIndex, string(propsJSON), now, now)
	if err != nil {
		return fmt.Errorf("store item %q: %w", item.Name, err)
	}
	return nil
}

func (s *PostgresStore) GetItem(ctx context.Context, projectID, itemName string) (*platstore.Item, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT project_id, name, format, item_type, source_bytes, block_index, properties, created_at, updated_at
		 FROM items WHERE project_id=$1 AND name=$2`, projectID, itemName)
	return scanItemPg(row)
}

func (s *PostgresStore) ListItems(ctx context.Context, projectID string) ([]*platstore.Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id, name, format, item_type, source_bytes, block_index, properties, created_at, updated_at
		 FROM items WHERE project_id=$1 ORDER BY name`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()

	var result []*platstore.Item
	for rows.Next() {
		item, err := scanItemPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *PostgresStore) DeleteItem(ctx context.Context, projectID, itemName string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `DELETE FROM blocks WHERE project_id=$1 AND item_name=$2`, projectID, itemName)
	if err != nil {
		return fmt.Errorf("delete item blocks: %w", err)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM items WHERE project_id=$1 AND name=$2`, projectID, itemName)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("item %q not found in project %s", itemName, projectID)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Block storage
// ---------------------------------------------------------------------------

func (s *PostgresStore) StoreBlocks(ctx context.Context, projectID string, blocks []*model.Block) error {
	return s.storeBlocks(ctx, projectID, "", blocks)
}

func (s *PostgresStore) StoreBlocksForItem(ctx context.Context, projectID, itemName string, blocks []*model.Block) error {
	return s.storeBlocks(ctx, projectID, itemName, blocks)
}

func (s *PostgresStore) storeBlocks(ctx context.Context, projectID, itemName string, blocks []*model.Block) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO blocks (id, project_id, item_name, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, targets_json, properties, annotations, stored_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT(project_id, item_name, id) DO UPDATE SET
			name=EXCLUDED.name, type=EXCLUDED.type, mime_type=EXCLUDED.mime_type,
			translatable=EXCLUDED.translatable, content_hash=EXCLUDED.content_hash,
			context_hash=EXCLUDED.context_hash, source_json=EXCLUDED.source_json,
			targets_json=EXCLUDED.targets_json, properties=EXCLUDED.properties,
			annotations=EXCLUDED.annotations, updated_at=EXCLUDED.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	hashStmt, err := tx.PrepareContext(ctx,
		`SELECT content_hash FROM blocks WHERE project_id = $1 AND item_name = $2 AND id = $3`)
	if err != nil {
		return fmt.Errorf("prepare hash lookup: %w", err)
	}
	defer hashStmt.Close()

	now := time.Now().UTC()
	for _, b := range blocks {
		identity := model.ComputeIdentity(b)

		var existingHash string
		hashErr := hashStmt.QueryRowContext(ctx, projectID, itemName, b.ID).Scan(&existingHash)
		if hashErr != nil && hashErr != sql.ErrNoRows {
			return fmt.Errorf("hash lookup for block %s: %w", b.ID, hashErr)
		}
		isNew := hashErr == sql.ErrNoRows

		sourceJSON, err := json.Marshal(b.Source)
		if err != nil {
			return fmt.Errorf("marshal source for block %s: %w", b.ID, err)
		}
		targetsJSON, err := json.Marshal(b.Targets)
		if err != nil {
			return fmt.Errorf("marshal targets for block %s: %w", b.ID, err)
		}
		propsJSON, err := json.Marshal(b.Properties)
		if err != nil {
			return fmt.Errorf("marshal properties for block %s: %w", b.ID, err)
		}
		annsJSON, err := json.Marshal(b.Annotations)
		if err != nil {
			return fmt.Errorf("marshal annotations for block %s: %w", b.ID, err)
		}

		_, err = stmt.ExecContext(ctx,
			b.ID, projectID, itemName, b.Name, b.Type, b.MimeType, b.Translatable,
			identity.ContentHash, identity.ContextHash,
			string(sourceJSON), string(targetsJSON),
			string(propsJSON), string(annsJSON), now, now)
		if err != nil {
			return fmt.Errorf("store block %s: %w", b.ID, err)
		}

		if isNew {
			if err := logChangePg(ctx, tx, projectID, b.ID, "source_added", "", identity.ContentHash); err != nil {
				return fmt.Errorf("log change for block %s: %w", b.ID, err)
			}
		} else if existingHash != identity.ContentHash {
			if err := logChangePg(ctx, tx, projectID, b.ID, "source_modified", "", identity.ContentHash); err != nil {
				return fmt.Errorf("log change for block %s: %w", b.ID, err)
			}
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) GetBlock(ctx context.Context, projectID, blockID string) (*platstore.StoredBlock, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, item_name, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, targets_json, properties, annotations, stored_at, updated_at
		 FROM blocks WHERE project_id=$1 AND id=$2`, projectID, blockID)
	if err != nil {
		return nil, fmt.Errorf("query block: %w", err)
	}
	defer rows.Close()

	var result *platstore.StoredBlock
	for rows.Next() {
		sb, err := scanStoredBlockPg(rows)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return nil, fmt.Errorf("block ID %s is ambiguous - found in multiple items", blockID)
		}
		result = sb
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("block %s not found in project %s", blockID, projectID)
	}
	return result, nil
}

func (s *PostgresStore) GetBlocks(ctx context.Context, query platstore.BlockQuery) ([]*platstore.StoredBlock, error) {
	where := []string{"project_id = $1"}
	args := []any{query.ProjectID}
	paramN := 2

	if query.ItemName != "" {
		where = append(where, fmt.Sprintf("item_name = $%d", paramN))
		args = append(args, query.ItemName)
		paramN++
	}
	if len(query.IDs) > 0 {
		placeholders := make([]string, len(query.IDs))
		for i, id := range query.IDs {
			placeholders[i] = fmt.Sprintf("$%d", paramN)
			args = append(args, id)
			paramN++
		}
		where = append(where, fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")))
	}
	if query.ContentHash != "" {
		where = append(where, fmt.Sprintf("content_hash = $%d", paramN))
		args = append(args, query.ContentHash)
		paramN++
	}
	if query.Translatable != nil {
		where = append(where, fmt.Sprintf("translatable = $%d", paramN))
		args = append(args, *query.Translatable)
		paramN++
	}

	q := fmt.Sprintf(
		`SELECT id, project_id, item_name, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, targets_json, properties, annotations, stored_at, updated_at
		 FROM blocks WHERE %s ORDER BY id`, strings.Join(where, " AND "))

	if query.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", query.Limit)
	}
	if query.Offset > 0 {
		q += fmt.Sprintf(" OFFSET %d", query.Offset)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query blocks: %w", err)
	}
	defer rows.Close()

	var result []*platstore.StoredBlock
	for rows.Next() {
		sb, err := scanStoredBlockPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sb)
	}
	return result, rows.Err()
}

func (s *PostgresStore) DeleteBlock(ctx context.Context, projectID, blockID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM blocks WHERE project_id=$1 AND id=$2`, projectID, blockID)
	if err != nil {
		return fmt.Errorf("delete block: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("block %s not found in project %s", blockID, projectID)
	}

	if err := logChangePg(ctx, tx, projectID, blockID, "source_removed", "", ""); err != nil {
		return fmt.Errorf("log change for deleted block %s: %w", blockID, err)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Version management
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateVersion(ctx context.Context, projectID, label, description string) (*platstore.Version, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	versionID := uuid.NewString()
	now := time.Now().UTC()

	var blockCount int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM blocks WHERE project_id=$1`, projectID).Scan(&blockCount)
	if err != nil {
		return nil, fmt.Errorf("count blocks: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO versions (id, project_id, label, description, block_count, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		versionID, projectID, label, description, blockCount, now)
	if err != nil {
		return nil, fmt.Errorf("insert version: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO version_blocks (version_id, block_id, content_hash)
		 SELECT $1, id, content_hash FROM blocks WHERE project_id=$2`,
		versionID, projectID)
	if err != nil {
		return nil, fmt.Errorf("snapshot blocks: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit version: %w", err)
	}

	return &platstore.Version{
		ID:          versionID,
		ProjectID:   projectID,
		Label:       label,
		Description: description,
		BlockCount:  blockCount,
		CreatedAt:   now,
	}, nil
}

func (s *PostgresStore) GetVersion(ctx context.Context, versionID string) (*platstore.Version, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, label, description, block_count, created_at FROM versions WHERE id=$1`,
		versionID)

	var v platstore.Version
	err := row.Scan(&v.ID, &v.ProjectID, &v.Label, &v.Description, &v.BlockCount, &v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan version: %w", err)
	}
	return &v, nil
}

func (s *PostgresStore) ListVersions(ctx context.Context, projectID string) ([]*platstore.Version, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, label, description, block_count, created_at
		 FROM versions WHERE project_id=$1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()

	var result []*platstore.Version
	for rows.Next() {
		var v platstore.Version
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Label, &v.Description, &v.BlockCount, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		result = append(result, &v)
	}
	return result, rows.Err()
}

func (s *PostgresStore) Diff(ctx context.Context, fromVersionID, toVersionID string) (*platstore.VersionDiff, error) {
	diff := &platstore.VersionDiff{
		FromVersion: fromVersionID,
		ToVersion:   toVersionID,
	}

	fromBlocks, err := queryVersionBlocks(ctx, s.db.DB, fromVersionID)
	if err != nil {
		return nil, fmt.Errorf("query from blocks: %w", err)
	}

	toBlocks, err := queryVersionBlocks(ctx, s.db.DB, toVersionID)
	if err != nil {
		return nil, fmt.Errorf("query to blocks: %w", err)
	}

	for id, toHash := range toBlocks {
		fromHash, existed := fromBlocks[id]
		if !existed {
			diff.Changes = append(diff.Changes, platstore.BlockChange{
				BlockID: id, ChangeType: platstore.ChangeAdded, NewHash: toHash,
			})
		} else if fromHash != toHash {
			diff.Changes = append(diff.Changes, platstore.BlockChange{
				BlockID: id, ChangeType: platstore.ChangeModified, OldHash: fromHash, NewHash: toHash,
			})
		}
	}
	for id, fromHash := range fromBlocks {
		if _, exists := toBlocks[id]; !exists {
			diff.Changes = append(diff.Changes, platstore.BlockChange{
				BlockID: id, ChangeType: platstore.ChangeRemoved, OldHash: fromHash,
			})
		}
	}

	return diff, nil
}

// ---------------------------------------------------------------------------
// Scan helpers (PostgreSQL — uses time.Time directly for TIMESTAMPTZ)
// ---------------------------------------------------------------------------

func scanProjectPg(row scanner) (*platstore.Project, error) {
	var p platstore.Project
	var srcLocale, targetLocales, propsJSON string
	err := row.Scan(&p.ID, &p.Name, &srcLocale, &targetLocales, &propsJSON, &p.WorkspaceID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan project: %w", err)
	}
	p.SourceLocale = model.LocaleID(srcLocale)
	p.TargetLocales = splitLocales(targetLocales)
	if err := json.Unmarshal([]byte(propsJSON), &p.Properties); err != nil {
		p.Properties = map[string]string{}
	}
	return &p, nil
}

func scanItemPg(row scanner) (*platstore.Item, error) {
	var item platstore.Item
	var propsJSON string
	err := row.Scan(&item.ProjectID, &item.Name, &item.Format, &item.ItemType,
		&item.SourceBytes, &item.BlockIndex, &propsJSON, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan item: %w", err)
	}
	if err := json.Unmarshal([]byte(propsJSON), &item.Properties); err != nil {
		item.Properties = map[string]string{}
	}
	return &item, nil
}

func scanStoredBlockPg(row scanner) (*platstore.StoredBlock, error) {
	var sb platstore.StoredBlock
	sb.Block = &model.Block{}
	var sourceJSON, targetsJSON, propsJSON, annsJSON string

	err := row.Scan(
		&sb.Block.ID, &sb.ProjectID, &sb.ItemName, &sb.Block.Name, &sb.Block.Type,
		&sb.Block.MimeType, &sb.Block.Translatable, &sb.ContentHash, &sb.ContextHash,
		&sourceJSON, &targetsJSON, &propsJSON, &annsJSON, &sb.StoredAt, &sb.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan block: %w", err)
	}

	if err := json.Unmarshal([]byte(sourceJSON), &sb.Block.Source); err != nil {
		sb.Block.Source = nil
	}
	if err := json.Unmarshal([]byte(targetsJSON), &sb.Block.Targets); err != nil {
		sb.Block.Targets = make(map[model.LocaleID][]*model.Segment)
	}
	if err := json.Unmarshal([]byte(propsJSON), &sb.Block.Properties); err != nil {
		sb.Block.Properties = make(map[string]string)
	}
	sb.Block.Annotations = make(map[string]model.Annotation)

	return &sb, nil
}

// queryVersionBlocks loads block_id→content_hash map for a version, using defer for cleanup.
func queryVersionBlocks(ctx context.Context, db *sql.DB, versionID string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT block_id, content_hash FROM version_blocks WHERE version_id=$1`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]string{}
	for rows.Next() {
		var id, hash string
		if err := rows.Scan(&id, &hash); err != nil {
			return nil, err
		}
		result[id] = hash
	}
	return result, rows.Err()
}

// logChangePg inserts a single change log entry within a PostgreSQL transaction.
func logChangePg(ctx context.Context, tx *sql.Tx, projectID, blockID, changeType, locale, contentHash string) error {
	now := time.Now().UTC()
	var localeVal any
	if locale == "" {
		localeVal = nil
	} else {
		localeVal = locale
	}
	var hashVal any
	if contentHash == "" {
		hashVal = nil
	} else {
		hashVal = contentHash
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO change_log (project_id, block_id, change_type, locale, content_hash, logged_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		projectID, blockID, changeType, localeVal, hashVal, now)
	return err
}
