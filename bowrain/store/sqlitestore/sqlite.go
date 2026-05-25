package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
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

// defaultStream returns "main" when stream is empty.
func defaultStream(stream string) string {
	if stream == "" {
		return "main"
	}
	return stream
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

	locales := joinLocales(p.TargetLanguages)
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
		`INSERT INTO projects (id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, string(p.DefaultSourceLanguage), locales, p.TargetLanguageMode, p.DefaultStream, p.DashboardVisibility, string(propsJSON),
		p.WorkspaceID, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetProject(ctx context.Context, id string) (*platstore.Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, archived, archived_at, created_at, updated_at
		 FROM projects WHERE id = ?`, id)
	return scanProject(row)
}

func (s *SQLiteStore) ListProjects(ctx context.Context) ([]*platstore.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, archived, archived_at, created_at, updated_at
		 FROM projects WHERE archived=0 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return storage.ScanRows(rows, scanProject)
}

func (s *SQLiteStore) UpdateProject(ctx context.Context, p *platstore.Project) error {
	p.UpdatedAt = time.Now().UTC()
	locales := joinLocales(p.TargetLanguages)
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
		`UPDATE projects SET name=?, default_source_language=?, target_languages=?, target_language_mode=?, default_stream=?, dashboard_visibility=?, properties=?, workspace_id=?, updated_at=?
		 WHERE id=?`,
		p.Name, string(p.DefaultSourceLanguage), locales, p.TargetLanguageMode, p.DefaultStream, p.DashboardVisibility, string(propsJSON),
		p.WorkspaceID, p.UpdatedAt.Format(time.RFC3339), p.ID)
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
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %s not found", id)
	}
	return nil
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
		`SELECT id, name, default_source_language, target_languages, target_language_mode, default_stream, dashboard_visibility, properties, workspace_id, archived, archived_at, created_at, updated_at
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

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO collections (id, project_id, name, kind, item_label, is_default, stream, connector_config, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProjectID, c.Name, string(c.Kind), c.ItemLabel, c.IsDefault, c.Stream,
		string(configJSON), now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetCollection(ctx context.Context, projectID, collectionID string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, created_at, updated_at
		 FROM collections WHERE project_id=? AND id=?`, projectID, collectionID)
	return scanCollection(row)
}

func (s *SQLiteStore) GetCollectionByName(ctx context.Context, projectID, name, stream string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, created_at, updated_at
		 FROM collections WHERE project_id=? AND name=? AND (stream='' OR stream=?)`,
		projectID, name, stream)
	return scanCollection(row)
}

func (s *SQLiteStore) GetDefaultCollection(ctx context.Context, projectID string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, created_at, updated_at
		 FROM collections WHERE project_id=? AND is_default=1`, projectID)
	return scanCollection(row)
}

func (s *SQLiteStore) ListCollections(ctx context.Context, projectID, stream string) ([]*platstore.Collection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, created_at, updated_at
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

	_, err = s.db.ExecContext(ctx,
		`UPDATE collections SET name=?, kind=?, item_label=?, stream=?, connector_config=?, updated_at=?
		 WHERE project_id=? AND id=?`,
		c.Name, string(c.Kind), c.ItemLabel, c.Stream, string(configJSON),
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
	var kindStr, configJSON, createdStr, updatedStr string
	err := row.Scan(&c.ID, &c.ProjectID, &c.Name, &kindStr, &c.ItemLabel,
		&c.IsDefault, &c.Stream, &configJSON, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan collection: %w", err)
	}
	c.Kind = platstore.CollectionKind(kindStr)
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if err := json.Unmarshal([]byte(configJSON), &c.ConnectorConfig); err != nil {
		c.ConnectorConfig = map[string]string{}
	}
	return &c, nil
}

// ---------------------------------------------------------------------------
// Item management
// ---------------------------------------------------------------------------

func (s *SQLiteStore) StoreItem(ctx context.Context, projectID, stream string, item *platstore.Item) error {
	stream = defaultStream(stream)
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

	// Resolve collection_id to the default collection if not set.
	if item.CollectionID == "" {
		defColl, defErr := s.GetDefaultCollection(ctx, projectID)
		if defErr == nil {
			item.CollectionID = defColl.ID
		}
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO items (id, project_id, stream, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, stream, name) DO UPDATE SET
			format=excluded.format, item_type=excluded.item_type,
			block_index=excluded.block_index, preview_html=excluded.preview_html,
			properties=excluded.properties, collection_id=CASE WHEN excluded.collection_id='' THEN items.collection_id ELSE excluded.collection_id END,
			updated_at=excluded.updated_at`,
		item.ID, projectID, stream, item.Name, item.Format, item.ItemType,
		item.BlockIndex, item.PreviewHTML, string(propsJSON), item.CollectionID,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store item %q: %w", item.Name, err)
	}
	return nil
}

func (s *SQLiteStore) GetItem(ctx context.Context, projectID, stream, itemName string) (*platstore.Item, error) {
	stream = defaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at
		 FROM items WHERE project_id=? AND stream=? AND name=?`, projectID, stream, itemName)
	return scanItem(row)
}

func (s *SQLiteStore) ListItems(ctx context.Context, projectID, stream string) ([]*platstore.Item, error) {
	stream = defaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at
		 FROM items WHERE project_id=? AND stream=? ORDER BY name`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	return storage.ScanRows(rows, scanItem)
}

func (s *SQLiteStore) DeleteItem(ctx context.Context, projectID, stream, itemName string) error {
	stream = defaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete blocks belonging to this item.
	_, err = tx.ExecContext(ctx, `DELETE FROM blocks WHERE project_id=? AND item_name=?`, projectID, itemName)
	if err != nil {
		return fmt.Errorf("delete item blocks: %w", err)
	}

	// Delete the item itself.
	res, err := tx.ExecContext(ctx, `DELETE FROM items WHERE project_id=? AND stream=? AND name=?`, projectID, stream, itemName)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("item %q not found in project %s", itemName, projectID)
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetItemByID(ctx context.Context, projectID, stream, itemID string) (*platstore.Item, error) {
	stream = defaultStream(stream)
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

func (s *SQLiteStore) storeBlocks(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error {
	stream = defaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// When storing blocks for a specific item, map format-reader IDs (source_id)
	// to internal project-unique IDs. Blocks stored without an item keep their
	// original ID (they already carry an internal ID from a prior store call).
	existingSourceIDs := map[string]string{} // source_id → internal id
	if itemName != "" {
		rows, err := tx.QueryContext(ctx,
			`SELECT source_id, id FROM blocks WHERE project_id=? AND item_name=? AND source_id != ''`,
			projectID, itemName)
		if err != nil {
			return fmt.Errorf("load source_id mapping: %w", err)
		}
		for rows.Next() {
			var srcID, intID string
			if err := rows.Scan(&srcID, &intID); err != nil {
				rows.Close()
				return fmt.Errorf("scan source_id mapping: %w", err)
			}
			existingSourceIDs[srcID] = intID
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("source_id mapping rows: %w", err)
		}
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO blocks (id, project_id, item_name, source_id, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, properties, stored_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, id) DO UPDATE SET
			name=excluded.name, type=excluded.type, mime_type=excluded.mime_type,
			translatable=excluded.translatable, content_hash=excluded.content_hash,
			context_hash=excluded.context_hash, source_json=excluded.source_json,
			properties=excluded.properties, updated_at=excluded.updated_at`)
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
		var hashQuery string
		var hashArgs []any
		if itemName != "" {
			hashQuery = `SELECT id, content_hash FROM blocks WHERE project_id=? AND item_name=?`
			hashArgs = []any{projectID, itemName}
		} else {
			hashQuery = `SELECT id, content_hash FROM blocks WHERE project_id=?`
			hashArgs = []any{projectID}
		}
		hashRows, err := tx.QueryContext(ctx, hashQuery, hashArgs...)
		if err != nil {
			return fmt.Errorf("batch hash lookup: %w", err)
		}
		var ids []string
		for hashRows.Next() {
			var bid, ch string
			if err := hashRows.Scan(&bid, &ch); err != nil {
				hashRows.Close()
				return fmt.Errorf("scan hash: %w", err)
			}
			existingHashes[bid] = ch
			ids = append(ids, bid)
		}
		hashRows.Close()

		if len(ids) > 0 {
			localeMap, err := bstore.LoadBlockTargetLocales(ctx, tx, "sqlite", projectID, stream, ids)
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
			sourceID = b.ID
			if existingID, found := existingSourceIDs[sourceID]; found {
				internalID = existingID
			} else {
				internalID = newBlockID()
				existingSourceIDs[sourceID] = internalID
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
				_ = recordTargetHistory(ctx, tx, projectID, stream, internalID, oldTargets, b.Targets)
			}
		}

		sourceJSON, err := json.Marshal(b.Source)
		if err != nil {
			return fmt.Errorf("marshal source for block %s: %w", internalID, err)
		}
		propsJSON, err := json.Marshal(b.Properties)
		if err != nil {
			return fmt.Errorf("marshal properties for block %s: %w", internalID, err)
		}

		translatable := 0
		if b.Translatable {
			translatable = 1
		}

		_, err = stmt.ExecContext(ctx,
			internalID, projectID, itemName, sourceID, b.Name, b.Type, b.MimeType, translatable,
			identity.ContentHash, identity.ContextHash,
			string(sourceJSON), string(propsJSON), now, now)
		if err != nil {
			return fmt.Errorf("store block %s: %w", internalID, err)
		}

		// Write targets + annotations into the kind-specific tables.
		nowTime, _ := time.Parse(time.RFC3339, now)
		if err := bstore.SyncBlockOverlays(ctx, tx, "sqlite", projectID, stream, internalID, b.Targets, b.Annotations, nowTime); err != nil {
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

// newBlockID generates a short random block ID (8 chars, base62-encoded).
func newBlockID() string { return id.New() }

func (s *SQLiteStore) GetBlock(ctx context.Context, projectID, stream, blockID string) (*platstore.StoredBlock, error) {
	stream = defaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, item_name, source_id, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, properties, stored_at, updated_at
		 FROM blocks WHERE project_id=? AND id=?`, projectID, blockID)
	sb, err := scanStoredBlock(row)
	if err != nil {
		return nil, fmt.Errorf("block %s not found in project %s", blockID, projectID)
	}
	if err := bstore.HydrateOverlays(ctx, s.db.DB, "sqlite", projectID, stream, []*platstore.StoredBlock{sb}); err != nil {
		return nil, err
	}
	return sb, nil
}

func (s *SQLiteStore) GetBlocks(ctx context.Context, query platstore.BlockQuery) ([]*platstore.StoredBlock, error) {
	where := []string{"project_id = ?"}
	args := []any{query.ProjectID}

	if query.ItemName != "" {
		where = append(where, "item_name = ?")
		args = append(args, query.ItemName)
	}
	if len(query.IDs) > 0 {
		var pb strings.Builder
		pb.WriteString("id IN (")
		for i, id := range query.IDs {
			if i > 0 {
				pb.WriteByte(',')
			}
			pb.WriteByte('?')
			args = append(args, id)
		}
		pb.WriteByte(')')
		where = append(where, pb.String())
	}
	if query.ContentHash != "" {
		where = append(where, "content_hash = ?")
		args = append(args, query.ContentHash)
	}
	if query.Translatable != nil {
		v := 0
		if *query.Translatable {
			v = 1
		}
		where = append(where, "translatable = ?")
		args = append(args, v)
	}

	var qb strings.Builder
	qb.WriteString(`SELECT id, project_id, item_name, source_id, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, properties, stored_at, updated_at
		 FROM blocks WHERE `)
	qb.WriteString(strings.Join(where, " AND "))
	qb.WriteString(" ORDER BY id")

	if query.Limit > 0 {
		fmt.Fprintf(&qb, " LIMIT %d", query.Limit)
	}
	if query.Offset > 0 {
		fmt.Fprintf(&qb, " OFFSET %d", query.Offset)
	}
	q := qb.String()

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query blocks: %w", err)
	}
	result, err := storage.ScanRows(rows, scanStoredBlock)
	if err != nil {
		return nil, err
	}
	if err := bstore.HydrateOverlays(ctx, s.db.DB, "sqlite", query.ProjectID, defaultStream(query.Stream), result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SQLiteStore) GetBlockStats(ctx context.Context, projectID, stream string) ([]platstore.BlockStatRow, error) {
	stream = defaultStream(stream)

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
	args := []any{projectID}
	for i, item := range items {
		placeholders[i] = "?"
		args = append(args, item.Name)
	}

	q := fmt.Sprintf(
		`SELECT id, item_name, translatable, source_json
		 FROM blocks WHERE project_id = ? AND item_name IN (%s)
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
		var translatable int
		if err := rows.Scan(&blockID, &itemName, &translatable, &sourceJSON); err != nil {
			return nil, fmt.Errorf("scan block stat: %w", err)
		}
		ordered = append(ordered, pending{
			blockID: blockID, itemName: itemName, translatable: translatable == 1,
			sourceWords: countWordsFromSourceJSON(sourceJSON),
		})
		blockIDs = append(blockIDs, blockID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	locales, err := bstore.LoadBlockTargetLocales(ctx, s.db.DB, "sqlite", projectID, stream, blockIDs)
	if err != nil {
		return nil, err
	}
	var result []platstore.BlockStatRow
	for _, p := range ordered {
		result = append(result, platstore.BlockStatRow{
			ItemName:      p.itemName,
			Translatable:  p.translatable,
			SourceWords:   p.sourceWords,
			TargetLocales: locales[p.blockID],
		})
	}
	return result, nil
}

func (s *SQLiteStore) DeleteBlock(ctx context.Context, projectID, stream, blockID string) error {
	stream = defaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM blocks WHERE project_id=? AND id=?`, projectID, blockID)
	if err != nil {
		return fmt.Errorf("delete block: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("block %s not found in project %s", blockID, projectID)
	}

	if err := logChange(ctx, tx, projectID, stream, blockID, "source_removed", "", ""); err != nil {
		return fmt.Errorf("log change for deleted block %s: %w", blockID, err)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Version management
// ---------------------------------------------------------------------------

func (s *SQLiteStore) CreateVersion(ctx context.Context, projectID, stream, label, description string) (*platstore.Version, error) {
	_ = defaultStream(stream) // versions snapshot all blocks in the project
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	versionID := id.New()
	now := time.Now().UTC()

	// Count blocks.
	var blockCount int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM blocks WHERE project_id=?`, projectID).Scan(&blockCount)
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
		 SELECT ?, id, content_hash FROM blocks WHERE project_id=?`,
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
	_ = defaultStream(stream)
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

func joinLocales(locales []model.LocaleID) string {
	parts := make([]string, len(locales))
	for i, l := range locales {
		parts[i] = string(l)
	}
	return strings.Join(parts, ",")
}

func splitLocales(s string) []model.LocaleID {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	locales := make([]model.LocaleID, len(parts))
	for i, p := range parts {
		locales[i] = model.LocaleID(strings.TrimSpace(p))
	}
	return locales
}

// scanner is an alias for storage.Scanner, the interface shared by *sql.Row
// and *sql.Rows. Used by the scanX helper functions.
type scanner = storage.Scanner

func scanProject(row scanner) (*platstore.Project, error) {
	var p platstore.Project
	var srcLocale, targetLocales, propsJSON, createdStr, updatedStr string
	var archived int
	var archivedAtStr sql.NullString
	err := row.Scan(&p.ID, &p.Name, &srcLocale, &targetLocales, &p.TargetLanguageMode, &p.DefaultStream, &p.DashboardVisibility, &propsJSON, &p.WorkspaceID,
		&archived, &archivedAtStr, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan project: %w", err)
	}
	p.DefaultSourceLanguage = model.LocaleID(srcLocale)
	p.TargetLanguages = splitLocales(targetLocales)
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

func scanStoredBlock(row scanner) (*platstore.StoredBlock, error) {
	var sb platstore.StoredBlock
	sb.Block = &model.Block{}
	var translatable int
	var sourceJSON, propsJSON, storedStr, updatedStr string

	err := row.Scan(
		&sb.Block.ID, &sb.ProjectID, &sb.ItemName, &sb.SourceID, &sb.Block.Name, &sb.Block.Type,
		&sb.Block.MimeType, &translatable, &sb.ContentHash, &sb.ContextHash,
		&sourceJSON, &propsJSON, &storedStr, &updatedStr)
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
	// Targets + Annotations hydrated via bstore.HydrateOverlays after
	// the caller has scanned all rows. Leave empty here.
	sb.Block.Targets = make(map[model.VariantKey]*model.Target)
	sb.Block.Annotations = make(map[string]model.Annotation)
	return &sb, nil
}

// ---------------------------------------------------------------------------
// Asset CRUD (Bowrain AD-007)
// ---------------------------------------------------------------------------

func (s *SQLiteStore) StoreAsset(ctx context.Context, projectID, stream string, asset *platstore.Asset) error {
	stream = defaultStream(stream)
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

func (s *SQLiteStore) GetAsset(ctx context.Context, projectID, stream, assetID string) (*platstore.Asset, error) {
	stream = defaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, item_name, source_id, blob_key, mime_type, filename,
			size_bytes, alt_text, properties, processing_status, processing_hint, stream, created_at, updated_at
		 FROM assets WHERE project_id=? AND stream=? AND id=?`, projectID, stream, assetID)
	return scanAsset(row)
}

func (s *SQLiteStore) ListAssets(ctx context.Context, projectID, stream, itemName string) ([]*platstore.Asset, error) {
	stream = defaultStream(stream)
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
	stream = defaultStream(stream)
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

func scanAsset(row assetScanner) (*platstore.Asset, error) {
	var a platstore.Asset
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

func (s *SQLiteStore) StoreAssetVariant(ctx context.Context, projectID string, variant *platstore.AssetVariant) error {
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
		_ = logChange(ctx, tx, assetProjectID, assetStream, variant.AssetID, changeType, variant.Locale, variant.BlobKey)
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetAssetVariant(ctx context.Context, _, assetID, locale string) (*platstore.AssetVariant, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT asset_id, locale, blob_key, status, mime_type, size_bytes, properties, created_at, updated_at
		 FROM asset_variants WHERE asset_id=? AND locale=?`, assetID, locale)
	return scanAssetVariant(row)
}

func (s *SQLiteStore) ListAssetVariants(ctx context.Context, _, assetID string) ([]*platstore.AssetVariant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT asset_id, locale, blob_key, status, mime_type, size_bytes, properties, created_at, updated_at
		 FROM asset_variants WHERE asset_id=? ORDER BY locale`, assetID)
	if err != nil {
		return nil, fmt.Errorf("list asset variants: %w", err)
	}
	return storage.ScanRows(rows, scanAssetVariant)
}

func scanAssetVariant(row assetScanner) (*platstore.AssetVariant, error) {
	var v platstore.AssetVariant
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
