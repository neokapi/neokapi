package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gokapi/gokapi/core/kaz"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/internal/storage"
	"github.com/google/uuid"
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

// Close closes the underlying database.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ---------------------------------------------------------------------------
// Project CRUD
// ---------------------------------------------------------------------------

func (s *SQLiteStore) CreateProject(ctx context.Context, p *Project) error {
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
		`INSERT INTO projects (id, name, source_locale, target_locales, properties, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, string(p.SourceLocale), locales, string(propsJSON),
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetProject(ctx context.Context, id string) (*Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, source_locale, target_locales, properties, created_at, updated_at
		 FROM projects WHERE id = ?`, id)
	return scanProject(row)
}

func (s *SQLiteStore) ListProjects(ctx context.Context) ([]*Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, source_locale, target_locales, properties, created_at, updated_at
		 FROM projects ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var result []*Project
	for rows.Next() {
		p, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) UpdateProject(ctx context.Context, p *Project) error {
	p.UpdatedAt = time.Now().UTC()
	locales := joinLocales(p.TargetLocales)
	propsJSON, err := json.Marshal(p.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET name=?, source_locale=?, target_locales=?, properties=?, updated_at=?
		 WHERE id=?`,
		p.Name, string(p.SourceLocale), locales, string(propsJSON),
		p.UpdatedAt.Format(time.RFC3339), p.ID)
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

// ---------------------------------------------------------------------------
// Block storage
// ---------------------------------------------------------------------------

func (s *SQLiteStore) StoreBlocks(ctx context.Context, projectID string, blocks []*model.Block) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO blocks (id, project_id, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, targets_json, properties, annotations, stored_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, id) DO UPDATE SET
			name=excluded.name, type=excluded.type, mime_type=excluded.mime_type,
			translatable=excluded.translatable, content_hash=excluded.content_hash,
			context_hash=excluded.context_hash, source_json=excluded.source_json,
			targets_json=excluded.targets_json, properties=excluded.properties,
			annotations=excluded.annotations, updated_at=excluded.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, b := range blocks {
		identity := model.ComputeIdentity(b)
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

		translatable := 0
		if b.Translatable {
			translatable = 1
		}

		_, err = stmt.ExecContext(ctx,
			b.ID, projectID, b.Name, b.Type, b.MimeType, translatable,
			identity.ContentHash, identity.ContextHash,
			string(sourceJSON), string(targetsJSON),
			string(propsJSON), string(annsJSON), now, now)
		if err != nil {
			return fmt.Errorf("store block %s: %w", b.ID, err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetBlock(ctx context.Context, projectID, blockID string) (*StoredBlock, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, targets_json, properties, annotations, stored_at, updated_at
		 FROM blocks WHERE project_id=? AND id=?`, projectID, blockID)
	return scanStoredBlock(row)
}

func (s *SQLiteStore) GetBlocks(ctx context.Context, query BlockQuery) ([]*StoredBlock, error) {
	where := []string{"project_id = ?"}
	args := []any{query.ProjectID}

	if len(query.IDs) > 0 {
		placeholders := make([]string, len(query.IDs))
		for i, id := range query.IDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where = append(where, fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")))
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

	q := fmt.Sprintf(
		`SELECT id, project_id, name, type, mime_type, translatable, content_hash, context_hash,
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

	var result []*StoredBlock
	for rows.Next() {
		sb, err := scanStoredBlockRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sb)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) DeleteBlock(ctx context.Context, projectID, blockID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM blocks WHERE project_id=? AND id=?`, projectID, blockID)
	if err != nil {
		return fmt.Errorf("delete block: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("block %s not found in project %s", blockID, projectID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Version management
// ---------------------------------------------------------------------------

func (s *SQLiteStore) CreateVersion(ctx context.Context, projectID, label, description string) (*Version, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	versionID := uuid.NewString()
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

	return &Version{
		ID:          versionID,
		ProjectID:   projectID,
		Label:       label,
		Description: description,
		BlockCount:  blockCount,
		CreatedAt:   now,
	}, nil
}

func (s *SQLiteStore) GetVersion(ctx context.Context, versionID string) (*Version, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, label, description, block_count, created_at FROM versions WHERE id=?`,
		versionID)

	var v Version
	var createdStr string
	err := row.Scan(&v.ID, &v.ProjectID, &v.Label, &v.Description, &v.BlockCount, &createdStr)
	if err != nil {
		return nil, fmt.Errorf("scan version: %w", err)
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &v, nil
}

func (s *SQLiteStore) ListVersions(ctx context.Context, projectID string) ([]*Version, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, label, description, block_count, created_at
		 FROM versions WHERE project_id=? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()

	var result []*Version
	for rows.Next() {
		var v Version
		var createdStr string
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Label, &v.Description, &v.BlockCount, &createdStr); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		result = append(result, &v)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) Diff(ctx context.Context, fromVersionID, toVersionID string) (*VersionDiff, error) {
	diff := &VersionDiff{
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
			diff.Changes = append(diff.Changes, BlockChange{
				BlockID: id, ChangeType: ChangeAdded, NewHash: toHash,
			})
		} else if fromHash != toHash {
			diff.Changes = append(diff.Changes, BlockChange{
				BlockID: id, ChangeType: ChangeModified, OldHash: fromHash, NewHash: toHash,
			})
		}
	}
	for id, fromHash := range fromBlocks {
		if _, exists := toBlocks[id]; !exists {
			diff.Changes = append(diff.Changes, BlockChange{
				BlockID: id, ChangeType: ChangeRemoved, OldHash: fromHash,
			})
		}
	}

	return diff, nil
}

// ---------------------------------------------------------------------------
// KAZ export/import
// ---------------------------------------------------------------------------

func (s *SQLiteStore) ExportKAZ(ctx context.Context, projectID string, w io.Writer) error {
	proj, err := s.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	blocks, err := s.GetBlocks(ctx, BlockQuery{ProjectID: projectID})
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	versions, err := s.ListVersions(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list versions: %w", err)
	}

	var exportBlocks []kaz.ExportBlock
	for _, sb := range blocks {
		exportBlocks = append(exportBlocks, kaz.BlockToExport(sb.Block))
	}

	var exportVersions []kaz.ExportVersion
	for _, v := range versions {
		// Get block IDs for this version.
		rows, err := s.db.QueryContext(ctx,
			`SELECT block_id FROM version_blocks WHERE version_id=?`, v.ID)
		if err != nil {
			return fmt.Errorf("query version blocks: %w", err)
		}
		var blockIDs []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			blockIDs = append(blockIDs, id)
		}
		rows.Close()

		exportVersions = append(exportVersions, kaz.ExportVersion{
			ID:          v.ID,
			Label:       v.Label,
			Description: v.Description,
			BlockIDs:    blockIDs,
			CreatedAt:   v.CreatedAt.Format(time.RFC3339),
		})
	}

	locales := make([]string, len(proj.TargetLocales))
	for i, l := range proj.TargetLocales {
		locales[i] = string(l)
	}

	return kaz.ExportStore(w, kaz.StoreExportOptions{
		ProjectID:     proj.ID,
		ProjectName:   proj.Name,
		SourceLocale:  string(proj.SourceLocale),
		TargetLocales: locales,
		Blocks:        exportBlocks,
		Versions:      exportVersions,
	})
}

func (s *SQLiteStore) ImportKAZ(ctx context.Context, r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read KAZ data: %w", err)
	}

	pkg, err := kaz.ImportStoreFromBytes(data)
	if err != nil {
		return "", fmt.Errorf("parse KAZ: %w", err)
	}

	// Create project.
	projectID := pkg.Manifest.ProjectID
	if projectID == "" {
		projectID = uuid.NewString()
	}

	locales := make([]model.LocaleID, len(pkg.Manifest.TargetLocales))
	for i, l := range pkg.Manifest.TargetLocales {
		locales[i] = model.LocaleID(l)
	}

	proj := &Project{
		ID:            projectID,
		Name:          pkg.Manifest.ProjectName,
		SourceLocale:  model.LocaleID(pkg.Manifest.SourceLocale),
		TargetLocales: locales,
		Properties:    map[string]string{},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := s.CreateProject(ctx, proj); err != nil {
		return "", fmt.Errorf("create project: %w", err)
	}

	// Import blocks.
	var blocks []*model.Block
	for _, eb := range pkg.Blocks {
		blocks = append(blocks, kaz.ExportToBlock(eb))
	}
	if len(blocks) > 0 {
		if err := s.StoreBlocks(ctx, projectID, blocks); err != nil {
			return "", fmt.Errorf("store blocks: %w", err)
		}
	}

	return projectID, nil
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

type scanner interface {
	Scan(dest ...any) error
}

func scanProject(row scanner) (*Project, error) {
	var p Project
	var srcLocale, targetLocales, propsJSON, createdStr, updatedStr string
	err := row.Scan(&p.ID, &p.Name, &srcLocale, &targetLocales, &propsJSON, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan project: %w", err)
	}
	p.SourceLocale = model.LocaleID(srcLocale)
	p.TargetLocales = splitLocales(targetLocales)
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if err := json.Unmarshal([]byte(propsJSON), &p.Properties); err != nil {
		p.Properties = map[string]string{}
	}
	return &p, nil
}

func scanProjectRow(rows *sql.Rows) (*Project, error) {
	return scanProject(rows)
}

func scanStoredBlock(row scanner) (*StoredBlock, error) {
	var sb StoredBlock
	sb.Block = &model.Block{}
	var translatable int
	var sourceJSON, targetsJSON, propsJSON, annsJSON, storedStr, updatedStr string

	err := row.Scan(
		&sb.Block.ID, &sb.ProjectID, &sb.Block.Name, &sb.Block.Type,
		&sb.Block.MimeType, &translatable, &sb.ContentHash, &sb.ContextHash,
		&sourceJSON, &targetsJSON, &propsJSON, &annsJSON, &storedStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan block: %w", err)
	}

	sb.Block.Translatable = translatable != 0
	sb.StoredAt, _ = time.Parse(time.RFC3339, storedStr)
	sb.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	if err := json.Unmarshal([]byte(sourceJSON), &sb.Block.Source); err != nil {
		sb.Block.Source = nil
	}
	if err := json.Unmarshal([]byte(targetsJSON), &sb.Block.Targets); err != nil {
		sb.Block.Targets = make(map[model.LocaleID][]*model.Segment)
	}
	if err := json.Unmarshal([]byte(propsJSON), &sb.Block.Properties); err != nil {
		sb.Block.Properties = make(map[string]string)
	}
	// Annotations use an interface type; skip deserialization for now.
	sb.Block.Annotations = make(map[string]model.Annotation)

	return &sb, nil
}

func scanStoredBlockRow(rows *sql.Rows) (*StoredBlock, error) {
	return scanStoredBlock(rows)
}
