package store

import (
	"context"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/platform/store"
)

// ---------------------------------------------------------------------------
// Stream CRUD (PostgreSQL)
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateStream(ctx context.Context, st *platstore.Stream) error {
	if st.Name == "" {
		return fmt.Errorf("stream name cannot be empty")
	}
	// "main" can now be created explicitly (e.g. during project setup).
	if st.Visibility == "" {
		st.Visibility = platstore.StreamPublic
	}
	now := time.Now().UTC()
	st.CreatedAt = now

	if st.Parent != "" {
		parent := defaultStream(st.Parent)
		cursor, err := s.LatestCursor(ctx, st.ProjectID, parent)
		if err != nil {
			return fmt.Errorf("get parent cursor: %w", err)
		}
		st.BaseCursor = cursor
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO streams (project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		st.ProjectID, st.Name, st.Parent, st.BaseCursor, st.Archived,
		string(st.Visibility), st.Description, now, st.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert stream: %w", err)
	}

	// Copy items from the parent stream into the new stream.
	parentStream := defaultStream(st.Parent)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO items (id, project_id, stream, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at)
		 SELECT substr(md5(random()::text), 1, 8), project_id, $1, name, format, item_type, block_index, preview_html, properties, collection_id, $2, $2
		 FROM items WHERE project_id = $3 AND stream = $4`,
		st.Name, now, st.ProjectID, parentStream)
	if err != nil {
		return fmt.Errorf("copy parent items: %w", err)
	}

	return nil
}

func (s *PostgresStore) GetStream(ctx context.Context, projectID, name string) (*platstore.Stream, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by
		 FROM streams WHERE project_id = $1 AND name = $2`, projectID, name)
	return scanStreamPg(row)
}

func (s *PostgresStore) ListStreams(ctx context.Context, projectID string, includeArchived bool) ([]*platstore.Stream, error) {
	var query string
	var args []any
	if includeArchived {
		query = `SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by
				 FROM streams WHERE project_id = $1 ORDER BY name`
		args = []any{projectID}
	} else {
		query = `SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by
				 FROM streams WHERE project_id = $1 AND archived = FALSE ORDER BY name`
		args = []any{projectID}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	defer rows.Close()

	var result []*platstore.Stream
	for rows.Next() {
		st, err := scanStreamPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, st)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateStream(ctx context.Context, st *platstore.Stream) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE streams SET parent=$1, base_cursor=$2, archived=$3, visibility=$4, description=$5
		 WHERE project_id=$6 AND name=$7`,
		st.Parent, st.BaseCursor, st.Archived, string(st.Visibility),
		st.Description, st.ProjectID, st.Name)
	if err != nil {
		return fmt.Errorf("update stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("stream %q not found in project %s", st.Name, st.ProjectID)
	}
	return nil
}

func (s *PostgresStore) DeleteStream(ctx context.Context, projectID, name string) error {
	if name == "main" {
		return fmt.Errorf("cannot delete the main stream")
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM streams WHERE project_id=$1 AND name=$2`, projectID, name)
	if err != nil {
		return fmt.Errorf("delete stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("stream %q not found in project %s", name, projectID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Stream operations
// ---------------------------------------------------------------------------

func (s *PostgresStore) MergeStream(ctx context.Context, projectID, streamName string, opts platstore.MergeOptions) (*platstore.MergeResult, error) {
	stream, err := s.GetStream(ctx, projectID, streamName)
	if err != nil {
		return nil, fmt.Errorf("get stream: %w", err)
	}
	if stream.Parent == "" {
		return nil, fmt.Errorf("stream %q has no parent to merge into", streamName)
	}

	parentStream := defaultStream(stream.Parent)

	changes, err := s.GetChanges(ctx, projectID, streamName, stream.BaseCursor, nil, MaxChangesPerRequest)
	if err != nil {
		return nil, fmt.Errorf("get stream changes: %w", err)
	}

	result := &platstore.MergeResult{}
	blockChanges := map[string]string{}
	for _, c := range changes.Changes {
		blockChanges[c.BlockID] = c.ChangeType
	}

	for blockID, changeType := range blockChanges {
		var ct platstore.ChangeType
		switch {
		case changeType == "source_added":
			ct = platstore.ChangeAdded
			result.AddedBlocks++
		case changeType == "source_removed":
			ct = platstore.ChangeRemoved
			result.RemovedBlocks++
		default:
			ct = platstore.ChangeModified
			result.ModifiedBlocks++
		}
		result.Changes = append(result.Changes, platstore.BlockChange{
			BlockID:    blockID,
			ChangeType: ct,
		})
	}
	result.MergedBlocks = len(blockChanges)

	if opts.DryRun {
		return result, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for blockID, changeType := range blockChanges {
		if changeType == "source_removed" {
			continue
		}
		if err := logChangePg(ctx, tx, projectID, parentStream, blockID, changeType, "", ""); err != nil {
			return nil, fmt.Errorf("log merge change: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit merge: %w", err)
	}

	return result, nil
}

func (s *PostgresStore) DiffStream(ctx context.Context, projectID, streamName string) (*platstore.StreamDiff, error) {
	stream, err := s.GetStream(ctx, projectID, streamName)
	if err != nil {
		return nil, fmt.Errorf("get stream: %w", err)
	}

	parentName := defaultStream(stream.Parent)

	changes, err := s.GetChanges(ctx, projectID, streamName, stream.BaseCursor, nil, MaxChangesPerRequest)
	if err != nil {
		return nil, fmt.Errorf("get stream changes: %w", err)
	}

	diff := &platstore.StreamDiff{
		StreamName: streamName,
		ParentName: parentName,
	}

	blockChanges := map[string]string{}
	for _, c := range changes.Changes {
		blockChanges[c.BlockID] = c.ChangeType
	}

	for blockID, changeType := range blockChanges {
		var ct platstore.ChangeType
		switch {
		case changeType == "source_added":
			ct = platstore.ChangeAdded
		case changeType == "source_removed":
			ct = platstore.ChangeRemoved
		default:
			ct = platstore.ChangeModified
		}
		diff.Changes = append(diff.Changes, platstore.BlockChange{
			BlockID:    blockID,
			ChangeType: ct,
		})
	}

	return diff, nil
}

// ---------------------------------------------------------------------------
// Stream membership
// ---------------------------------------------------------------------------

func (s *PostgresStore) AddStreamMember(ctx context.Context, projectID, streamName, userID string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO stream_members (project_id, stream, user_id, added_at) VALUES ($1, $2, $3, $4)
		 ON CONFLICT(project_id, stream, user_id) DO NOTHING`,
		projectID, streamName, userID, now)
	if err != nil {
		return fmt.Errorf("add stream member: %w", err)
	}
	return nil
}

func (s *PostgresStore) RemoveStreamMember(ctx context.Context, projectID, streamName, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM stream_members WHERE project_id=$1 AND stream=$2 AND user_id=$3`,
		projectID, streamName, userID)
	if err != nil {
		return fmt.Errorf("remove stream member: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("member %s not found in stream %s/%s", userID, projectID, streamName)
	}
	return nil
}

func (s *PostgresStore) ListStreamMembers(ctx context.Context, projectID, streamName string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id FROM stream_members WHERE project_id=$1 AND stream=$2 ORDER BY user_id`,
		projectID, streamName)
	if err != nil {
		return nil, fmt.Errorf("list stream members: %w", err)
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("scan stream member: %w", err)
		}
		members = append(members, uid)
	}
	return members, rows.Err()
}

// ---------------------------------------------------------------------------
// Scan helper
// ---------------------------------------------------------------------------

func scanStreamPg(row scanner) (*platstore.Stream, error) {
	var st platstore.Stream
	var visibility string
	err := row.Scan(&st.ProjectID, &st.Name, &st.Parent, &st.BaseCursor,
		&st.Archived, &visibility, &st.Description, &st.CreatedAt, &st.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("scan stream: %w", err)
	}
	st.Visibility = platstore.StreamVisibility(visibility)
	return &st, nil
}
