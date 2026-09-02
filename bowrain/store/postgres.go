package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/crypto"
	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// PostgresStore implements ContentStore using PostgreSQL.
type PostgresStore struct {
	db     *storage.PgDB
	cipher *crypto.Cipher // seals connector_config at rest; nil = plaintext
}

// SetSecretsCipher enables encryption-at-rest for secret columns (connector
// credentials). A nil cipher (no key configured) leaves values as plaintext;
// existing plaintext rows are read transparently and re-sealed on next write.
func (s *PostgresStore) SetSecretsCipher(c *crypto.Cipher) { s.cipher = c }

// NewPostgresStore opens a PostgreSQL-backed ContentStore.
func NewPostgresStore(connStr string) (*PostgresStore, error) {
	db, err := storage.OpenPostgres(connStr)
	if err != nil {
		return nil, fmt.Errorf("open store database: %w", err)
	}
	if err := storage.MigratePostgresNS(db, "store_schema_migrations", Migrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate store schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

// NewPostgresStoreFromDB wraps an existing PgDB for content store use.
func NewPostgresStoreFromDB(db *storage.PgDB) (*PostgresStore, error) {
	if err := storage.MigratePostgresNS(db, "store_schema_migrations", Migrations); err != nil {
		return nil, fmt.Errorf("migrate store schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

// SQLDB returns the underlying *sql.DB for sharing with subsystem stores.
func (s *PostgresStore) SQLDB() *sql.DB {
	return s.db.DB
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
		p.ID = id.New()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	locales := storeutil.JoinLocales(p.TargetLanguages)
	propsJSON, err := json.Marshal(p.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	if p.TargetLanguageMode == "" {
		p.TargetLanguageMode = "defined"
	}
	if p.DashboardVisibility == "" {
		p.DashboardVisibility = "private"
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO projects (id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, converge_policy, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		p.ID, p.Name, string(p.DefaultSourceLanguage), locales, p.TargetLanguageMode, p.DefaultStream, p.DashboardVisibility, string(propsJSON),
		p.WorkspaceID, platstore.NormalizeConvergePolicy(p.ConvergePolicy), now, now)
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetProject(ctx context.Context, id string) (*platstore.Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, converge_policy, archived, archived_at, created_at, updated_at
		 FROM projects WHERE id = $1`, id)
	return scanProject(row)
}

func (s *PostgresStore) ListProjects(ctx context.Context) ([]*platstore.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, converge_policy, archived, archived_at, created_at, updated_at
		 FROM projects WHERE archived=FALSE ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	result := make([]*platstore.Project, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateProject(ctx context.Context, p *platstore.Project) error {
	p.UpdatedAt = time.Now().UTC()
	locales := storeutil.JoinLocales(p.TargetLanguages)
	propsJSON, err := json.Marshal(p.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	if p.TargetLanguageMode == "" {
		p.TargetLanguageMode = "defined"
	}
	if p.DashboardVisibility == "" {
		p.DashboardVisibility = "private"
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET name=$1, default_source_language=$2, target_languages=$3, target_language_mode=$4, default_stream=$5, dashboard_visibility=$6, properties=$7, workspace_id=$8, converge_policy=$9, updated_at=$10
		 WHERE id=$11`,
		p.Name, string(p.DefaultSourceLanguage), locales, p.TargetLanguageMode, p.DefaultStream, p.DashboardVisibility, string(propsJSON),
		p.WorkspaceID, platstore.NormalizeConvergePolicy(p.ConvergePolicy), p.UpdatedAt, p.ID)
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %s not found", id)
	}

	// Most of the project's content goes with it on the projects foreign key.
	// These tables carry a bare project_id, so no cascade reaches them and the
	// verb clears them itself — otherwise a deleted project's history, notes,
	// decisions and proposals outlive every block they describe.
	tables := append(storeutil.ProjectScopedTablesWithoutCascade(), "change_log_archive")
	for _, table := range tables {
		//nolint:gosec // table is a fixed literal from storeutil, never user input
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE project_id=$1`, id); err != nil {
			return fmt.Errorf("delete %s for project %s: %w", table, id, err)
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) ArchiveProject(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET archived=TRUE, archived_at=$1, updated_at=$1 WHERE id=$2`, now, id)
	if err != nil {
		return fmt.Errorf("archive project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %s not found", id)
	}
	return nil
}

func (s *PostgresStore) RestoreProject(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET archived=FALSE, archived_at=NULL, updated_at=$1 WHERE id=$2`, now, id)
	if err != nil {
		return fmt.Errorf("restore project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %s not found", id)
	}
	return nil
}

func (s *PostgresStore) ListArchivedProjects(ctx context.Context, workspaceID string) ([]*platstore.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, converge_policy, archived, archived_at, created_at, updated_at
		 FROM projects WHERE workspace_id=$1 AND archived=TRUE ORDER BY archived_at DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list archived projects: %w", err)
	}
	defer rows.Close()
	result := make([]*platstore.Project, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Collection management
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateCollection(ctx context.Context, c *platstore.Collection) error {
	return s.createCollectionTx(ctx, s.db, c)
}

// createCollectionTx is CreateCollection against an arbitrary runner, so the
// same body serves the standalone call and the push transition.
func (s *PostgresStore) createCollectionTx(ctx context.Context, r storage.Runner, c *platstore.Collection) error {
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.ID == "" {
		c.ID = id.New()
	}
	if c.Kind == "" {
		c.Kind = platstore.CollectionUploaded
	}
	if c.ItemLabel == "" {
		c.ItemLabel = "item"
	}

	configJSON, err := json.Marshal(c.ConnectorConfig)
	if err != nil {
		return fmt.Errorf("marshal connector config: %w", err)
	}
	sealedConfig, err := s.cipher.Seal(string(configJSON))
	if err != nil {
		return fmt.Errorf("seal connector config: %w", err)
	}
	// The coordinates are slugs a recipe declares in plain sight, so unlike the
	// connector config — which carries credentials — they are stored unsealed.
	contextJSON, err := json.Marshal(c.Context)
	if err != nil {
		return fmt.Errorf("marshal collection context: %w", err)
	}
	c.Owner = venue.NormalizeContextOwner(c.Owner)

	_, err = r.ExecContext(ctx,
		`INSERT INTO collections (id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		c.ID, c.ProjectID, c.Name, string(c.Kind), c.ItemLabel, c.IsDefault, c.Stream,
		sealedConfig, string(contextJSON), c.Owner, c.ContextHash, c.PreviewKind, c.PreviewURL, now, now)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetCollection(ctx context.Context, projectID, collectionID string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at
		 FROM collections WHERE project_id=$1 AND id=$2`, projectID, collectionID)
	return s.scanCollectionPg(row)
}

func (s *PostgresStore) GetCollectionByName(ctx context.Context, projectID, name, stream string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at
		 FROM collections WHERE project_id=$1 AND name=$2 AND (stream='' OR stream=$3)`,
		projectID, name, stream)
	return s.scanCollectionPg(row)
}

func (s *PostgresStore) GetDefaultCollection(ctx context.Context, projectID string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at
		 FROM collections WHERE project_id=$1 AND is_default=TRUE`, projectID)
	return s.scanCollectionPg(row)
}

func (s *PostgresStore) ListCollections(ctx context.Context, projectID, stream string) ([]*platstore.Collection, error) {
	return s.listCollectionsTx(ctx, s.db, projectID, stream)
}

// listCollectionsTx is ListCollections against an arbitrary runner. A push
// reconciles against the collections its own transition may have just created,
// so this read has to see them.
func (s *PostgresStore) listCollectionsTx(ctx context.Context, r storage.Runner, projectID, stream string) ([]*platstore.Collection, error) {
	rows, err := r.QueryContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at
		 FROM collections WHERE project_id=$1 AND (stream='' OR stream=$2)
		 ORDER BY is_default DESC, name`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()

	var result []*platstore.Collection
	for rows.Next() {
		c, err := s.scanCollectionPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateCollection(ctx context.Context, c *platstore.Collection) error {
	return s.updateCollectionTx(ctx, s.db, c)
}

// updateCollectionTx is UpdateCollection against an arbitrary runner.
func (s *PostgresStore) updateCollectionTx(ctx context.Context, r storage.Runner, c *platstore.Collection) error {
	c.UpdatedAt = time.Now().UTC()

	configJSON, err := json.Marshal(c.ConnectorConfig)
	if err != nil {
		return fmt.Errorf("marshal connector config: %w", err)
	}
	sealedConfig, err := s.cipher.Seal(string(configJSON))
	if err != nil {
		return fmt.Errorf("seal connector config: %w", err)
	}
	contextJSON, err := json.Marshal(c.Context)
	if err != nil {
		return fmt.Errorf("marshal collection context: %w", err)
	}
	c.Owner = venue.NormalizeContextOwner(c.Owner)

	_, err = r.ExecContext(ctx,
		`UPDATE collections SET name=$1, kind=$2, item_label=$3, stream=$4, connector_config=$5, context=$6, owner=$7, context_hash=$8, preview_kind=$9, preview_url=$10, updated_at=$11
		 WHERE project_id=$12 AND id=$13`,
		c.Name, string(c.Kind), c.ItemLabel, c.Stream, sealedConfig,
		string(contextJSON), c.Owner, c.ContextHash, c.PreviewKind, c.PreviewURL, c.UpdatedAt, c.ProjectID, c.ID)
	if err != nil {
		return fmt.Errorf("update collection: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteCollection(ctx context.Context, projectID, collectionID string) error {
	var isDefault bool
	err := s.db.QueryRowContext(ctx,
		`SELECT is_default FROM collections WHERE project_id=$1 AND id=$2`,
		projectID, collectionID).Scan(&isDefault)
	if err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	if isDefault {
		return errors.New("cannot delete the default collection")
	}

	var defaultID string
	err = s.db.QueryRowContext(ctx,
		`SELECT id FROM collections WHERE project_id=$1 AND is_default=TRUE`, projectID).Scan(&defaultID)
	if err != nil {
		return fmt.Errorf("get default collection: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE items SET collection_id=$1 WHERE project_id=$2 AND collection_id=$3`,
		defaultID, projectID, collectionID)
	if err != nil {
		return fmt.Errorf("reassign items: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`DELETE FROM collections WHERE project_id=$1 AND id=$2`, projectID, collectionID)
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	return nil
}

func (s *PostgresStore) scanCollectionPg(row scanner) (*platstore.Collection, error) {
	var c platstore.Collection
	var kindStr, configJSON, contextJSON string
	err := row.Scan(&c.ID, &c.ProjectID, &c.Name, &kindStr, &c.ItemLabel,
		&c.IsDefault, &c.Stream, &configJSON, &contextJSON, &c.Owner, &c.ContextHash,
		&c.PreviewKind, &c.PreviewURL, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan collection: %w", err)
	}
	c.Kind = platstore.CollectionKind(kindStr)
	plainConfig, err := s.cipher.Open(configJSON)
	if err != nil {
		return nil, fmt.Errorf("open connector config: %w", err)
	}
	if err := json.Unmarshal([]byte(plainConfig), &c.ConnectorConfig); err != nil {
		c.ConnectorConfig = map[string]string{}
	}
	if err := json.Unmarshal([]byte(contextJSON), &c.Context); err != nil {
		c.Context = map[string]string{}
	}
	c.Owner = venue.NormalizeContextOwner(c.Owner)
	return &c, nil
}

// ---------------------------------------------------------------------------
// Item management
// ---------------------------------------------------------------------------

func (s *PostgresStore) StoreItem(ctx context.Context, projectID, stream string, item *platstore.Item) error {
	return storeItemTx(ctx, s.db, projectID, stream, item)
}

// storeItemTx is the work, on whatever executor the caller brings.
func storeItemTx(ctx context.Context, tx Runner, projectID, stream string, item *platstore.Item) error {
	stream = storeutil.DefaultStream(stream)
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
	if item.ID == "" {
		item.ID = id.New()
	}

	// An empty incoming collection id means "unspecified", not "no collection",
	// and what it resolves to depends on whether this item is already bound:
	// an item with no binding yet falls to the project's default collection,
	// while an item that is already filed under one keeps that binding. A
	// caller that could not resolve a collection this time round — a transient
	// lookup failure on a re-push — must not move content out of the collection
	// it was filed under. Both halves are decided in the statement below, on the
	// raw incoming id ($10): resolving the default here instead would hand the
	// upsert a non-empty id and clobber the existing binding (#1840).
	fallbackCollectionID := ""
	if item.CollectionID == "" {
		// On the caller's executor, not the pool: a push that reconciled its
		// collections in the same transaction must see them here, and a read
		// outside it would not.
		_ = tx.QueryRowContext(ctx,
			`SELECT id FROM collections WHERE project_id=$1 AND is_default=TRUE`,
			projectID).Scan(&fallbackCollectionID)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO items (id, project_id, stream, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, COALESCE(NULLIF($10::text, ''), $13::text), $11, $12)
		 ON CONFLICT(project_id, stream, name) DO UPDATE SET
			format=EXCLUDED.format, item_type=EXCLUDED.item_type,
			block_index=EXCLUDED.block_index, preview_html=EXCLUDED.preview_html,
			properties=EXCLUDED.properties,
			collection_id=COALESCE(NULLIF($10::text, ''), NULLIF(items.collection_id, ''), EXCLUDED.collection_id),
			updated_at=EXCLUDED.updated_at`,
		item.ID, projectID, stream, item.Name, item.Format, item.ItemType,
		item.BlockIndex, item.PreviewHTML, string(propsJSON), item.CollectionID, now, now,
		fallbackCollectionID)
	if err != nil {
		return fmt.Errorf("store item %q: %w", item.Name, err)
	}
	return nil
}

func (s *PostgresStore) GetItem(ctx context.Context, projectID, stream, itemName string) (*platstore.Item, error) {
	stream = storeutil.DefaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at
		 FROM items WHERE project_id=$1 AND stream=$2 AND name=$3`, projectID, stream, itemName)
	return scanItemPg(row)
}

func (s *PostgresStore) ListItems(ctx context.Context, projectID, stream string) ([]*platstore.Item, error) {
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at
		 FROM items WHERE project_id=$1 AND stream=$2 ORDER BY name`, projectID, stream)
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

func (s *PostgresStore) DeleteItem(ctx context.Context, projectID, stream, itemName string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := deleteItemTx(ctx, tx, projectID, stream, itemName); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteItemTx is the work, on whatever executor the caller brings — its own
// transaction, or the one a whole push is landing in.
func deleteItemTx(ctx context.Context, tx Runner, projectID, stream, itemName string) error {
	stream = storeutil.DefaultStream(stream)

	// Remove this stream's item row first; its absence is the not-found signal.
	// The id comes back with it, because the rows describing an item are keyed
	// on what it IS and the name is only how this call addressed it.
	var itemID string
	err := tx.QueryRowContext(ctx,
		`DELETE FROM items WHERE project_id=$1 AND stream=$2 AND name=$3 RETURNING id`,
		projectID, stream, itemName).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("item %q not found in project %s", itemName, projectID)
	}
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}

	// Everything describing this item on this stream goes with it. Each table
	// is stream-scoped, and so now is blocks, so a sibling branch holding the
	// same item at the same ids is untouched by any of it.
	for _, table := range storeutil.BlockScopedTables() {
		q := `DELETE FROM ` + table + ` WHERE project_id=$1 AND stream=$2
			 AND block_id IN (SELECT id FROM blocks WHERE project_id=$1 AND stream=$2 AND item_name=$3)`
		if _, err := tx.ExecContext(ctx, q, projectID, stream, itemName); err != nil {
			return fmt.Errorf("delete %s for item %q: %w", table, itemName, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM unit_decisions WHERE project_id=$1 AND stream=$2 AND item_id=$3`,
		projectID, stream, itemID); err != nil {
		return fmt.Errorf("delete unit decisions for item %q: %w", itemName, err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM blocks WHERE project_id=$1 AND stream=$2 AND item_name=$3`,
		projectID, stream, itemName); err != nil {
		return fmt.Errorf("delete item blocks: %w", err)
	}
	return nil
}

// renameItemTx moves an item to a new path, keeping its identity.
//
// The rows describing an item hang off its id, so the rename is one UPDATE
// there. Blocks are the exception: they carry item_name as their address, so
// they move with it. Nothing about a block's identity changes — which is the
// point. A rename that minted new blocks would orphan every approval on the
// file, and "delete the old, create the new" is exactly what did that.
func renameItemTx(ctx context.Context, tx Runner, projectID, stream, itemID, newName string) error {
	stream = storeutil.DefaultStream(stream)

	var oldName string
	err := tx.QueryRowContext(ctx,
		`SELECT name FROM items WHERE project_id=$1 AND stream=$2 AND id=$3`,
		projectID, stream, itemID).Scan(&oldName)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("rename item %s: not found in project %s", itemID, projectID)
	}
	if err != nil {
		return fmt.Errorf("read item %s for rename: %w", itemID, err)
	}
	if oldName == newName {
		return nil
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE items SET name=$4, updated_at=NOW() WHERE project_id=$1 AND stream=$2 AND id=$3`,
		projectID, stream, itemID, newName); err != nil {
		return fmt.Errorf("rename item %s to %q: %w", itemID, newName, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE blocks SET item_name=$4 WHERE project_id=$1 AND stream=$2 AND item_name=$3`,
		projectID, stream, oldName, newName); err != nil {
		return fmt.Errorf("move blocks of %q to %q: %w", oldName, newName, err)
	}
	// The decision ledger keys on the item id and carries the name only as a
	// label; a label left behind would report approvals against a path that no
	// longer exists.
	if _, err := tx.ExecContext(ctx,
		`UPDATE unit_decisions SET item_name=$4 WHERE project_id=$1 AND stream=$2 AND item_id=$3`,
		projectID, stream, itemID, newName); err != nil {
		return fmt.Errorf("relabel decisions of %q: %w", newName, err)
	}
	return nil
}

func (s *PostgresStore) GetItemByID(ctx context.Context, projectID, stream, itemID string) (*platstore.Item, error) {
	stream = storeutil.DefaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at
		 FROM items WHERE project_id=$1 AND stream=$2 AND id=$3`, projectID, stream, itemID)
	return scanItemPg(row)
}

// ---------------------------------------------------------------------------
// Block storage
// ---------------------------------------------------------------------------

func (s *PostgresStore) StoreBlocks(ctx context.Context, projectID, stream string, blocks []*model.Block) error {
	return s.storeBlocks(ctx, projectID, stream, "", blocks)
}

func (s *PostgresStore) StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error {
	return s.storeBlocks(ctx, projectID, stream, itemName, blocks)
}

// PruneItemBlocks removes the blocks of one item the producer no longer
// declares. See store.BlockStore for what it is for.
//
// The item's rows are loaded and diffed in Go rather than handed to SQL as a
// NOT IN list: `keep` is one file's worth of keys, but a NOT IN over it binds a
// parameter per key and Postgres stops at 65535 — the same ceiling the batched
// hash prefetch in storeBlocks exists to stay under.
//
// Scoped to one stream, so pruning a branch leaves its parent alone.
func (s *PostgresStore) PruneItemBlocks(ctx context.Context, projectID, stream, itemName string, keep []string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	n, err := pruneItemBlocksTx(ctx, tx, projectID, stream, itemName, keep)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit prune for item %q: %w", itemName, err)
	}
	return n, nil
}

// pruneItemBlocksTx is the work, on whatever executor the caller brings — its
// own transaction, or the one a whole push is landing in.
func pruneItemBlocksTx(ctx context.Context, tx Runner, projectID, stream, itemName string, keep []string) (int, error) {
	if itemName == "" {
		return 0, nil
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id, source_id FROM blocks WHERE project_id=$1 AND stream=$2 AND item_name=$3`,
		projectID, stream, itemName)
	if err != nil {
		return 0, fmt.Errorf("load item blocks for prune: %w", err)
	}
	declared := make(map[string]struct{}, len(keep))
	for _, k := range keep {
		declared[k] = struct{}{}
	}
	var stale []string
	for rows.Next() {
		var id, srcID string
		if err := rows.Scan(&id, &srcID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan item block for prune: %w", err)
		}
		// The producer's key for this row: what it last sent, falling back to
		// the row id for a legacy row stored before source ids were recorded.
		key := srcID
		if key == "" {
			key = id
		}
		if _, still := declared[key]; !still {
			stale = append(stale, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("item block rows for prune: %w", err)
	}
	if len(stale) == 0 {
		return 0, nil
	}

	args := append([]any{projectID, stream}, anyStrings(stale)...)
	for _, table := range storeutil.BlockScopedTables() {
		q := `DELETE FROM ` + table + ` WHERE project_id=$1 AND stream=$2 AND block_id IN (` +
			placeholderList("pg", 3, len(stale)) + `)`
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return 0, fmt.Errorf("prune %s for item %q: %w", table, itemName, err)
		}
	}
	del := `DELETE FROM blocks WHERE project_id=$1 AND stream=$2 AND id IN (` +
		placeholderList("pg", 3, len(stale)) + `)`
	if _, err := tx.ExecContext(ctx, del, args...); err != nil {
		return 0, fmt.Errorf("prune blocks for item %q: %w", itemName, err)
	}
	return len(stale), nil
}

func (s *PostgresStore) storeBlocks(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := storeBlocksTx(ctx, tx, projectID, stream, itemName, blocks); err != nil {
		return err
	}
	return tx.Commit()
}

// storeBlocksTx is the work, on whatever executor the caller brings.
func storeBlocksTx(ctx context.Context, tx Runner, projectID, stream, itemName string, blocks []*model.Block) error {
	stream = storeutil.DefaultStream(stream)

	// When storing blocks for a specific item, map format-reader IDs (source_id)
	// to internal project-unique IDs.
	//
	// Both directions are loaded, because a block can arrive carrying either
	// kind of ID. A push or a format read supplies the caller's own ID, which
	// this item may already have minted an internal ID for. But a caller that
	// read blocks *out* of this store — the extraction worker annotating an
	// item, the editor writing a parsed item back — hands back rows whose ID is
	// already the internal one. Treating that as an unseen caller ID minted a
	// second row for content that was already stored, under source_id = the
	// first row's ID: two rows, same content_hash, different ids, which is
	// exactly the duplication in #1527.
	existingSourceIDs := map[string]string{} // caller/source id → internal id
	internalSourceIDs := map[string]string{} // internal id → its source id
	// The item's durable identity, recorded on every block so a rename moves the
	// address and nothing else. Resolve-or-create, and through the same helper
	// the decision ledger uses: storing blocks for an item IS the assertion that
	// the item exists, and if the two paths minted ids independently a block and
	// the decision about it would disagree on which file they belong to. Empty
	// only for an item-less write.
	var itemID string
	if itemName != "" {
		var err error
		if itemID, err = resolveItemIDPg(ctx, tx, projectID, stream, itemName); err != nil {
			return err
		}
	}
	if itemName != "" {
		rows, err := tx.QueryContext(ctx,
			`SELECT source_id, id FROM blocks WHERE project_id=$1 AND stream=$2 AND item_name=$3`,
			projectID, stream, itemName)
		if err != nil {
			return fmt.Errorf("load source_id mapping: %w", err)
		}
		for rows.Next() {
			var srcID, intID string
			if err := rows.Scan(&srcID, &intID); err != nil {
				rows.Close()
				return fmt.Errorf("scan source_id mapping: %w", err)
			}
			if srcID != "" {
				existingSourceIDs[srcID] = intID
			}
			internalSourceIDs[intID] = srcID
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("source_id mapping rows: %w", err)
		}

		// Adopt legacy item-less rows: the historical connector-ingest path
		// stored fetched blocks with item_name='' under their raw format-reader
		// IDs. When this item stores a reader ID it does not already map, claim
		// the orphaned row for the item (stamping item_name + source_id) instead
		// of minting a duplicate internal ID — the row, its targets, and its
		// history heal in place, and the adopted row is picked up by the hash
		// query below like any other item-scoped block.
		adopt, err := tx.PrepareContext(ctx,
			`UPDATE blocks SET item_name=$1, source_id=$2 WHERE project_id=$3 AND stream=$4 AND id=$5 AND item_name=''`)
		if err != nil {
			return fmt.Errorf("prepare legacy adoption: %w", err)
		}
		for _, b := range blocks {
			srcKey := convergence.BlockKey(b)
			if _, mapped := existingSourceIDs[srcKey]; mapped {
				continue
			}
			if _, isInternal := internalSourceIDs[b.ID]; isInternal {
				continue // already a row of this item; nothing to adopt
			}
			// The row is found by its internal id; the source_id it adopts is
			// the caller's durable key, not that id.
			res, err := adopt.ExecContext(ctx, itemName, srcKey, projectID, stream, b.ID)
			if err != nil {
				adopt.Close()
				return fmt.Errorf("adopt legacy block %s: %w", b.ID, err)
			}
			if n, _ := res.RowsAffected(); n > 0 {
				existingSourceIDs[srcKey] = b.ID
			}
		}
		adopt.Close()
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO blocks (id, project_id, stream, item_name, item_id, source_id, name, type, mime_type, translatable,
			content_hash, context_hash, source_json, properties, overlays, word_count, stored_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 ON CONFLICT(project_id, stream, id) DO UPDATE SET
			-- An item-less write (StoreBlocks, the editor saving one target)
			-- carries no item, and must not be read as the block having lost the
			-- one it has. Blank means unsaid, exactly as it does one level up in
			-- keepUndeclaredProperties.
			item_name=CASE WHEN EXCLUDED.item_name <> '' THEN EXCLUDED.item_name ELSE blocks.item_name END,
			item_id=CASE WHEN EXCLUDED.item_id <> '' THEN EXCLUDED.item_id ELSE blocks.item_id END,
			name=EXCLUDED.name, type=EXCLUDED.type, mime_type=EXCLUDED.mime_type,
			translatable=EXCLUDED.translatable, content_hash=EXCLUDED.content_hash,
			context_hash=EXCLUDED.context_hash, source_json=EXCLUDED.source_json,
			word_count=EXCLUDED.word_count,
			properties=EXCLUDED.properties, overlays=EXCLUDED.overlays, updated_at=EXCLUDED.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	// Batch-load existing block source hashes + prior target locales
	// for change-log diffing. Targets live in the translations table
	// (#403/#405); we pull their locales here so logChange can
	// distinguish target_added vs target_modified on upsert.
	type existingBlock struct {
		contentHash string
		locales     map[string]struct{}
	}
	existingBlocks := map[string]existingBlock{}
	oldTargetText := map[string]map[string]string{} // blockID → variant → prior text, for block_history
	{
		// Everything below is read back through each incoming block's resolved
		// row id, so the prefetch is bounded to that id set. The source-id
		// mapping is final by this point, which is what makes the set
		// computable up front. It used to load the whole item — or, with no
		// item scope, the whole project, which fed 75k ids into one IN(...)
		// and blew Postgres's 65535-bind-parameter limit the first time a
		// translation job ran against a real corpus.
		var ids []string
		seen := map[string]struct{}{}
		for _, b := range blocks {
			rid := b.ID
			if itemName != "" {
				if _, isInternal := internalSourceIDs[b.ID]; !isInternal {
					if mapped, found := existingSourceIDs[convergence.BlockKey(b)]; found {
						rid = mapped
					}
				}
			}
			if _, dup := seen[rid]; !dup {
				seen[rid] = struct{}{}
				ids = append(ids, rid)
			}
		}

		const prefetchChunk = 5000
		for start := 0; start < len(ids); start += prefetchChunk {
			chunk := ids[start:min(start+prefetchChunk, len(ids))]

			// The concatenated fragment is "$2,$3,…" from a count — never
			// caller data; values travel as bind parameters below.
			hashQuery := `SELECT id, content_hash FROM blocks WHERE project_id=$1 AND stream=$2 AND id IN (` +
				placeholderList("pg", 3, len(chunk)) + `)`
			hashRows, err := tx.QueryContext(ctx, hashQuery, append([]any{projectID, stream}, anyStrings(chunk)...)...)
			if err != nil {
				return fmt.Errorf("batch hash lookup: %w", err)
			}
			var present []string
			for hashRows.Next() {
				var bid, ch string
				if err := hashRows.Scan(&bid, &ch); err != nil {
					hashRows.Close()
					return fmt.Errorf("scan hash: %w", err)
				}
				existingBlocks[bid] = existingBlock{contentHash: ch, locales: map[string]struct{}{}}
				present = append(present, bid)
			}
			// A truncated read here silently drops existing blocks from the
			// prefetch, so they get relabeled source_added — losing approvals and
			// target history on a source change.
			if err := hashRows.Err(); err != nil {
				hashRows.Close()
				return fmt.Errorf("batch hash lookup rows: %w", err)
			}
			hashRows.Close()
			if len(present) == 0 {
				continue
			}

			// Load existing locales per block for target diff.
			localeMap, err := LoadBlockTargetLocales(ctx, tx, "pg", projectID, stream, present)
			if err != nil {
				return fmt.Errorf("batch locale lookup: %w", err)
			}
			for bid, locs := range localeMap {
				if eb, ok := existingBlocks[bid]; ok {
					for _, l := range locs {
						eb.locales[l] = struct{}{}
					}
					existingBlocks[bid] = eb
				}
			}

			// Capture prior target text for change-history before the upsert
			// overwrites it.
			ot, otErr := loadOldTargetText(ctx, tx, projectID, stream, present)
			if otErr != nil {
				return fmt.Errorf("batch old-target load: %w", otErr)
			}
			maps.Copy(oldTargetText, ot)
		}
	}

	now := time.Now().UTC()
	for _, b := range blocks {
		sourceID := ""
		internalID := b.ID

		if itemName != "" {
			// An ID this item's rows already carry is that row — the block was
			// read out of this store and is being written back. Checked before
			// the caller-ID map because naming a row directly is the stronger
			// claim; the two only ever collide on rows this bug already
			// created, where resolving to the original row is also the repair.
			if existingSource, isInternal := internalSourceIDs[b.ID]; isInternal {
				internalID = b.ID
				sourceID = existingSource
			} else {
				// The caller's DURABLE identity, not its reader id. A reader
				// numbers blocks as it goes for formats with no natural key, so
				// keying rows on that id meant deleting one paragraph renumbered
				// every block below it and each stored row rebound to its
				// neighbour's text — carrying that row's history, decisions and
				// translation onto content that was never reviewed.
				//
				// convergence.BlockKey resolves to the structural name a reader
				// assigns (core/model/structural.go), which moves only when the
				// document's structure does. The block's own ID still rides the
				// wire untouched, so the round trip is unaffected.
				sourceID = convergence.BlockKey(b)
				if existingID, found := existingSourceIDs[sourceID]; found {
					internalID = existingID
				} else {
					internalID = storeutil.NewBlockID()
					existingSourceIDs[sourceID] = internalID
					internalSourceIDs[internalID] = sourceID
				}
			}
			b.ID = internalID
		}

		identity := model.ComputeIdentity(b)

		existing, isExisting := existingBlocks[internalID]
		isNew := !isExisting
		existingHash := existing.contentHash
		_ = existingHash // used in change detection below

		sourceJSON, err := json.Marshal(b.Source)
		if err != nil {
			return fmt.Errorf("marshal source for block %s: %w", internalID, err)
		}
		// Fold the source-authoring status into properties: the store has no
		// SourceStatus column, so the source-first gate's stamp must ride here to
		// survive the round-trip (bowrain/core/store.PropsForStore).
		propsJSON, err := json.Marshal(platstore.PropsForStore(b))
		if err != nil {
			return fmt.Errorf("marshal properties for block %s: %w", internalID, err)
		}
		// Stand-off overlays (segmentation, term, entity, term-candidate, qa,
		// alignment) ride alongside source_json so they survive the round-trip —
		// without this they would be dropped and the entity→concept promote path
		// would break on the next GetBlocks. Nil/empty overlays encode to "[]".
		overlaysJSON, err := MarshalOverlays(b.Overlays)
		if err != nil {
			return fmt.Errorf("marshal overlays for block %s: %w", internalID, err)
		}

		_, err = stmt.ExecContext(ctx,
			internalID, projectID, stream, itemName, itemID, sourceID, b.Name, b.Type, b.MimeType, b.Translatable,
			identity.ContentHash, identity.ContextHash,
			string(sourceJSON), string(propsJSON), string(overlaysJSON),
			model.CountWordsInRunsJSON(string(sourceJSON)), now, now)
		if err != nil {
			return fmt.Errorf("store block %s: %w", internalID, err)
		}

		// Write targets + annotations into the kind-specific tables.
		if err := SyncBlockOverlays(ctx, tx, "pg", projectID, stream, internalID, b.Targets, b.AnnoMap(), now); err != nil {
			return err
		}

		// Record content history for changed targets so prior content can be
		// restored (per-edit rollback). Uses the pre-upsert text captured above.
		if err := recordTargetHistoryPg(ctx, tx, projectID, stream, internalID, oldTargetText[internalID], b.Targets, now); err != nil {
			return err
		}

		if isNew {
			if err := logChange(ctx, tx, projectID, stream, internalID, "source_added", "", identity.ContentHash); err != nil {
				return fmt.Errorf("log change for block %s: %w", internalID, err)
			}
			for key := range b.Targets {
				variant := VariantKeyText(key)
				if err := logChange(ctx, tx, projectID, stream, internalID, "target_added", variant, ""); err != nil {
					return fmt.Errorf("log target change for block %s variant %s: %w", internalID, variant, err)
				}
			}
		} else {
			if existingHash != identity.ContentHash {
				if err := logChange(ctx, tx, projectID, stream, internalID, "source_modified", "", identity.ContentHash); err != nil {
					return fmt.Errorf("log change for block %s: %w", internalID, err)
				}
				// The source half of every pairing this unit's decisions blessed
				// has moved, so the projections are re-derived against the
				// ledger on EVERY stream — the source row is stream-global. An
				// approval made for the old wording drops to the presence
				// baseline; a decision that blessed exactly the wording the
				// source now carries applies again. The decisions themselves
				// stay: they are facts about a text, and that history is what
				// lets a restored text find its approval without a re-review.
				if err := settleDecisionProjectionsPg(ctx, tx, projectID, stream, internalID, identity.ContentHash); err != nil {
					return err
				}
			}
			for key := range b.Targets {
				variant := VariantKeyText(key)
				if _, had := existing.locales[variant]; had {
					if err := logChange(ctx, tx, projectID, stream, internalID, "target_modified", variant, ""); err != nil {
						return fmt.Errorf("log target change for block %s variant %s: %w", internalID, variant, err)
					}
				} else {
					if err := logChange(ctx, tx, projectID, stream, internalID, "target_added", variant, ""); err != nil {
						return fmt.Errorf("log target change for block %s variant %s: %w", internalID, variant, err)
					}
				}
			}
		}
	}

	return nil
}

func (s *PostgresStore) GetBlock(ctx context.Context, projectID, stream, blockID string) (*venue.StoredBlock, error) {
	stream = storeutil.DefaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, item_name, source_id, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, properties, overlays, stored_at, updated_at
		 FROM blocks WHERE project_id=$1 AND stream=$2 AND id=$3`, projectID, stream, blockID)
	sb, err := scanStoredBlockPg(row)
	if err != nil {
		return nil, fmt.Errorf("block %s not found in project %s stream %s", blockID, projectID, stream)
	}
	if err := HydrateOverlays(ctx, s.db.DB, "pg", projectID, stream, []*venue.StoredBlock{sb}); err != nil {
		return nil, err
	}
	return sb, nil
}

// blockFilterPg is the FROM/WHERE half of a BlockQuery: the block predicates,
// plus the per-locale translations join the status and target-text filters
// read. GetBlocks and CountBlocks share it so a filtered page and its counts
// can never disagree about what matches.
type blockFilterPg struct {
	join  string // "" when the query needs no per-locale row
	where string
	args  []any
}

// pgStatusBucket is the SQL twin of the editor's getBlockStatus: the bucket a
// block's target for the joined locale falls into. Both must agree, or the
// server's counts contradict the chip the editor renders beside them.
const pgStatusBucket = `CASE
	WHEN COALESCE(t.target_json->>'status', '') IN ('reviewed', 'signed-off') THEN 'reviewed'
	WHEN COALESCE(t.text, '') = '' THEN 'not-started'
	WHEN t.target_json->>'status' = 'translated' THEN 'translated'
	WHEN t.target_json->>'status' = 'draft' THEN 'draft'
	WHEN b.properties::jsonb->>'translation-status' = 'reviewed' THEN 'reviewed'
	WHEN b.properties::jsonb->>'translation-status' = 'draft' THEN 'draft'
	WHEN b.properties::jsonb->>'translation-origin' IN ('machine', 'pseudo') THEN 'draft'
	ELSE 'translated'
END`

// pgSourceTextMatch tests the substring against each source run's text
// separately. Text runs serialize flat — {"text":"literal"} — so the run's
// text is at ->>'text'; every other run kind nests under its own
// discriminator and yields NULL there. A run array is the only shape
// source_json ever holds, but a nil run slice marshals to JSON null, which
// jsonb_array_elements rejects — hence the type guard rather than a bare cast.
const pgSourceTextMatch = `EXISTS (
	SELECT 1 FROM jsonb_array_elements(
		CASE WHEN jsonb_typeof(b.source_json::jsonb) = 'array' THEN b.source_json::jsonb ELSE '[]'::jsonb END
	) r WHERE strpos(lower(COALESCE(r->>'text', '')), lower(%s)) > 0
)`

// pgBlockFilter renders a BlockQuery into bound SQL. withStatus is false for
// the counts query, whose histogram would otherwise be filtered to one bucket.
func pgBlockFilter(query platstore.BlockQuery, withStatus bool) blockFilterPg {
	where := []string{"b.project_id = $1"}
	args := []any{query.ProjectID}
	paramN := 2
	bind := func(v any) string {
		args = append(args, v)
		p := fmt.Sprintf("$%d", paramN)
		paramN++
		return p
	}

	// A block belongs to one stream. Unfiltered, every query would read its
	// branches' rows alongside its own — the same block id exists on each — so
	// the scope is applied here rather than left to each caller to remember.
	where = append(where, "b.stream = "+bind(storeutil.DefaultStream(query.Stream)))

	if query.ItemName != "" {
		where = append(where, "b.item_name = "+bind(query.ItemName))
	}
	if len(query.IDs) > 0 {
		var pb strings.Builder
		pb.WriteString("b.id IN (")
		for i, id := range query.IDs {
			if i > 0 {
				pb.WriteByte(',')
			}
			pb.WriteString(bind(id))
		}
		pb.WriteByte(')')
		where = append(where, pb.String())
	}
	if query.ContentHash != "" {
		where = append(where, "b.content_hash = "+bind(query.ContentHash))
	}
	// The keyset cursor. Rows come back in id order, so "after this id" is the
	// next page — see BlockQuery.AfterID.
	if query.AfterID != "" {
		where = append(where, "b.id > "+bind(query.AfterID))
	}
	// The cursor pointing the other way — see BlockQuery.BeforeID. The rows are
	// selected in descending id order so a Limit takes the NEAREST predecessors;
	// GetBlocks puts them back in ascending order.
	if query.BeforeID != "" {
		where = append(where, "b.id < "+bind(query.BeforeID))
	}
	if query.Translatable != nil {
		where = append(where, "b.translatable = "+bind(*query.Translatable))
	}

	join := ""
	if query.TargetLocale != "" {
		stream := bind(storeutil.DefaultStream(query.Stream))
		locale := bind(query.TargetLocale)
		join = fmt.Sprintf(`LEFT JOIN translations t
			ON t.project_id = b.project_id AND t.block_id = b.id AND t.stream = %s AND t.locale = %s`, stream, locale)
	}
	if query.Text != "" {
		// One placeholder, referenced from both sides of the OR.
		p := bind(query.Text)
		match := fmt.Sprintf(pgSourceTextMatch, p)
		if join != "" {
			match = fmt.Sprintf("(%s OR strpos(lower(COALESCE(t.text, '')), lower(%s)) > 0)", match, p)
		}
		where = append(where, match)
	}
	if withStatus && query.Status != "" && join != "" {
		where = append(where, pgStatusBucket+" = "+bind(query.Status))
	}

	return blockFilterPg{join: join, where: strings.Join(where, " AND "), args: args}
}

// Instrumented because this is the operation class that has twice been the
// answer: the 22-second dashboard (#2105) and the read that OOM-killed the
// server. A transaction that says a request took 22 seconds restates the
// request log; a span saying this ran forty times is the finding.

// blockScope describes what a block query was scoped BY, never what it was
// scoped to. A span description is a label Sentry groups on, so putting a
// project id in it would mint a separate row per project and group nothing —
// the same reason a transaction is named by its route and not its URL. What is
// worth seeing is the shape: whether a read was bounded to one item, to a page,
// or to everything.
func blockScope(q platstore.BlockQuery) string {
	switch {
	case len(q.IDs) > 0:
		return "by-id"
	case q.ItemName != "":
		return "item"
	case q.AfterID != "":
		return "page"
	case q.Limit > 0:
		return "limited"
	default:
		return "whole-stream"
	}
}

func (s *PostgresStore) GetBlocks(ctx context.Context, query platstore.BlockQuery) ([]*venue.StoredBlock, error) {
	defer observe.StartSpan(ctx, "db.query", "store.GetBlocks "+blockScope(query))()

	f := pgBlockFilter(query, true)

	// Constant skeleton + rendered fragments that carry $N placeholders only;
	// every value binds through args. Limit and offset are ints, formatted.
	const skeleton = `SELECT b.id, b.project_id, b.item_name, b.source_id, b.name, b.type, b.mime_type, b.translatable,
			b.content_hash, b.context_hash, b.source_json, b.properties, b.overlays, b.stored_at, b.updated_at
		 FROM blocks b %s WHERE %s ORDER BY b.id%s%s`
	order := ""
	if query.BeforeID != "" {
		order = " DESC"
	}
	page := ""
	if query.Limit > 0 {
		page += fmt.Sprintf(" LIMIT %d", query.Limit)
	}
	if query.Offset > 0 {
		page += fmt.Sprintf(" OFFSET %d", query.Offset)
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(skeleton, f.join, f.where, order, page), f.args...)
	if err != nil {
		return nil, fmt.Errorf("query blocks: %w", err)
	}
	defer rows.Close()

	var result []*venue.StoredBlock
	for rows.Next() {
		sb, err := scanStoredBlockPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sb)
	}
	if order != "" {
		slices.Reverse(result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := HydrateOverlays(ctx, s.db.DB, "pg", query.ProjectID, storeutil.DefaultStream(query.Stream), result); err != nil {
		return nil, err
	}
	return result, nil
}

// CountBlocks answers the editor's progress bar with one aggregate instead of
// a full page of hydrated blocks.
func (s *PostgresStore) CountBlocks(ctx context.Context, query platstore.BlockQuery) (platstore.BlockCounts, error) {
	defer observe.StartSpan(ctx, "db.query", "store.CountBlocks "+blockScope(query))()

	f := pgBlockFilter(query, false)
	bucket := "''"
	if f.join != "" {
		bucket = pgStatusBucket
	}

	const skeleton = `SELECT count(*),
			count(*) FILTER (WHERE x.translatable),
			count(*) FILTER (WHERE x.translatable AND x.bucket = 'not-started'),
			count(*) FILTER (WHERE x.translatable AND x.bucket = 'draft'),
			count(*) FILTER (WHERE x.translatable AND x.bucket = 'translated'),
			count(*) FILTER (WHERE x.translatable AND x.bucket = 'reviewed')
		 FROM (SELECT b.translatable, %s AS bucket FROM blocks b %s WHERE %s) x`

	var out platstore.BlockCounts
	err := s.db.QueryRowContext(ctx, fmt.Sprintf(skeleton, bucket, f.join, f.where), f.args...).
		Scan(&out.Total, &out.Translatable, &out.NotStarted, &out.Draft, &out.Translated, &out.Reviewed)
	if err != nil {
		return platstore.BlockCounts{}, fmt.Errorf("count blocks: %w", err)
	}
	return out, nil
}

// ListPendingReview pages the (block, locale) pairs whose stored target still
// needs a review decision. One indexed join answers what the review session
// once assembled with a blocks fetch per item — minutes of "gathering" at
// dogfood scale (978 items), and only ever the first dashboard page of it.
func (s *PostgresStore) ListPendingReview(ctx context.Context, q platstore.PendingReviewQuery) ([]platstore.PendingReviewRef, int, error) {
	stream := storeutil.DefaultStream(q.Stream)
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	// Constant skeleton + $N placeholder tokens only; every value binds
	// through args (the G201-triage contract this package's query builders
	// share).
	//
	// The status test is the review-completion gate's predicate (see
	// server/review_loop.go targetPendingReview): anything below reviewed is
	// pending — including an EMPTY status, which an IN ('draft','translated')
	// spelling once excluded, leaving a block that stalls completion forever
	// invisible in the queue. Known narrower edge: a code-only target
	// (RunsHaveContent true, flattened text empty) is pending for the gate
	// but skipped here by text <> ''.
	//
	// The items join carries each block's collection, and is LEFT so a block
	// whose item has no row for the stream stays in the queue (as ungrouped)
	// rather than disappearing from it. The filter tests the same COALESCE the
	// projection returns, so what a caller filters by is what it reads back.
	const skeleton = `SELECT %s FROM blocks b JOIN translations t
		ON t.project_id = b.project_id AND t.block_id = b.id AND t.stream = $2
		LEFT JOIN items i
		ON i.project_id = b.project_id AND i.stream = $2 AND i.name = b.item_name
		WHERE b.project_id = $1 AND b.stream = $2 AND b.translatable AND t.text <> ''
		AND COALESCE(t.target_json->>'status', '') NOT IN ('reviewed', 'signed-off')%s`
	args := []any{q.ProjectID, stream}
	scope := ""
	next := 3
	if len(q.Locales) > 0 {
		scope = fmt.Sprintf(` AND t.locale = ANY($%d)`, next)
		args = append(args, q.Locales)
		next++
	}
	if q.CollectionID != nil {
		scope += fmt.Sprintf(` AND COALESCE(i.collection_id, '') = $%d`, next)
		args = append(args, *q.CollectionID)
		next++
	}

	var total int
	countQuery := fmt.Sprintf(skeleton, "count(*)", scope)
	if err := s.db.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count pending review: %w", err)
	}

	query := fmt.Sprintf(skeleton+` ORDER BY b.item_name, b.id, t.locale LIMIT $%d OFFSET $%d`,
		"b.id, b.item_name, t.locale, COALESCE(i.collection_id, '')", scope, next, next+1)
	rows, err := s.db.DB.QueryContext(ctx, query, append(args, limit, q.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list pending review: %w", err)
	}
	defer rows.Close()
	var refs []platstore.PendingReviewRef
	for rows.Next() {
		var r platstore.PendingReviewRef
		if err := rows.Scan(&r.BlockID, &r.ItemName, &r.Locale, &r.CollectionID); err != nil {
			return nil, 0, fmt.Errorf("scan pending review: %w", err)
		}
		refs = append(refs, r)
	}
	return refs, total, rows.Err()
}

func (s *PostgresStore) GetBlockStats(ctx context.Context, projectID, stream string) ([]platstore.BlockStatRow, error) {
	defer observe.StartSpan(ctx, "db.query", "store.GetBlockStats")()

	stream = storeutil.DefaultStream(stream)

	// Get item names for the stream to scope the query.
	items, err := s.ListItems(ctx, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	// Build IN clause for item names.
	placeholders := make([]string, len(items))
	args := []any{projectID, stream}
	for i, item := range items {
		placeholders[i] = fmt.Sprintf("$%d", i+3)
		args = append(args, item.Name)
	}

	// word_count is written at store time; NULL marks a row that predates the
	// column, and only those rows pay the source_json decode. Deriving
	// coverage used to deserialize every block's source runs on every call —
	// at 74,916 blocks, minutes of JSON for numbers the write path already
	// knew.
	q := fmt.Sprintf(
		`SELECT id, item_name, translatable,
			CASE WHEN word_count IS NULL THEN source_json ELSE '' END,
			COALESCE(word_count, -1)
		 FROM blocks WHERE project_id = $1 AND stream = $2 AND item_name IN (%s)
		 ORDER BY item_name, id`,
		strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query block stats: %w", err)
	}
	defer rows.Close()

	type pending struct {
		blockID      string
		itemName     string
		translatable bool
		sourceWords  int
	}
	var ordered []pending
	var blockIDs []string
	for rows.Next() {
		var blockID, itemName, sourceJSON string
		var translatable bool
		var wordCount int
		if err := rows.Scan(&blockID, &itemName, &translatable, &sourceJSON, &wordCount); err != nil {
			return nil, fmt.Errorf("scan block stat: %w", err)
		}
		if wordCount < 0 {
			wordCount = model.CountWordsInRunsJSON(sourceJSON)
		}
		ordered = append(ordered, pending{
			blockID: blockID, itemName: itemName, translatable: translatable,
			sourceWords: wordCount,
		})
		blockIDs = append(blockIDs, blockID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	states, err := LoadBlockTargetStates(ctx, s.db.DB, "pg", projectID, stream, blockIDs)
	if err != nil {
		return nil, err
	}
	var result []platstore.BlockStatRow
	for _, p := range ordered {
		locales, approved := SplitTargetStates(states[p.blockID])
		result = append(result, platstore.BlockStatRow{
			ItemName:        p.itemName,
			Translatable:    p.translatable,
			SourceWords:     p.sourceWords,
			TargetLocales:   locales,
			ApprovedLocales: approved,
		})
	}
	return result, rows.Err()
}

func (s *PostgresStore) DeleteBlock(ctx context.Context, projectID, stream, blockID string) error {
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The decision ledger keys on the block's unit identity (source_id), not its
	// id, so read the scope before the row is gone.
	var itemName, sourceID, itemID string
	err = tx.QueryRowContext(ctx,
		`SELECT item_name, source_id, item_id FROM blocks WHERE project_id=$1 AND stream=$2 AND id=$3`,
		projectID, stream, blockID).Scan(&itemName, &sourceID, &itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("block %s not found in project %s stream %s", blockID, projectID, stream)
	}
	if err != nil {
		return fmt.Errorf("look up block %s: %w", blockID, err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM blocks WHERE project_id=$1 AND stream=$2 AND id=$3`,
		projectID, stream, blockID); err != nil {
		return fmt.Errorf("delete block: %w", err)
	}

	// Everything filed under the block's id on THIS stream goes with it — a
	// branch holding the same id keeps its own rows, which is the whole point
	// of a branch.
	for _, table := range storeutil.BlockScopedTables() {
		//nolint:gosec // table is a fixed literal from storeutil, never user input
		q := `DELETE FROM ` + table + ` WHERE project_id=$1 AND stream=$2 AND block_id=$3`
		if _, err := tx.ExecContext(ctx, q, projectID, stream, blockID); err != nil {
			return fmt.Errorf("delete %s for block %s: %w", table, blockID, err)
		}
	}
	if sourceID != "" {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM unit_decisions WHERE project_id=$1 AND stream=$2 AND item_id=$3 AND unit=$4`,
			projectID, stream, itemID, sourceID); err != nil {
			return fmt.Errorf("delete unit decisions for block %s: %w", blockID, err)
		}
	}

	if err := logChange(ctx, tx, projectID, stream, blockID, "source_removed", "", ""); err != nil {
		return fmt.Errorf("log change for deleted block %s: %w", blockID, err)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Version management
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateVersion(ctx context.Context, projectID, stream, label, description string) (*platstore.Version, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	versionID := id.New()
	now := time.Now().UTC()
	// A version is a point in ONE stream's history — the parameter has always
	// said so, and the two statements below used to ignore it and snapshot the
	// whole project. With a branch holding its own rows under the same block
	// ids, that would count every branch's copy into main's version.
	stream = storeutil.DefaultStream(stream)

	var blockCount int
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM blocks WHERE project_id=$1 AND stream=$2`,
		projectID, stream).Scan(&blockCount)
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
		 SELECT $1, id, content_hash FROM blocks WHERE project_id=$2 AND stream=$3`,
		versionID, projectID, stream)
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

func (s *PostgresStore) ListVersions(ctx context.Context, projectID, stream string) ([]*platstore.Version, error) {
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

func scanProject(row scanner) (*platstore.Project, error) {
	var p platstore.Project
	var srcLocale, targetLocales, propsJSON string
	err := row.Scan(&p.ID, &p.Name, &srcLocale, &targetLocales, &p.TargetLanguageMode, &p.DefaultStream, &p.DashboardVisibility, &propsJSON, &p.WorkspaceID,
		&p.ConvergePolicy, &p.Archived, &p.ArchivedAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan project: %w", err)
	}
	p.DefaultSourceLanguage = model.LocaleID(srcLocale)
	p.TargetLanguages = storeutil.SplitLocales(targetLocales)
	if p.DashboardVisibility == "" {
		p.DashboardVisibility = "private"
	}
	if err := json.Unmarshal([]byte(propsJSON), &p.Properties); err != nil {
		p.Properties = map[string]string{}
	}
	return &p, nil
}

func scanItemPg(row scanner) (*platstore.Item, error) {
	var item platstore.Item
	var propsJSON string
	err := row.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Format, &item.ItemType,
		&item.BlockIndex, &item.PreviewHTML, &propsJSON, &item.CollectionID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan item: %w", err)
	}
	if err := json.Unmarshal([]byte(propsJSON), &item.Properties); err != nil {
		item.Properties = map[string]string{}
	}
	return &item, nil
}

func scanStoredBlockPg(row scanner) (*venue.StoredBlock, error) {
	var sb venue.StoredBlock
	sb.Block = &model.Block{}
	var sourceJSON, propsJSON, overlaysJSON string

	err := row.Scan(
		&sb.Block.ID, &sb.ProjectID, &sb.ItemName, &sb.SourceID, &sb.Block.Name, &sb.Block.Type,
		&sb.Block.MimeType, &sb.Block.Translatable, &sb.ContentHash, &sb.ContextHash,
		&sourceJSON, &propsJSON, &overlaysJSON, &sb.StoredAt, &sb.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan block: %w", err)
	}

	if err := json.Unmarshal([]byte(sourceJSON), &sb.Block.Source); err != nil {
		sb.Block.Source = nil
	}
	if err := json.Unmarshal([]byte(propsJSON), &sb.Block.Properties); err != nil {
		sb.Block.Properties = make(map[string]string)
	}
	// Rehydrate stand-off overlays from the sibling column (symmetric with the
	// MarshalOverlays write above).
	if overlays, err := UnmarshalOverlays([]byte(overlaysJSON)); err == nil {
		sb.Block.Overlays = overlays
	}
	// Lift the folded source-authoring status back onto the block (source-first
	// gate). Symmetric with PropsForStore on the write side.
	platstore.ApplySourceStatusFromProps(sb.Block)
	// The stored source id IS the block's durable unit — it is what a decision,
	// a translation and a history entry are filed under here. Reading it back
	// onto the block is what makes a pull round-trip the identity rather than
	// hand back a block whose key would be re-derived from its name.
	sb.Block.Unit = sb.SourceID
	// Targets + Annotations are hydrated separately via hydrateOverlays
	// after all rows are scanned — see GetBlock / GetBlocks. Leave
	// empty here.
	sb.Block.Targets = make(map[model.VariantKey]*model.Target)
	return &sb, nil
}

// hydrateOverlays populates Targets + Annotations on the supplied
// blocks from the kind-specific tables. Single round trip per table,
// not per-block. Safe for empty input.
func HydrateOverlays(
	ctx context.Context,
	db Querier,
	dialect string,
	projectID, stream string,
	blocks []*venue.StoredBlock,
) error {
	if len(blocks) == 0 {
		return nil
	}
	ids := make([]string, 0, len(blocks))
	byID := make(map[string]*venue.StoredBlock, len(blocks))
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil {
			continue
		}
		ids = append(ids, sb.Block.ID)
		byID[sb.Block.ID] = sb
	}
	// Chunked: every id lands in one IN(...) placeholder, and a whole-project
	// hydrate (the review loop, an unscoped GetBlocks) at corpus scale blew
	// Postgres's 65,535-bind-parameter limit. Same bound as the storeBlocks
	// prefetch, kept far below the limit for SQLite's sake too.
	const hydrateChunk = 5000
	for start := 0; start < len(ids); start += hydrateChunk {
		chunk := ids[start:min(start+hydrateChunk, len(ids))]
		targets, annotations, err := LoadBlockOverlays(ctx, db, dialect, projectID, stream, chunk)
		if err != nil {
			return fmt.Errorf("hydrate overlays: %w", err)
		}
		for id, locs := range targets {
			if sb := byID[id]; sb != nil {
				sb.Block.Targets = locs
			}
		}
		for id, anns := range annotations {
			if sb := byID[id]; sb != nil {
				for k, v := range anns {
					sb.Block.SetAnno(k, v)
				}
			}
		}
	}
	return nil
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

// logChange inserts a single change log entry within a PostgreSQL transaction.
func logChange(ctx context.Context, tx Runner, projectID, stream, blockID, changeType, locale, contentHash string) error {
	stream = storeutil.DefaultStream(stream)
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
		`INSERT INTO change_log (project_id, stream, block_id, change_type, locale, content_hash, correlation_id, logged_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		projectID, stream, blockID, changeType, localeVal, hashVal, ChangeContextFromContext(ctx).CorrelationID, now)
	return err
}

// ---------------------------------------------------------------------------
// Asset CRUD (Bowrain AD-007)
// ---------------------------------------------------------------------------

func (s *PostgresStore) StoreAsset(ctx context.Context, projectID, stream string, asset *venue.Asset) error {
	stream = storeutil.DefaultStream(stream)
	if asset.ID == "" {
		asset.ID = id.New()
	}
	now := time.Now().UTC()
	asset.ProjectID = projectID
	asset.Stream = stream
	asset.CreatedAt = now
	asset.UpdatedAt = now

	if asset.ProcessingStatus == "" {
		asset.ProcessingStatus = "none"
	}

	propsJSON, err := json.Marshal(asset.Properties)
	if err != nil {
		return fmt.Errorf("marshal asset properties: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingID string
	_ = tx.QueryRowContext(ctx,
		`SELECT id FROM assets WHERE project_id=$1 AND blob_key=$2 AND stream=$3`,
		projectID, asset.BlobKey, stream).Scan(&existingID)
	isNew := existingID == ""

	_, err = tx.ExecContext(ctx,
		`INSERT INTO assets (id, project_id, item_name, source_id, blob_key, mime_type, filename,
			size_bytes, alt_text, properties, processing_status, processing_hint, stream, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT (project_id, blob_key) WHERE stream = 'main' DO UPDATE SET
			item_name=EXCLUDED.item_name, source_id=EXCLUDED.source_id, mime_type=EXCLUDED.mime_type,
			filename=EXCLUDED.filename, size_bytes=EXCLUDED.size_bytes, alt_text=EXCLUDED.alt_text,
			properties=EXCLUDED.properties, processing_status=EXCLUDED.processing_status,
			processing_hint=EXCLUDED.processing_hint, updated_at=EXCLUDED.updated_at`,
		asset.ID, projectID, asset.ItemName, asset.SourceID, asset.BlobKey, asset.MimeType,
		asset.Filename, asset.SizeBytes, asset.AltText, string(propsJSON),
		asset.ProcessingStatus, asset.ProcessingHint, stream, now, now)
	if err != nil {
		return fmt.Errorf("store asset: %w", err)
	}

	changeType := "asset_modified"
	if isNew {
		changeType = "asset_added"
	}
	assetID := asset.ID
	if existingID != "" {
		assetID = existingID
	}
	if err := logChange(ctx, tx, projectID, stream, assetID, changeType, "", asset.BlobKey); err != nil {
		return fmt.Errorf("log asset change: %w", err)
	}

	return tx.Commit()
}

func (s *PostgresStore) GetAsset(ctx context.Context, projectID, stream, assetID string) (*venue.Asset, error) {
	stream = storeutil.DefaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, item_name, source_id, blob_key, mime_type, filename,
			size_bytes, alt_text, properties, processing_status, processing_hint, stream, created_at, updated_at
		 FROM assets WHERE project_id=$1 AND stream=$2 AND id=$3`, projectID, stream, assetID)
	return scanAsset(row)
}

func (s *PostgresStore) ListAssets(ctx context.Context, projectID, stream, itemName string) ([]*venue.Asset, error) {
	stream = storeutil.DefaultStream(stream)
	var rows *sql.Rows
	var err error
	if itemName != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, project_id, item_name, source_id, blob_key, mime_type, filename,
				size_bytes, alt_text, properties, processing_status, processing_hint, stream, created_at, updated_at
			 FROM assets WHERE project_id=$1 AND stream=$2 AND item_name=$3 ORDER BY filename`, projectID, stream, itemName)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, project_id, item_name, source_id, blob_key, mime_type, filename,
				size_bytes, alt_text, properties, processing_status, processing_hint, stream, created_at, updated_at
			 FROM assets WHERE project_id=$1 AND stream=$2 ORDER BY filename`, projectID, stream)
	}
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	defer rows.Close()

	var result []*venue.Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (s *PostgresStore) DeleteAsset(ctx context.Context, projectID, stream, assetID string) error {
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`DELETE FROM assets WHERE project_id=$1 AND stream=$2 AND id=$3`, projectID, stream, assetID)
	if err != nil {
		return fmt.Errorf("delete asset: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("asset %q not found", assetID)
	}

	if err := logChange(ctx, tx, projectID, stream, assetID, "asset_removed", "", ""); err != nil {
		return fmt.Errorf("log asset removal: %w", err)
	}

	return tx.Commit()
}

type assetScanner interface {
	Scan(dest ...any) error
}

func scanAsset(row assetScanner) (*venue.Asset, error) {
	var a venue.Asset
	var propsJSON string
	err := row.Scan(&a.ID, &a.ProjectID, &a.ItemName, &a.SourceID, &a.BlobKey, &a.MimeType,
		&a.Filename, &a.SizeBytes, &a.AltText, &propsJSON, &a.ProcessingStatus, &a.ProcessingHint,
		&a.Stream, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan asset: %w", err)
	}
	if err := json.Unmarshal([]byte(propsJSON), &a.Properties); err != nil {
		a.Properties = map[string]string{}
	}
	return &a, nil
}

// ---------------------------------------------------------------------------
// Asset Variants (Bowrain AD-007)
// ---------------------------------------------------------------------------

func (s *PostgresStore) StoreAssetVariant(ctx context.Context, projectID string, variant *venue.AssetVariant) error {
	now := time.Now().UTC()
	variant.CreatedAt = now
	variant.UpdatedAt = now

	if variant.Status == "" {
		variant.Status = "pending"
	}

	propsJSON, err := json.Marshal(variant.Properties)
	if err != nil {
		return fmt.Errorf("marshal variant properties: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingKey string
	_ = tx.QueryRowContext(ctx,
		`SELECT blob_key FROM asset_variants WHERE asset_id=$1 AND locale=$2`,
		variant.AssetID, variant.Locale).Scan(&existingKey)
	isNew := existingKey == ""

	_, err = tx.ExecContext(ctx,
		`INSERT INTO asset_variants (asset_id, locale, blob_key, status, mime_type, size_bytes, properties, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (asset_id, locale) DO UPDATE SET
			blob_key=EXCLUDED.blob_key, status=EXCLUDED.status, mime_type=EXCLUDED.mime_type,
			size_bytes=EXCLUDED.size_bytes, properties=EXCLUDED.properties, updated_at=EXCLUDED.updated_at`,
		variant.AssetID, variant.Locale, variant.BlobKey, variant.Status, variant.MimeType,
		variant.SizeBytes, string(propsJSON), now, now)
	if err != nil {
		return fmt.Errorf("store asset variant: %w", err)
	}

	var assetProjectID, assetStream string
	err = tx.QueryRowContext(ctx,
		`SELECT project_id, stream FROM assets WHERE id=$1`, variant.AssetID).Scan(&assetProjectID, &assetStream)
	if err == nil {
		changeType := "variant_modified"
		if isNew {
			changeType = "variant_added"
		}
		if variant.Status == "approved" && existingKey != "" {
			changeType = "variant_approved"
		}
		if err := logChange(ctx, tx, assetProjectID, assetStream, variant.AssetID, changeType, variant.Locale, variant.BlobKey); err != nil {
			return fmt.Errorf("log change for asset variant %s/%s: %w", variant.AssetID, variant.Locale, err)
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) GetAssetVariant(ctx context.Context, _, assetID, locale string) (*venue.AssetVariant, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT asset_id, locale, blob_key, status, mime_type, size_bytes, properties, created_at, updated_at
		 FROM asset_variants WHERE asset_id=$1 AND locale=$2`, assetID, locale)
	return scanAssetVariant(row)
}

func (s *PostgresStore) ListAssetVariants(ctx context.Context, _, assetID string) ([]*venue.AssetVariant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT asset_id, locale, blob_key, status, mime_type, size_bytes, properties, created_at, updated_at
		 FROM asset_variants WHERE asset_id=$1 ORDER BY locale`, assetID)
	if err != nil {
		return nil, fmt.Errorf("list asset variants: %w", err)
	}
	defer rows.Close()

	var result []*venue.AssetVariant
	for rows.Next() {
		v, err := scanAssetVariant(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func scanAssetVariant(row assetScanner) (*venue.AssetVariant, error) {
	var v venue.AssetVariant
	var propsJSON string
	err := row.Scan(&v.AssetID, &v.Locale, &v.BlobKey, &v.Status, &v.MimeType,
		&v.SizeBytes, &propsJSON, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan asset variant: %w", err)
	}
	if err := json.Unmarshal([]byte(propsJSON), &v.Properties); err != nil {
		v.Properties = map[string]string{}
	}
	return &v, nil
}
