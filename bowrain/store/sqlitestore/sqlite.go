package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// SQLiteStore implements ContentStore using SQLite via the shared storage layer.
type SQLiteStore struct {
	db *storage.DB
}

// NewSQLiteStore opens (or creates) a SQLite-backed ContentStore.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store database: %w", err)
	}
	if err := storage.Migrate(db, storeMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate store schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// DB returns the underlying *sql.DB for sharing with other stores
// that need the same database connection (e.g., AutomationRuleStore).
func (s *SQLiteStore) DB() *sql.DB {
	return s.db.DB
}

// Close closes the underlying database.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ---------------------------------------------------------------------------
// Project CRUD
// ---------------------------------------------------------------------------

func (s *SQLiteStore) CreateProject(ctx context.Context, p *platstore.Project) error {
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
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, string(p.DefaultSourceLanguage), locales, p.TargetLanguageMode, p.DefaultStream, p.DashboardVisibility, string(propsJSON),
		p.WorkspaceID, platstore.NormalizeConvergePolicy(p.ConvergePolicy), now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetProject(ctx context.Context, id string) (*platstore.Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, converge_policy, archived, archived_at, created_at, updated_at
		 FROM projects WHERE id = ?`, id)
	return scanProject(row)
}

func (s *SQLiteStore) ListProjects(ctx context.Context) ([]*platstore.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, converge_policy, archived, archived_at, created_at, updated_at
		 FROM projects WHERE archived=0 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return storage.ScanRows(rows, scanProject)
}

func (s *SQLiteStore) UpdateProject(ctx context.Context, p *platstore.Project) error {
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
		`UPDATE projects SET name=?, default_source_language=?, target_languages=?, target_language_mode=?, default_stream=?, dashboard_visibility=?, properties=?, workspace_id=?, converge_policy=?, updated_at=?
		 WHERE id=?`,
		p.Name, string(p.DefaultSourceLanguage), locales, p.TargetLanguageMode, p.DefaultStream, p.DashboardVisibility, string(propsJSON),
		p.WorkspaceID, platstore.NormalizeConvergePolicy(p.ConvergePolicy), p.UpdatedAt.Format(time.RFC3339), p.ID)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %s not found", p.ID)
	}
	return nil
}

func (s *SQLiteStore) DeleteProject(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id=?`, id)
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
	for _, table := range storeutil.ProjectScopedTablesWithoutCascade() {
		//nolint:gosec // table is a fixed literal from storeutil, never user input
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE project_id=?`, id); err != nil {
			return fmt.Errorf("delete %s for project %s: %w", table, id, err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) ArchiveProject(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET archived=1, archived_at=?, updated_at=? WHERE id=?`, now, now, id)
	if err != nil {
		return fmt.Errorf("archive project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %s not found", id)
	}
	return nil
}

func (s *SQLiteStore) RestoreProject(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET archived=0, archived_at=NULL, updated_at=? WHERE id=?`, now, id)
	if err != nil {
		return fmt.Errorf("restore project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %s not found", id)
	}
	return nil
}

func (s *SQLiteStore) ListArchivedProjects(ctx context.Context, workspaceID string) ([]*platstore.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, converge_policy, archived, archived_at, created_at, updated_at
		 FROM projects WHERE workspace_id=? AND archived=1 ORDER BY archived_at DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list archived projects: %w", err)
	}
	return storage.ScanRows(rows, scanProject)
}

// ---------------------------------------------------------------------------
// Collection management
// ---------------------------------------------------------------------------

func (s *SQLiteStore) CreateCollection(ctx context.Context, c *platstore.Collection) error {
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
	contextJSON, err := json.Marshal(c.Context)
	if err != nil {
		return fmt.Errorf("marshal collection context: %w", err)
	}
	c.Owner = venue.NormalizeContextOwner(c.Owner)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO collections (id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProjectID, c.Name, string(c.Kind), c.ItemLabel, c.IsDefault, c.Stream,
		string(configJSON), string(contextJSON), c.Owner, c.ContextHash,
		c.PreviewKind, c.PreviewURL,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetCollection(ctx context.Context, projectID, collectionID string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at
		 FROM collections WHERE project_id=? AND id=?`, projectID, collectionID)
	return scanCollection(row)
}

func (s *SQLiteStore) GetCollectionByName(ctx context.Context, projectID, name, stream string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at
		 FROM collections WHERE project_id=? AND name=? AND (stream='' OR stream=?)`,
		projectID, name, stream)
	return scanCollection(row)
}

func (s *SQLiteStore) GetDefaultCollection(ctx context.Context, projectID string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at
		 FROM collections WHERE project_id=? AND is_default=1`, projectID)
	return scanCollection(row)
}

func (s *SQLiteStore) ListCollections(ctx context.Context, projectID, stream string) ([]*platstore.Collection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, context, owner, context_hash, preview_kind, preview_url, created_at, updated_at
		 FROM collections WHERE project_id=? AND (stream='' OR stream=?)
		 ORDER BY is_default DESC, name`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	return storage.ScanRows(rows, scanCollection)
}

func (s *SQLiteStore) UpdateCollection(ctx context.Context, c *platstore.Collection) error {
	c.UpdatedAt = time.Now().UTC()

	configJSON, err := json.Marshal(c.ConnectorConfig)
	if err != nil {
		return fmt.Errorf("marshal connector config: %w", err)
	}
	contextJSON, err := json.Marshal(c.Context)
	if err != nil {
		return fmt.Errorf("marshal collection context: %w", err)
	}
	c.Owner = venue.NormalizeContextOwner(c.Owner)

	_, err = s.db.ExecContext(ctx,
		`UPDATE collections SET name=?, kind=?, item_label=?, stream=?, connector_config=?, context=?, owner=?, context_hash=?, preview_kind=?, preview_url=?, updated_at=?
		 WHERE project_id=? AND id=?`,
		c.Name, string(c.Kind), c.ItemLabel, c.Stream, string(configJSON),
		string(contextJSON), c.Owner, c.ContextHash, c.PreviewKind, c.PreviewURL,
		c.UpdatedAt.Format(time.RFC3339), c.ProjectID, c.ID)
	if err != nil {
		return fmt.Errorf("update collection: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteCollection(ctx context.Context, projectID, collectionID string) error {
	// Prevent deleting the default collection.
	var isDefault int
	err := s.db.QueryRowContext(ctx,
		`SELECT is_default FROM collections WHERE project_id=? AND id=?`,
		projectID, collectionID).Scan(&isDefault)
	if err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	if isDefault == 1 {
		return errors.New("cannot delete the default collection")
	}

	// Reassign items from this collection to the default collection.
	var defaultID string
	err = s.db.QueryRowContext(ctx,
		`SELECT id FROM collections WHERE project_id=? AND is_default=1`, projectID).Scan(&defaultID)
	if err != nil {
		return fmt.Errorf("get default collection: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE items SET collection_id=? WHERE project_id=? AND collection_id=?`,
		defaultID, projectID, collectionID)
	if err != nil {
		return fmt.Errorf("reassign items: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`DELETE FROM collections WHERE project_id=? AND id=?`, projectID, collectionID)
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	return nil
}

func scanCollection(row scanner) (*platstore.Collection, error) {
	var c platstore.Collection
	var kindStr, configJSON, contextJSON, createdStr, updatedStr string
	err := row.Scan(&c.ID, &c.ProjectID, &c.Name, &kindStr, &c.ItemLabel,
		&c.IsDefault, &c.Stream, &configJSON, &contextJSON, &c.Owner, &c.ContextHash,
		&c.PreviewKind, &c.PreviewURL, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan collection: %w", err)
	}
	c.Kind = platstore.CollectionKind(kindStr)
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if err := json.Unmarshal([]byte(configJSON), &c.ConnectorConfig); err != nil {
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

func (s *SQLiteStore) StoreItem(ctx context.Context, projectID, stream string, item *platstore.Item) error {
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
	// raw incoming id: resolving the default here instead would hand the upsert
	// a non-empty id and clobber the existing binding (#1840). Mirrors
	// PostgresStore.
	fallbackCollectionID := ""
	if item.CollectionID == "" {
		defColl, defErr := s.GetDefaultCollection(ctx, projectID)
		if defErr == nil && defColl != nil {
			fallbackCollectionID = defColl.ID
		}
	}

	// SQLite binds positionally, so the raw incoming collection id is passed
	// twice — once for the insert's fallback, once for the conflict branch.
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO items (id, project_id, stream, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(?, ''), ?), ?, ?)
		 ON CONFLICT(project_id, stream, name) DO UPDATE SET
			format=excluded.format, item_type=excluded.item_type,
			block_index=excluded.block_index, preview_html=excluded.preview_html,
			properties=excluded.properties,
			collection_id=COALESCE(NULLIF(?, ''), NULLIF(items.collection_id, ''), excluded.collection_id),
			updated_at=excluded.updated_at`,
		item.ID, projectID, stream, item.Name, item.Format, item.ItemType,
		item.BlockIndex, item.PreviewHTML, string(propsJSON),
		item.CollectionID, fallbackCollectionID,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
		item.CollectionID)
	if err != nil {
		return fmt.Errorf("store item %q: %w", item.Name, err)
	}
	return nil
}

func (s *SQLiteStore) GetItem(ctx context.Context, projectID, stream, itemName string) (*platstore.Item, error) {
	stream = storeutil.DefaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at
		 FROM items WHERE project_id=? AND stream=? AND name=?`, projectID, stream, itemName)
	return scanItem(row)
}

func (s *SQLiteStore) ListItems(ctx context.Context, projectID, stream string) ([]*platstore.Item, error) {
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at
		 FROM items WHERE project_id=? AND stream=? ORDER BY name`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	return storage.ScanRows(rows, scanItem)
}

func (s *SQLiteStore) DeleteItem(ctx context.Context, projectID, stream, itemName string) error {
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The id first — the rows describing an item are keyed on what it IS, and
	// the name is only how this call addressed it. Its absence is the
	// not-found signal.
	var itemID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM items WHERE project_id=? AND stream=? AND name=?`,
		projectID, stream, itemName).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("item %q not found in project %s", itemName, projectID)
	}
	if err != nil {
		return fmt.Errorf("look up item %q: %w", itemName, err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM items WHERE project_id=? AND stream=? AND id=?`, projectID, stream, itemID); err != nil {
		return fmt.Errorf("delete item: %w", err)
	}

	// Everything describing this item on this stream goes with it. Each table
	// is stream-scoped, and so now is blocks, so a sibling branch holding the
	// same item at the same ids is untouched by any of it.
	for _, table := range storeutil.BlockScopedTables() {
		//nolint:gosec // table is a fixed literal from storeutil, never user input
		q := `DELETE FROM ` + table + ` WHERE project_id=? AND stream=?
			 AND block_id IN (SELECT id FROM blocks WHERE project_id=? AND stream=? AND item_name=?)`
		if _, err := tx.ExecContext(ctx, q, projectID, stream, projectID, stream, itemName); err != nil {
			return fmt.Errorf("delete %s for item %q: %w", table, itemName, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM unit_decisions WHERE project_id=? AND stream=? AND item_id=?`,
		projectID, stream, itemID); err != nil {
		return fmt.Errorf("delete unit decisions for item %q: %w", itemName, err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM blocks WHERE project_id=? AND stream=? AND item_name=?`,
		projectID, stream, itemName); err != nil {
		return fmt.Errorf("delete item blocks: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetItemByID(ctx context.Context, projectID, stream, itemID string) (*platstore.Item, error) {
	stream = storeutil.DefaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at
		 FROM items WHERE project_id=? AND stream=? AND id=?`, projectID, stream, itemID)
	return scanItem(row)
}

func scanItem(row scanner) (*platstore.Item, error) {
	var item platstore.Item
	var propsJSON, createdStr, updatedStr string
	err := row.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Format, &item.ItemType,
		&item.BlockIndex, &item.PreviewHTML, &propsJSON, &item.CollectionID, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan item: %w", err)
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	item.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if err := json.Unmarshal([]byte(propsJSON), &item.Properties); err != nil {
		item.Properties = map[string]string{}
	}
	return &item, nil
}

// ---------------------------------------------------------------------------
// Block storage
// ---------------------------------------------------------------------------

func (s *SQLiteStore) StoreBlocks(ctx context.Context, projectID, stream string, blocks []*model.Block) error {
	return s.storeBlocks(ctx, projectID, stream, "", blocks)
}

func (s *SQLiteStore) StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error {
	return s.storeBlocks(ctx, projectID, stream, itemName, blocks)
}

// PruneItemBlocks removes the blocks of one item the producer no longer
// declares. See store.BlockStore for what it is for.
//
// The item's rows are loaded and diffed in Go rather than handed to SQL as a
// NOT IN list: `keep` is one file's worth of keys, and a bound parameter per
// key runs into SQLITE_MAX_VARIABLE_NUMBER on a large enough document.
func (s *SQLiteStore) PruneItemBlocks(ctx context.Context, projectID, stream, itemName string, keep []string) (int, error) {
	if itemName == "" {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		`SELECT id, COALESCE(source_id, '') FROM blocks WHERE project_id=? AND stream=? AND item_name=?`,
		projectID, stream, itemName)
	if err != nil {
		return 0, fmt.Errorf("load item blocks for prune: %w", err)
	}
	declared := make(map[string]struct{}, len(keep))
	for _, k := range keep {
		declared[k] = struct{}{}
	}
	var stale []any
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

	marks := strings.TrimSuffix(strings.Repeat("?,", len(stale)), ",")
	args := append([]any{projectID, stream}, stale...)
	for _, table := range storeutil.BlockScopedTables() {
		//nolint:gosec // table is a fixed literal from storeutil, never user input
		q := `DELETE FROM ` + table + ` WHERE project_id=? AND stream=? AND block_id IN (` + marks + `)`
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return 0, fmt.Errorf("prune %s for item %q: %w", table, itemName, err)
		}
	}
	//nolint:gosec // placeholder list, not data — every id binds below
	del := `DELETE FROM blocks WHERE project_id=? AND stream=? AND id IN (` + marks + `)`
	if _, err := tx.ExecContext(ctx, del, args...); err != nil {
		return 0, fmt.Errorf("prune blocks for item %q: %w", itemName, err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit prune for item %q: %w", itemName, err)
	}
	return len(stale), nil
}

func (s *SQLiteStore) storeBlocks(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error {
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// When storing blocks for a specific item, map format-reader IDs (source_id)
	// to internal project-unique IDs. Blocks stored without an item keep their
	// original ID (they already carry an internal ID from a prior store call).
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
	// address and nothing else. Resolve-or-create, through the same helper the
	// decision ledger uses: storing blocks for an item IS the assertion that the
	// item exists, and two paths minting ids independently would leave a block
	// and the decision about it disagreeing on which file they belong to.
	var itemID string
	if itemName != "" {
		var err error
		if itemID, err = resolveItemIDSQLite(ctx, tx, projectID, stream, itemName); err != nil {
			return err
		}
	}
	if itemName != "" {
		rows, err := tx.QueryContext(ctx,
			`SELECT source_id, id FROM blocks WHERE project_id=? AND stream=? AND item_name=?`,
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
			`UPDATE blocks SET item_name=?, source_id=? WHERE project_id=? AND stream=? AND id=? AND item_name=''`)
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
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, stream, id) DO UPDATE SET
			-- An item-less write (StoreBlocks — the editor saving one target)
			-- carries no item, and must not be read as the block having lost the
			-- one it has. Blank means unsaid, exactly as it does one level up in
			-- keepUndeclaredProperties.
			item_name=CASE WHEN excluded.item_name <> '' THEN excluded.item_name ELSE blocks.item_name END,
			item_id=CASE WHEN excluded.item_id <> '' THEN excluded.item_id ELSE blocks.item_id END,
			name=excluded.name, type=excluded.type, mime_type=excluded.mime_type,
			translatable=excluded.translatable, content_hash=excluded.content_hash,
			context_hash=excluded.context_hash, source_json=excluded.source_json,
			properties=excluded.properties, overlays=excluded.overlays,
			word_count=excluded.word_count, updated_at=excluded.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	// Batch-load existing block hashes + existing target locales so
	// we can diff against the new write for change-log purposes.
	// Targets now live in the translations table (#403/#405); we
	// query it directly here rather than parsing inline JSON.
	existingHashes := map[string]string{}
	existingLocales := map[string]map[string]struct{}{}
	{
		// Bounded to the rows this call touches, mirroring the Postgres store:
		// everything below is read back through each incoming block's resolved
		// row id, and the source-id mapping is final by this point. Loading the
		// whole item — or, with no item scope, the whole project — fed every id
		// into one IN(...), which SQLite's variable limit does not survive at
		// corpus scale any better than Postgres's did.
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

		const prefetchChunk = 500 // stay well under SQLite's bind-variable limit
		for start := 0; start < len(ids); start += prefetchChunk {
			chunk := ids[start:min(start+prefetchChunk, len(ids))]

			// The concatenated fragment is ",?,?,…" from a count — never
			// caller data; values travel as bind parameters below.
			hashQuery := `SELECT id, content_hash FROM blocks WHERE project_id=? AND stream=? AND id IN (?` + //nolint:gosec // placeholder list, not data
				strings.Repeat(",?", len(chunk)-1) + `)`
			args := make([]any, 0, len(chunk)+2)
			args = append(args, projectID, stream)
			for _, id := range chunk {
				args = append(args, id)
			}
			hashRows, err := tx.QueryContext(ctx, hashQuery, args...)
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
				existingHashes[bid] = ch
				present = append(present, bid)
			}
			hashRows.Close()
			if err := hashRows.Err(); err != nil {
				return fmt.Errorf("hash lookup rows: %w", err)
			}
			if len(present) == 0 {
				continue
			}

			localeMap, err := bstore.LoadBlockTargetLocales(ctx, tx, "sqlite", projectID, stream, present)
			if err != nil {
				return fmt.Errorf("batch locale lookup: %w", err)
			}
			for bid, locs := range localeMap {
				set := make(map[string]struct{}, len(locs))
				for _, l := range locs {
					set[l] = struct{}{}
				}
				existingLocales[bid] = set
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
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
				// The caller's DURABLE identity, not its reader id — the same
				// keying the Postgres store moved to: a reader numbers blocks
				// as it goes for formats with no natural key, so keying rows
				// on that id meant deleting one paragraph renumbered every
				// block below it and each stored row rebound to its
				// neighbour's text. convergence.BlockKey resolves to the
				// structural name a reader assigns; the block's own ID still
				// rides the wire untouched.
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

		existingHash, isExisting := existingHashes[internalID]
		isNew := !isExisting
		_ = existingHash // used in change detection below

		// Record target history before overwriting.
		if !isNew && len(b.Targets) > 0 {
			oldTargets, loadErr := loadExistingTargets(ctx, tx, projectID, itemName, internalID)
			if loadErr == nil && oldTargets != nil {
				if err := recordTargetHistory(ctx, tx, projectID, stream, internalID, oldTargets, b.Targets); err != nil {
					return err
				}
			}
		}

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
		overlaysJSON, err := bstore.MarshalOverlays(b.Overlays)
		if err != nil {
			return fmt.Errorf("marshal overlays for block %s: %w", internalID, err)
		}

		translatable := 0
		if b.Translatable {
			translatable = 1
		}

		_, err = stmt.ExecContext(ctx,
			internalID, projectID, stream, itemName, itemID, sourceID, b.Name, b.Type, b.MimeType, translatable,
			identity.ContentHash, identity.ContextHash,
			string(sourceJSON), string(propsJSON), string(overlaysJSON),
			model.CountWordsInRunsJSON(string(sourceJSON)), now, now)
		if err != nil {
			return fmt.Errorf("store block %s: %w", internalID, err)
		}

		// Write targets + annotations into the kind-specific tables.
		nowTime, _ := time.Parse(time.RFC3339, now)
		if err := bstore.SyncBlockOverlays(ctx, tx, "sqlite", projectID, stream, internalID, b.Targets, b.AnnoMap(), nowTime); err != nil {
			return err
		}

		// Append to change log.
		if isNew {
			if err := logChange(ctx, tx, projectID, stream, internalID, "source_added", "", identity.ContentHash); err != nil {
				return fmt.Errorf("log change for block %s: %w", internalID, err)
			}
			// Log target additions for new blocks that already have translations.
			for key := range b.Targets {
				variant := bstore.VariantKeyText(key)
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
				// ledger — mirrors settleDecisionProjectionsPg exactly.
				if err := settleDecisionProjections(ctx, tx, projectID, stream, internalID, identity.ContentHash); err != nil {
					return err
				}
			}
			// Log target changes — added vs modified based on whether the
			// variant already had a row in the translations table. The
			// payload diff (modified-but-same?) is a best-effort check:
			// if the variant was already there and we're upserting, it's
			// a modification worth logging.
			prev := existingLocales[internalID]
			for key := range b.Targets {
				variant := bstore.VariantKeyText(key)
				if _, had := prev[variant]; had {
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

	return tx.Commit()
}

func (s *SQLiteStore) GetBlock(ctx context.Context, projectID, stream, blockID string) (*venue.StoredBlock, error) {
	stream = storeutil.DefaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, item_name, source_id, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, properties, overlays, stored_at, updated_at
		 FROM blocks WHERE project_id=? AND stream=? AND id=?`, projectID, stream, blockID)
	sb, err := scanStoredBlock(row)
	if err != nil {
		return nil, fmt.Errorf("block %s not found in project %s", blockID, projectID)
	}
	if err := bstore.HydrateOverlays(ctx, s.db.DB, "sqlite", projectID, stream, []*venue.StoredBlock{sb}); err != nil {
		return nil, err
	}
	return sb, nil
}

// blockFilterSQLite is the FROM/WHERE half of a BlockQuery, shared by
// GetBlocks and CountBlocks so a filtered page and its counts can never
// disagree about what matches. Positional placeholders bind in string order,
// so the join's arguments precede the WHERE's.
type blockFilterSQLite struct {
	join  string // "" when the query needs no per-locale row
	where string
	args  []any
}

// sqliteStatusBucket is the SQLite twin of the Postgres bucket expression —
// itself the SQL twin of the editor's getBlockStatus — with json_extract
// standing in for the ->> operator.
const sqliteStatusBucket = `CASE
	WHEN COALESCE(json_extract(t.target_json, '$.status'), '') IN ('reviewed', 'signed-off') THEN 'reviewed'
	WHEN COALESCE(t.text, '') = '' THEN 'not-started'
	WHEN json_extract(t.target_json, '$.status') = 'translated' THEN 'translated'
	WHEN json_extract(t.target_json, '$.status') = 'draft' THEN 'draft'
	WHEN json_extract(b.properties, '$."translation-status"') = 'reviewed' THEN 'reviewed'
	WHEN json_extract(b.properties, '$."translation-status"') = 'draft' THEN 'draft'
	WHEN json_extract(b.properties, '$."translation-origin"') IN ('machine', 'pseudo') THEN 'draft'
	ELSE 'translated'
END`

// sqliteSourceTextMatch tests the substring against each source run's text
// separately, so no JSON punctuation can satisfy a search. Text runs
// serialize flat — {"text":"literal"} — so the run's text is at $.text; every
// other run kind nests under its own discriminator and yields NULL there.
const sqliteSourceTextMatch = `EXISTS (
	SELECT 1 FROM json_each(b.source_json) r
	WHERE instr(lower(COALESCE(json_extract(r.value, '$.text'), '')), lower(?)) > 0
)`

// sqliteBlockFilter renders a BlockQuery into bound SQL. withStatus is false
// for the counts query, whose histogram would otherwise be filtered to one
// bucket.
func sqliteBlockFilter(query platstore.BlockQuery, withStatus bool) blockFilterSQLite {
	where := []string{"b.project_id = ?"}
	whereArgs := []any{query.ProjectID}

	// A block belongs to one stream. Unfiltered, every query would read its
	// branches' rows alongside its own — the same block id exists on each — so
	// the scope is applied here rather than left to each caller to remember.
	where = append(where, "b.stream = ?")
	whereArgs = append(whereArgs, storeutil.DefaultStream(query.Stream))

	if query.ItemName != "" {
		where = append(where, "b.item_name = ?")
		whereArgs = append(whereArgs, query.ItemName)
	}
	if len(query.IDs) > 0 {
		var pb strings.Builder
		pb.WriteString("b.id IN (")
		for i, id := range query.IDs {
			if i > 0 {
				pb.WriteByte(',')
			}
			pb.WriteByte('?')
			whereArgs = append(whereArgs, id)
		}
		pb.WriteByte(')')
		where = append(where, pb.String())
	}
	if query.ContentHash != "" {
		where = append(where, "b.content_hash = ?")
		whereArgs = append(whereArgs, query.ContentHash)
	}
	// The keyset cursor. Rows come back in id order, so "after this id" is the
	// next page — see BlockQuery.AfterID.
	if query.AfterID != "" {
		where = append(where, "b.id > ?")
		whereArgs = append(whereArgs, query.AfterID)
	}
	// The cursor pointing the other way — see BlockQuery.BeforeID. The rows are
	// selected in descending id order so a Limit takes the NEAREST predecessors;
	// GetBlocks puts them back in ascending order.
	if query.BeforeID != "" {
		where = append(where, "b.id < ?")
		whereArgs = append(whereArgs, query.BeforeID)
	}
	if query.Translatable != nil {
		v := 0
		if *query.Translatable {
			v = 1
		}
		where = append(where, "b.translatable = ?")
		whereArgs = append(whereArgs, v)
	}

	join := ""
	var joinArgs []any
	if query.TargetLocale != "" {
		join = `LEFT JOIN translations t
			ON t.project_id = b.project_id AND t.block_id = b.id AND t.stream = ? AND t.locale = ?`
		joinArgs = append(joinArgs, storeutil.DefaultStream(query.Stream), query.TargetLocale)
	}
	if query.Text != "" {
		match := sqliteSourceTextMatch
		whereArgs = append(whereArgs, query.Text)
		if join != "" {
			match = "(" + match + " OR instr(lower(COALESCE(t.text, '')), lower(?)) > 0)"
			whereArgs = append(whereArgs, query.Text)
		}
		where = append(where, match)
	}
	if withStatus && query.Status != "" && join != "" {
		where = append(where, sqliteStatusBucket+" = ?")
		whereArgs = append(whereArgs, query.Status)
	}

	return blockFilterSQLite{join: join, where: strings.Join(where, " AND "), args: append(joinArgs, whereArgs...)}
}

func (s *SQLiteStore) GetBlocks(ctx context.Context, query platstore.BlockQuery) ([]*venue.StoredBlock, error) {
	f := sqliteBlockFilter(query, true)

	// Constant skeleton + rendered fragments carrying placeholders only; every
	// value binds through args. Limit and offset are ints, formatted.
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
	result, err := storage.ScanRows(rows, scanStoredBlock)
	if err != nil {
		return nil, err
	}
	if order != "" {
		slices.Reverse(result)
	}
	if err := bstore.HydrateOverlays(ctx, s.db.DB, "sqlite", query.ProjectID, storeutil.DefaultStream(query.Stream), result); err != nil {
		return nil, err
	}
	return result, nil
}

// CountBlocks answers the editor's progress bar with one aggregate instead of
// a full page of hydrated blocks.
func (s *SQLiteStore) CountBlocks(ctx context.Context, query platstore.BlockQuery) (platstore.BlockCounts, error) {
	f := sqliteBlockFilter(query, false)
	bucket := "''"
	if f.join != "" {
		bucket = sqliteStatusBucket
	}

	const skeleton = `SELECT count(*),
			SUM(CASE WHEN x.translatable THEN 1 ELSE 0 END),
			SUM(CASE WHEN x.translatable AND x.bucket = 'not-started' THEN 1 ELSE 0 END),
			SUM(CASE WHEN x.translatable AND x.bucket = 'draft' THEN 1 ELSE 0 END),
			SUM(CASE WHEN x.translatable AND x.bucket = 'translated' THEN 1 ELSE 0 END),
			SUM(CASE WHEN x.translatable AND x.bucket = 'reviewed' THEN 1 ELSE 0 END)
		 FROM (SELECT b.translatable, %s AS bucket FROM blocks b %s WHERE %s) x`

	// SUM over no rows is NULL, so every bucket scans through a nullable int.
	var total int
	var translatable, notStarted, draft, translated, reviewed sql.NullInt64
	err := s.db.QueryRowContext(ctx, fmt.Sprintf(skeleton, bucket, f.join, f.where), f.args...).
		Scan(&total, &translatable, &notStarted, &draft, &translated, &reviewed)
	if err != nil {
		return platstore.BlockCounts{}, fmt.Errorf("count blocks: %w", err)
	}
	return platstore.BlockCounts{
		Total:        total,
		Translatable: int(translatable.Int64),
		NotStarted:   int(notStarted.Int64),
		Draft:        int(draft.Int64),
		Translated:   int(translated.Int64),
		Reviewed:     int(reviewed.Int64),
	}, nil
}

// ListPendingReview pages the (block, locale) pairs whose stored target still
// needs a review decision — the SQLite twin of the Postgres query, with
// json_extract standing in for the ->> operator.
func (s *SQLiteStore) ListPendingReview(ctx context.Context, q platstore.PendingReviewQuery) ([]platstore.PendingReviewRef, int, error) {
	stream := storeutil.DefaultStream(q.Stream)
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	// The SQLite twin of the Postgres predicate: below-reviewed status is
	// pending, empty status included, and the LEFT JOIN on items carries each
	// block's collection without dropping a block whose item has no row for the
	// stream. Positional placeholders bind in string order — the two JOINs'
	// streams come before the WHERE's project.
	const skeleton = `SELECT %s FROM blocks b JOIN translations t
		ON t.project_id = b.project_id AND t.block_id = b.id AND t.stream = ?
		LEFT JOIN items i
		ON i.project_id = b.project_id AND i.stream = ? AND i.name = b.item_name
		WHERE b.project_id = ? AND b.stream = ? AND b.translatable AND t.text <> ''
		AND COALESCE(json_extract(t.target_json, '$.status'), '') NOT IN ('reviewed', 'signed-off')%s`
	args := []any{stream, stream, q.ProjectID, stream}
	scope := ""
	if len(q.Locales) > 0 {
		scope = fmt.Sprintf(` AND t.locale IN (?%s)`, strings.Repeat(",?", len(q.Locales)-1))
		for _, l := range q.Locales {
			args = append(args, l)
		}
	}
	if q.CollectionID != nil {
		scope += ` AND COALESCE(i.collection_id, '') = ?`
		args = append(args, *q.CollectionID)
	}

	var total int
	countQuery := fmt.Sprintf(skeleton, "count(*)", scope)
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count pending review: %w", err)
	}

	query := fmt.Sprintf(skeleton+` ORDER BY b.item_name, b.id, t.locale LIMIT ? OFFSET ?`,
		"b.id, b.item_name, t.locale, COALESCE(i.collection_id, '')", scope)
	rows, err := s.db.QueryContext(ctx, query, append(args, limit, q.Offset)...)
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

func (s *SQLiteStore) GetBlockStats(ctx context.Context, projectID, stream string) ([]platstore.BlockStatRow, error) {
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
		placeholders[i] = "?"
		args = append(args, item.Name)
	}

	// word_count is written at store time; NULL marks a row that predates
	// the column, and only those rows pay the source_json decode — the same
	// contract as the Postgres store.
	q := fmt.Sprintf(
		`SELECT id, item_name, translatable,
			CASE WHEN word_count IS NULL THEN source_json ELSE '' END,
			COALESCE(word_count, -1)
		 FROM blocks WHERE project_id = ? AND stream = ? AND item_name IN (%s)
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
		var translatable, wordCount int
		if err := rows.Scan(&blockID, &itemName, &translatable, &sourceJSON, &wordCount); err != nil {
			return nil, fmt.Errorf("scan block stat: %w", err)
		}
		if wordCount < 0 {
			wordCount = model.CountWordsInRunsJSON(sourceJSON)
		}
		ordered = append(ordered, pending{
			blockID: blockID, itemName: itemName, translatable: translatable == 1,
			sourceWords: wordCount,
		})
		blockIDs = append(blockIDs, blockID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	states, err := bstore.LoadBlockTargetStates(ctx, s.db.DB, "sqlite", projectID, stream, blockIDs)
	if err != nil {
		return nil, err
	}
	var result []platstore.BlockStatRow
	for _, p := range ordered {
		locales, approved := bstore.SplitTargetStates(states[p.blockID])
		result = append(result, platstore.BlockStatRow{
			ItemName:        p.itemName,
			Translatable:    p.translatable,
			SourceWords:     p.sourceWords,
			TargetLocales:   locales,
			ApprovedLocales: approved,
		})
	}
	return result, nil
}

func (s *SQLiteStore) DeleteBlock(ctx context.Context, projectID, stream, blockID string) error {
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
		`SELECT item_name, source_id, item_id FROM blocks WHERE project_id=? AND stream=? AND id=?`,
		projectID, stream, blockID).Scan(&itemName, &sourceID, &itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("block %s not found in project %s stream %s", blockID, projectID, stream)
	}
	if err != nil {
		return fmt.Errorf("look up block %s: %w", blockID, err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM blocks WHERE project_id=? AND stream=? AND id=?`, projectID, stream, blockID); err != nil {
		return fmt.Errorf("delete block: %w", err)
	}

	// Everything filed under the block's id on THIS stream goes with it — a
	// branch holding the same id keeps its own rows, which is the whole point
	// of a branch.
	for _, table := range storeutil.BlockScopedTables() {
		//nolint:gosec // table is a fixed literal from storeutil, never user input
		q := `DELETE FROM ` + table + ` WHERE project_id=? AND stream=? AND block_id=?`
		if _, err := tx.ExecContext(ctx, q, projectID, stream, blockID); err != nil {
			return fmt.Errorf("delete %s for block %s: %w", table, blockID, err)
		}
	}
	if sourceID != "" {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM unit_decisions WHERE project_id=? AND stream=? AND item_id=? AND unit=?`,
			projectID, stream, itemID, sourceID); err != nil {
			return fmt.Errorf("delete unit decisions for block %s: %w", blockID, err)
		}
	}

	if err := logChange(ctx, tx, projectID, stream, blockID, "source_removed", "", ""); err != nil {
		return fmt.Errorf("log change for deleted block %s: %w", blockID, err)
	}

	return tx.Commit()
}

// ClearBlockTargets deletes every committed translation for a block (all
// locales/variants), leaving the block and its source intact.
func (s *SQLiteStore) ClearBlockTargets(ctx context.Context, projectID, stream, blockID string) error {
	stream = storeutil.DefaultStream(stream)
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM translations WHERE project_id=? AND stream=? AND block_id=?`,
		projectID, stream, blockID)
	if err != nil {
		return fmt.Errorf("clear block targets: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Version management
// ---------------------------------------------------------------------------

func (s *SQLiteStore) CreateVersion(ctx context.Context, projectID, stream, label, description string) (*platstore.Version, error) {
	// A version is a point in ONE stream's history — the parameter has always
	// said so, and the statements below used to ignore it and snapshot the whole
	// project.
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	versionID := id.New()
	now := time.Now().UTC()

	// Count blocks.
	var blockCount int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM blocks WHERE project_id=? AND stream=?`, projectID, stream).Scan(&blockCount)
	if err != nil {
		return nil, fmt.Errorf("count blocks: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO versions (id, project_id, label, description, block_count, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		versionID, projectID, label, description, blockCount, now.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("insert version: %w", err)
	}

	// Snapshot current block hashes.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO version_blocks (version_id, block_id, content_hash)
		 SELECT ?, id, content_hash FROM blocks WHERE project_id=? AND stream=?`,
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

func (s *SQLiteStore) GetVersion(ctx context.Context, versionID string) (*platstore.Version, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, label, description, block_count, created_at FROM versions WHERE id=?`,
		versionID)

	var v platstore.Version
	var createdStr string
	err := row.Scan(&v.ID, &v.ProjectID, &v.Label, &v.Description, &v.BlockCount, &createdStr)
	if err != nil {
		return nil, fmt.Errorf("scan version: %w", err)
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &v, nil
}

func (s *SQLiteStore) ListVersions(ctx context.Context, projectID, stream string) ([]*platstore.Version, error) {
	_ = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, label, description, block_count, created_at
		 FROM versions WHERE project_id=? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	return storage.ScanRows(rows, func(row scanner) (*platstore.Version, error) {
		var v platstore.Version
		var createdStr string
		if err := row.Scan(&v.ID, &v.ProjectID, &v.Label, &v.Description, &v.BlockCount, &createdStr); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		return &v, nil
	})
}

func (s *SQLiteStore) Diff(ctx context.Context, fromVersionID, toVersionID string) (*platstore.VersionDiff, error) {
	diff := &platstore.VersionDiff{
		FromVersion: fromVersionID,
		ToVersion:   toVersionID,
	}

	// Get blocks in "from" version.
	fromBlocks := map[string]string{} // blockID -> contentHash
	rows, err := s.db.QueryContext(ctx,
		`SELECT block_id, content_hash FROM version_blocks WHERE version_id=?`, fromVersionID)
	if err != nil {
		return nil, fmt.Errorf("query from blocks: %w", err)
	}
	for rows.Next() {
		var id, hash string
		if err := rows.Scan(&id, &hash); err != nil {
			rows.Close()
			return nil, err
		}
		fromBlocks[id] = hash
	}
	// A truncated read here would drop "from" blocks and mislabel them as
	// removed in the diff below.
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("from blocks rows: %w", err)
	}
	rows.Close()

	// Get blocks in "to" version.
	toBlocks := map[string]string{}
	rows, err = s.db.QueryContext(ctx,
		`SELECT block_id, content_hash FROM version_blocks WHERE version_id=?`, toVersionID)
	if err != nil {
		return nil, fmt.Errorf("query to blocks: %w", err)
	}
	for rows.Next() {
		var id, hash string
		if err := rows.Scan(&id, &hash); err != nil {
			rows.Close()
			return nil, err
		}
		toBlocks[id] = hash
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("to blocks rows: %w", err)
	}
	rows.Close()

	// Compute differences.
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
// Helpers
// ---------------------------------------------------------------------------

// scanner is an alias for storage.Scanner, the interface shared by *sql.Row
// and *sql.Rows. Used by the scanX helper functions.
type scanner = storage.Scanner

func scanProject(row scanner) (*platstore.Project, error) {
	var p platstore.Project
	var srcLocale, targetLocales, propsJSON, createdStr, updatedStr string
	var archived int
	var archivedAtStr sql.NullString
	err := row.Scan(&p.ID, &p.Name, &srcLocale, &targetLocales, &p.TargetLanguageMode, &p.DefaultStream, &p.DashboardVisibility, &propsJSON, &p.WorkspaceID,
		&p.ConvergePolicy, &archived, &archivedAtStr, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan project: %w", err)
	}
	p.DefaultSourceLanguage = model.LocaleID(srcLocale)
	p.TargetLanguages = storeutil.SplitLocales(targetLocales)
	if p.DashboardVisibility == "" {
		p.DashboardVisibility = "private"
	}
	p.Archived = archived != 0
	if archivedAtStr.Valid {
		t, _ := time.Parse(time.RFC3339, archivedAtStr.String)
		p.ArchivedAt = &t
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if err := json.Unmarshal([]byte(propsJSON), &p.Properties); err != nil {
		p.Properties = map[string]string{}
	}
	return &p, nil
}

func scanStoredBlock(row scanner) (*venue.StoredBlock, error) {
	var sb venue.StoredBlock
	sb.Block = &model.Block{}
	var translatable int
	var sourceJSON, propsJSON, overlaysJSON, storedStr, updatedStr string

	err := row.Scan(
		&sb.Block.ID, &sb.ProjectID, &sb.ItemName, &sb.SourceID, &sb.Block.Name, &sb.Block.Type,
		&sb.Block.MimeType, &translatable, &sb.ContentHash, &sb.ContextHash,
		&sourceJSON, &propsJSON, &overlaysJSON, &storedStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan block: %w", err)
	}

	sb.Block.Translatable = translatable != 0
	sb.StoredAt, _ = time.Parse(time.RFC3339, storedStr)
	sb.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	if err := json.Unmarshal([]byte(sourceJSON), &sb.Block.Source); err != nil {
		sb.Block.Source = nil
	}
	if err := json.Unmarshal([]byte(propsJSON), &sb.Block.Properties); err != nil {
		sb.Block.Properties = make(map[string]string)
	}
	// Rehydrate stand-off overlays from the sibling column (symmetric with the
	// bstore.MarshalOverlays write above).
	if overlays, err := bstore.UnmarshalOverlays([]byte(overlaysJSON)); err == nil {
		sb.Block.Overlays = overlays
	}
	// Lift the folded source-authoring status back onto the block (source-first
	// gate). Symmetric with PropsForStore on the write side.
	platstore.ApplySourceStatusFromProps(sb.Block)
	// Targets + Annotations hydrated via bstore.HydrateOverlays after
	// the caller has scanned all rows. Leave empty here.
	sb.Block.Targets = make(map[model.VariantKey]*model.Target)
	return &sb, nil
}

// ---------------------------------------------------------------------------
// Asset CRUD (Bowrain AD-007)
// ---------------------------------------------------------------------------

func (s *SQLiteStore) StoreAsset(ctx context.Context, projectID, stream string, asset *venue.Asset) error {
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

	// Check if this is a new asset or an update (for change log).
	var existingID string
	_ = tx.QueryRowContext(ctx,
		`SELECT id FROM assets WHERE project_id=? AND blob_key=? AND stream=?`,
		projectID, asset.BlobKey, stream).Scan(&existingID)
	isNew := existingID == ""

	_, err = tx.ExecContext(ctx,
		`INSERT INTO assets (id, project_id, item_name, source_id, blob_key, mime_type, filename,
			size_bytes, alt_text, properties, processing_status, processing_hint, stream, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, blob_key) WHERE stream = 'main' DO UPDATE SET
			item_name=excluded.item_name, source_id=excluded.source_id, mime_type=excluded.mime_type,
			filename=excluded.filename, size_bytes=excluded.size_bytes, alt_text=excluded.alt_text,
			properties=excluded.properties, processing_status=excluded.processing_status,
			processing_hint=excluded.processing_hint, updated_at=excluded.updated_at`,
		asset.ID, projectID, asset.ItemName, asset.SourceID, asset.BlobKey, asset.MimeType,
		asset.Filename, asset.SizeBytes, asset.AltText, string(propsJSON),
		asset.ProcessingStatus, asset.ProcessingHint, stream,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store asset: %w", err)
	}

	// Log change for incremental sync.
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

func (s *SQLiteStore) GetAsset(ctx context.Context, projectID, stream, assetID string) (*venue.Asset, error) {
	stream = storeutil.DefaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, item_name, source_id, blob_key, mime_type, filename,
			size_bytes, alt_text, properties, processing_status, processing_hint, stream, created_at, updated_at
		 FROM assets WHERE project_id=? AND stream=? AND id=?`, projectID, stream, assetID)
	return scanAsset(row)
}

func (s *SQLiteStore) ListAssets(ctx context.Context, projectID, stream, itemName string) ([]*venue.Asset, error) {
	stream = storeutil.DefaultStream(stream)
	var rows *sql.Rows
	var err error
	if itemName != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, project_id, item_name, source_id, blob_key, mime_type, filename,
				size_bytes, alt_text, properties, processing_status, processing_hint, stream, created_at, updated_at
			 FROM assets WHERE project_id=? AND stream=? AND item_name=? ORDER BY filename`, projectID, stream, itemName)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, project_id, item_name, source_id, blob_key, mime_type, filename,
				size_bytes, alt_text, properties, processing_status, processing_hint, stream, created_at, updated_at
			 FROM assets WHERE project_id=? AND stream=? ORDER BY filename`, projectID, stream)
	}
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	return storage.ScanRows(rows, scanAsset)
}

func (s *SQLiteStore) DeleteAsset(ctx context.Context, projectID, stream, assetID string) error {
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`DELETE FROM assets WHERE project_id=? AND stream=? AND id=?`, projectID, stream, assetID)
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

// assetScanner is an alias for scanner (storage.Scanner).
type assetScanner = scanner

func scanAsset(row assetScanner) (*venue.Asset, error) {
	var a venue.Asset
	var propsJSON, createdStr, updatedStr string
	err := row.Scan(&a.ID, &a.ProjectID, &a.ItemName, &a.SourceID, &a.BlobKey, &a.MimeType,
		&a.Filename, &a.SizeBytes, &a.AltText, &propsJSON, &a.ProcessingStatus, &a.ProcessingHint,
		&a.Stream, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan asset: %w", err)
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	a.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if err := json.Unmarshal([]byte(propsJSON), &a.Properties); err != nil {
		a.Properties = map[string]string{}
	}
	return &a, nil
}

// ---------------------------------------------------------------------------
// Asset Variants (Bowrain AD-007)
// ---------------------------------------------------------------------------

func (s *SQLiteStore) StoreAssetVariant(ctx context.Context, projectID string, variant *venue.AssetVariant) error {
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

	// Check if this is a new variant or an update.
	var existingKey string
	_ = tx.QueryRowContext(ctx,
		`SELECT blob_key FROM asset_variants WHERE asset_id=? AND locale=?`,
		variant.AssetID, variant.Locale).Scan(&existingKey)
	isNew := existingKey == ""

	_, err = tx.ExecContext(ctx,
		`INSERT INTO asset_variants (asset_id, locale, blob_key, status, mime_type, size_bytes, properties, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(asset_id, locale) DO UPDATE SET
			blob_key=excluded.blob_key, status=excluded.status, mime_type=excluded.mime_type,
			size_bytes=excluded.size_bytes, properties=excluded.properties, updated_at=excluded.updated_at`,
		variant.AssetID, variant.Locale, variant.BlobKey, variant.Status, variant.MimeType,
		variant.SizeBytes, string(propsJSON),
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store asset variant: %w", err)
	}

	// Log change for incremental sync — look up the asset's project/stream.
	var assetProjectID, assetStream string
	err = tx.QueryRowContext(ctx,
		`SELECT project_id, stream FROM assets WHERE id=?`, variant.AssetID).Scan(&assetProjectID, &assetStream)
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

func (s *SQLiteStore) GetAssetVariant(ctx context.Context, _, assetID, locale string) (*venue.AssetVariant, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT asset_id, locale, blob_key, status, mime_type, size_bytes, properties, created_at, updated_at
		 FROM asset_variants WHERE asset_id=? AND locale=?`, assetID, locale)
	return scanAssetVariant(row)
}

func (s *SQLiteStore) ListAssetVariants(ctx context.Context, _, assetID string) ([]*venue.AssetVariant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT asset_id, locale, blob_key, status, mime_type, size_bytes, properties, created_at, updated_at
		 FROM asset_variants WHERE asset_id=? ORDER BY locale`, assetID)
	if err != nil {
		return nil, fmt.Errorf("list asset variants: %w", err)
	}
	return storage.ScanRows(rows, scanAssetVariant)
}

func scanAssetVariant(row assetScanner) (*venue.AssetVariant, error) {
	var v venue.AssetVariant
	var propsJSON, createdStr, updatedStr string
	err := row.Scan(&v.AssetID, &v.Locale, &v.BlobKey, &v.Status, &v.MimeType,
		&v.SizeBytes, &propsJSON, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan asset variant: %w", err)
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	v.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if err := json.Unmarshal([]byte(propsJSON), &v.Properties); err != nil {
		v.Properties = map[string]string{}
	}
	return &v, nil
}
