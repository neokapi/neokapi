package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	platstore "github.com/neokapi/neokapi/platform/store"
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
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO projects (id, name, default_source_language, target_languages, target_language_mode, properties, workspace_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		p.ID, p.Name, string(p.DefaultSourceLanguage), locales, p.TargetLanguageMode, string(propsJSON),
		p.WorkspaceID, now, now)
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetProject(ctx context.Context, id string) (*platstore.Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, default_source_language, target_languages, target_language_mode, properties, workspace_id, archived, archived_at, created_at, updated_at
		 FROM projects WHERE id = $1`, id)
	return scanProjectPg(row)
}

func (s *PostgresStore) ListProjects(ctx context.Context) ([]*platstore.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, default_source_language, target_languages, target_language_mode, properties, workspace_id, archived, archived_at, created_at, updated_at
		 FROM projects WHERE archived=FALSE ORDER BY name`)
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
	locales := joinLocales(p.TargetLanguages)
	propsJSON, err := json.Marshal(p.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	if p.TargetLanguageMode == "" {
		p.TargetLanguageMode = "defined"
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET name=$1, default_source_language=$2, target_languages=$3, target_language_mode=$4, properties=$5, workspace_id=$6, updated_at=$7
		 WHERE id=$8`,
		p.Name, string(p.DefaultSourceLanguage), locales, p.TargetLanguageMode, string(propsJSON),
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
		`SELECT id, name, default_source_language, target_languages, target_language_mode, properties, workspace_id, archived, archived_at, created_at, updated_at
		 FROM projects WHERE workspace_id=$1 AND archived=TRUE ORDER BY archived_at DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list archived projects: %w", err)
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

// ---------------------------------------------------------------------------
// Collection management
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateCollection(ctx context.Context, c *platstore.Collection) error {
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
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		c.ID, c.ProjectID, c.Name, string(c.Kind), c.ItemLabel, c.IsDefault, c.Stream,
		string(configJSON), now, now)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetCollection(ctx context.Context, projectID, collectionID string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, created_at, updated_at
		 FROM collections WHERE project_id=$1 AND id=$2`, projectID, collectionID)
	return scanCollectionPg(row)
}

func (s *PostgresStore) GetCollectionByName(ctx context.Context, projectID, name, stream string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, created_at, updated_at
		 FROM collections WHERE project_id=$1 AND name=$2 AND (stream='' OR stream=$3)`,
		projectID, name, stream)
	return scanCollectionPg(row)
}

func (s *PostgresStore) GetDefaultCollection(ctx context.Context, projectID string) (*platstore.Collection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, created_at, updated_at
		 FROM collections WHERE project_id=$1 AND is_default=TRUE`, projectID)
	return scanCollectionPg(row)
}

func (s *PostgresStore) ListCollections(ctx context.Context, projectID, stream string) ([]*platstore.Collection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, kind, item_label, is_default, stream, connector_config, created_at, updated_at
		 FROM collections WHERE project_id=$1 AND (stream='' OR stream=$2)
		 ORDER BY is_default DESC, name`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()

	var result []*platstore.Collection
	for rows.Next() {
		c, err := scanCollectionPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateCollection(ctx context.Context, c *platstore.Collection) error {
	c.UpdatedAt = time.Now().UTC()

	configJSON, err := json.Marshal(c.ConnectorConfig)
	if err != nil {
		return fmt.Errorf("marshal connector config: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE collections SET name=$1, kind=$2, item_label=$3, stream=$4, connector_config=$5, updated_at=$6
		 WHERE project_id=$7 AND id=$8`,
		c.Name, string(c.Kind), c.ItemLabel, c.Stream, string(configJSON), c.UpdatedAt, c.ProjectID, c.ID)
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
		return fmt.Errorf("cannot delete the default collection")
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

func scanCollectionPg(row scanner) (*platstore.Collection, error) {
	var c platstore.Collection
	var kindStr, configJSON string
	err := row.Scan(&c.ID, &c.ProjectID, &c.Name, &kindStr, &c.ItemLabel,
		&c.IsDefault, &c.Stream, &configJSON, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan collection: %w", err)
	}
	c.Kind = platstore.CollectionKind(kindStr)
	if err := json.Unmarshal([]byte(configJSON), &c.ConnectorConfig); err != nil {
		c.ConnectorConfig = map[string]string{}
	}
	return &c, nil
}

// ---------------------------------------------------------------------------
// Item management
// ---------------------------------------------------------------------------

func (s *PostgresStore) StoreItem(ctx context.Context, projectID, stream string, item *platstore.Item) error {
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
		`INSERT INTO items (id, project_id, stream, name, format, item_type, source_bytes, block_index, properties, collection_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT(project_id, stream, name) DO UPDATE SET
			format=EXCLUDED.format, item_type=EXCLUDED.item_type,
			source_bytes=EXCLUDED.source_bytes, block_index=EXCLUDED.block_index,
			properties=EXCLUDED.properties, collection_id=CASE WHEN EXCLUDED.collection_id='' THEN items.collection_id ELSE EXCLUDED.collection_id END,
			updated_at=EXCLUDED.updated_at`,
		item.ID, projectID, stream, item.Name, item.Format, item.ItemType, item.SourceBytes,
		item.BlockIndex, string(propsJSON), item.CollectionID, now, now)
	if err != nil {
		return fmt.Errorf("store item %q: %w", item.Name, err)
	}
	return nil
}

func (s *PostgresStore) GetItem(ctx context.Context, projectID, stream, itemName string) (*platstore.Item, error) {
	stream = defaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, format, item_type, source_bytes, block_index, properties, collection_id, created_at, updated_at
		 FROM items WHERE project_id=$1 AND stream=$2 AND name=$3`, projectID, stream, itemName)
	return scanItemPg(row)
}

func (s *PostgresStore) ListItems(ctx context.Context, projectID, stream string) ([]*platstore.Item, error) {
	stream = defaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, format, item_type, source_bytes, block_index, properties, collection_id, created_at, updated_at
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
	stream = defaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `DELETE FROM blocks WHERE project_id=$1 AND item_name=$2`, projectID, itemName)
	if err != nil {
		return fmt.Errorf("delete item blocks: %w", err)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM items WHERE project_id=$1 AND stream=$2 AND name=$3`, projectID, stream, itemName)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("item %q not found in project %s", itemName, projectID)
	}

	return tx.Commit()
}

func (s *PostgresStore) GetItemByID(ctx context.Context, projectID, stream, itemID string) (*platstore.Item, error) {
	stream = defaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, format, item_type, source_bytes, block_index, properties, collection_id, created_at, updated_at
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

func (s *PostgresStore) storeBlocks(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error {
	stream = defaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// When storing blocks for a specific item, map format-reader IDs (source_id)
	// to internal project-unique IDs.
	existingSourceIDs := map[string]string{} // source_id → internal id
	if itemName != "" {
		rows, err := tx.QueryContext(ctx,
			`SELECT source_id, id FROM blocks WHERE project_id=$1 AND item_name=$2 AND source_id != ''`,
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
			source_json, targets_json, properties, annotations, stored_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT(project_id, id) DO UPDATE SET
			name=EXCLUDED.name, type=EXCLUDED.type, mime_type=EXCLUDED.mime_type,
			translatable=EXCLUDED.translatable, content_hash=EXCLUDED.content_hash,
			context_hash=EXCLUDED.context_hash, source_json=EXCLUDED.source_json,
			targets_json=CASE WHEN EXCLUDED.targets_json IN ('{}', 'null', '') THEN blocks.targets_json ELSE EXCLUDED.targets_json END,
			properties=EXCLUDED.properties,
			annotations=EXCLUDED.annotations, updated_at=EXCLUDED.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	hashStmt, err := tx.PrepareContext(ctx,
		`SELECT content_hash, targets_json FROM blocks WHERE project_id = $1 AND id = $2`)
	if err != nil {
		return fmt.Errorf("prepare hash lookup: %w", err)
	}
	defer hashStmt.Close()

	now := time.Now().UTC()
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

		var existingHash, existingTargetsJSON string
		hashErr := hashStmt.QueryRowContext(ctx, projectID, internalID).Scan(&existingHash, &existingTargetsJSON)
		if hashErr != nil && hashErr != sql.ErrNoRows {
			return fmt.Errorf("hash lookup for block %s: %w", internalID, hashErr)
		}
		isNew := hashErr == sql.ErrNoRows

		sourceJSON, err := json.Marshal(b.Source)
		if err != nil {
			return fmt.Errorf("marshal source for block %s: %w", internalID, err)
		}
		targetsJSON, err := json.Marshal(b.Targets)
		if err != nil {
			return fmt.Errorf("marshal targets for block %s: %w", internalID, err)
		}
		propsJSON, err := json.Marshal(b.Properties)
		if err != nil {
			return fmt.Errorf("marshal properties for block %s: %w", internalID, err)
		}
		annsJSON, err := serializeAnnotations(b.Annotations)
		if err != nil {
			return fmt.Errorf("marshal annotations for block %s: %w", internalID, err)
		}

		_, err = stmt.ExecContext(ctx,
			internalID, projectID, itemName, sourceID, b.Name, b.Type, b.MimeType, b.Translatable,
			identity.ContentHash, identity.ContextHash,
			string(sourceJSON), string(targetsJSON),
			string(propsJSON), string(annsJSON), now, now)
		if err != nil {
			return fmt.Errorf("store block %s: %w", internalID, err)
		}

		if isNew {
			if err := logChangePg(ctx, tx, projectID, stream, internalID, "source_added", "", identity.ContentHash); err != nil {
				return fmt.Errorf("log change for block %s: %w", internalID, err)
			}
			for locale := range b.Targets {
				if err := logChangePg(ctx, tx, projectID, stream, internalID, "target_added", string(locale), ""); err != nil {
					return fmt.Errorf("log target change for block %s locale %s: %w", internalID, locale, err)
				}
			}
		} else {
			if existingHash != identity.ContentHash {
				if err := logChangePg(ctx, tx, projectID, stream, internalID, "source_modified", "", identity.ContentHash); err != nil {
					return fmt.Errorf("log change for block %s: %w", internalID, err)
				}
			}
			if len(b.Targets) > 0 {
				var oldTargets map[model.LocaleID][]*model.Segment
				if existingTargetsJSON != "" {
					_ = json.Unmarshal([]byte(existingTargetsJSON), &oldTargets)
				}
				for locale, newSegs := range b.Targets {
					oldSegs, had := oldTargets[locale]
					if !had {
						if err := logChangePg(ctx, tx, projectID, stream, internalID, "target_added", string(locale), ""); err != nil {
							return fmt.Errorf("log target change for block %s locale %s: %w", internalID, locale, err)
						}
					} else {
						oldJSON, _ := json.Marshal(oldSegs)
						newJSON, _ := json.Marshal(newSegs)
						if string(oldJSON) != string(newJSON) {
							if err := logChangePg(ctx, tx, projectID, stream, internalID, "target_modified", string(locale), ""); err != nil {
								return fmt.Errorf("log target change for block %s locale %s: %w", internalID, locale, err)
							}
						}
					}
				}
			}
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) GetBlock(ctx context.Context, projectID, stream, blockID string) (*platstore.StoredBlock, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, item_name, source_id, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, targets_json, properties, annotations, stored_at, updated_at
		 FROM blocks WHERE project_id=$1 AND id=$2`, projectID, blockID)
	sb, err := scanStoredBlockPg(row)
	if err != nil {
		return nil, fmt.Errorf("block %s not found in project %s", blockID, projectID)
	}
	return sb, nil
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
	}

	q := fmt.Sprintf(
		`SELECT id, project_id, item_name, source_id, name, type, mime_type, translatable, content_hash, context_hash,
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

func (s *PostgresStore) GetBlockStats(ctx context.Context, projectID, stream string) ([]platstore.BlockStatRow, error) {
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
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, item.Name)
	}

	q := fmt.Sprintf(
		`SELECT item_name, translatable, source_json, targets_json
		 FROM blocks WHERE project_id = $1 AND item_name IN (%s)
		 ORDER BY item_name, id`,
		strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query block stats: %w", err)
	}
	defer rows.Close()

	var result []platstore.BlockStatRow
	for rows.Next() {
		var itemName, sourceJSON, targetsJSON string
		var translatable bool
		if err := rows.Scan(&itemName, &translatable, &sourceJSON, &targetsJSON); err != nil {
			return nil, fmt.Errorf("scan block stat: %w", err)
		}
		result = append(result, platstore.BlockStatRow{
			ItemName:      itemName,
			Translatable:  translatable,
			SourceWords:   countWordsFromSourceJSON(sourceJSON),
			TargetLocales: extractTargetLocales(targetsJSON),
		})
	}
	return result, rows.Err()
}

func (s *PostgresStore) DeleteBlock(ctx context.Context, projectID, stream, blockID string) error {
	stream = defaultStream(stream)
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

	if err := logChangePg(ctx, tx, projectID, stream, blockID, "source_removed", "", ""); err != nil {
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

func scanProjectPg(row scanner) (*platstore.Project, error) {
	var p platstore.Project
	var srcLocale, targetLocales, propsJSON string
	err := row.Scan(&p.ID, &p.Name, &srcLocale, &targetLocales, &p.TargetLanguageMode, &propsJSON, &p.WorkspaceID,
		&p.Archived, &p.ArchivedAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan project: %w", err)
	}
	p.DefaultSourceLanguage = model.LocaleID(srcLocale)
	p.TargetLanguages = splitLocales(targetLocales)
	if err := json.Unmarshal([]byte(propsJSON), &p.Properties); err != nil {
		p.Properties = map[string]string{}
	}
	return &p, nil
}

func scanItemPg(row scanner) (*platstore.Item, error) {
	var item platstore.Item
	var propsJSON string
	err := row.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Format, &item.ItemType,
		&item.SourceBytes, &item.BlockIndex, &propsJSON, &item.CollectionID, &item.CreatedAt, &item.UpdatedAt)
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
		&sb.Block.ID, &sb.ProjectID, &sb.ItemName, &sb.SourceID, &sb.Block.Name, &sb.Block.Type,
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
	sb.Block.Annotations = deserializeAnnotations(annsJSON)

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
func logChangePg(ctx context.Context, tx *sql.Tx, projectID, stream, blockID, changeType, locale, contentHash string) error {
	stream = defaultStream(stream)
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
		`INSERT INTO change_log (project_id, stream, block_id, change_type, locale, content_hash, logged_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		projectID, stream, blockID, changeType, localeVal, hashVal, now)
	return err
}

// ---------------------------------------------------------------------------
// Asset CRUD (AD-029)
// ---------------------------------------------------------------------------

func (s *PostgresStore) StoreAsset(ctx context.Context, projectID, stream string, asset *platstore.Asset) error {
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

	_, err = s.db.ExecContext(ctx,
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
	return nil
}

func (s *PostgresStore) GetAsset(ctx context.Context, projectID, stream, assetID string) (*platstore.Asset, error) {
	stream = defaultStream(stream)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, item_name, source_id, blob_key, mime_type, filename,
			size_bytes, alt_text, properties, processing_status, processing_hint, stream, created_at, updated_at
		 FROM assets WHERE project_id=$1 AND stream=$2 AND id=$3`, projectID, stream, assetID)
	return scanAssetPg(row)
}

func (s *PostgresStore) ListAssets(ctx context.Context, projectID, stream, itemName string) ([]*platstore.Asset, error) {
	stream = defaultStream(stream)
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

	var result []*platstore.Asset
	for rows.Next() {
		a, err := scanAssetPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (s *PostgresStore) DeleteAsset(ctx context.Context, projectID, stream, assetID string) error {
	stream = defaultStream(stream)
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM assets WHERE project_id=$1 AND stream=$2 AND id=$3`, projectID, stream, assetID)
	if err != nil {
		return fmt.Errorf("delete asset: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("asset %q not found", assetID)
	}
	return nil
}

type pgAssetScanner interface {
	Scan(dest ...any) error
}

func scanAssetPg(row pgAssetScanner) (*platstore.Asset, error) {
	var a platstore.Asset
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
// Asset Variants (AD-029)
// ---------------------------------------------------------------------------

func (s *PostgresStore) StoreAssetVariant(ctx context.Context, _ string, variant *platstore.AssetVariant) error {
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

	_, err = s.db.ExecContext(ctx,
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
	return nil
}

func (s *PostgresStore) GetAssetVariant(ctx context.Context, _, assetID, locale string) (*platstore.AssetVariant, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT asset_id, locale, blob_key, status, mime_type, size_bytes, properties, created_at, updated_at
		 FROM asset_variants WHERE asset_id=$1 AND locale=$2`, assetID, locale)
	return scanAssetVariantPg(row)
}

func (s *PostgresStore) ListAssetVariants(ctx context.Context, _, assetID string) ([]*platstore.AssetVariant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT asset_id, locale, blob_key, status, mime_type, size_bytes, properties, created_at, updated_at
		 FROM asset_variants WHERE asset_id=$1 ORDER BY locale`, assetID)
	if err != nil {
		return nil, fmt.Errorf("list asset variants: %w", err)
	}
	defer rows.Close()

	var result []*platstore.AssetVariant
	for rows.Next() {
		v, err := scanAssetVariantPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func scanAssetVariantPg(row pgAssetScanner) (*platstore.AssetVariant, error) {
	var v platstore.AssetVariant
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
