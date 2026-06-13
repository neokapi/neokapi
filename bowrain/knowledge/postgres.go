package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/id"
)

// PostgresKnowledgeStore implements knowledge.Store using PostgreSQL. The
// platform runs exclusively on PostgreSQL, so this is the only Store backend;
// there is no SQLite or in-memory variant. All methods are workspace-scoped.
//
// IDs and timestamps follow the surrounding store conventions: a store-assigned
// ID is generated with core/id.New when the caller leaves it empty, and zero
// timestamps are stamped with the current time. The change-set state machine is
// enforced by validating every status transition through the pure
// ValidateStatusTransition before any write.
type PostgresKnowledgeStore struct {
	db *storage.PgDB
}

// NewPostgresKnowledgeStore creates a PostgreSQL-backed knowledge store and
// applies the baseline schema under the knowledge_schema_migrations namespace.
func NewPostgresKnowledgeStore(db *storage.PgDB) (*PostgresKnowledgeStore, error) {
	if err := storage.MigratePostgresNS(db, "knowledge_schema_migrations", kgMigrations); err != nil {
		return nil, fmt.Errorf("knowledge migration: %w", err)
	}
	return &PostgresKnowledgeStore{db: db}, nil
}

// Close is a no-op; the caller owns the database connection.
func (s *PostgresKnowledgeStore) Close() error {
	return nil
}

// scanner is an alias for storage.Scanner, satisfied by *sql.Row and *sql.Rows.
type scanner = storage.Scanner

// ---------------------------------------------------------------------------
// Markets
// ---------------------------------------------------------------------------

func (s *PostgresKnowledgeStore) CreateMarket(ctx context.Context, m *Market) error {
	if m.ID == "" {
		m.ID = id.New()
	}
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	locales, err := json.Marshal(m.Locales)
	if err != nil {
		return fmt.Errorf("marshal locales: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO kg_markets (workspace_id, id, name, description, locales, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		m.WorkspaceID, m.ID, m.Name, m.Description, string(locales), m.CreatedAt, m.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert market: %w", err)
	}
	return nil
}

func (s *PostgresKnowledgeStore) GetMarket(ctx context.Context, workspaceID, marketID string) (*Market, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT workspace_id, id, name, description, locales, created_at, updated_at
		 FROM kg_markets WHERE workspace_id = $1 AND id = $2`, workspaceID, marketID)
	return scanMarket(row)
}

func (s *PostgresKnowledgeStore) UpdateMarket(ctx context.Context, m *Market) error {
	m.UpdatedAt = time.Now().UTC()

	locales, err := json.Marshal(m.Locales)
	if err != nil {
		return fmt.Errorf("marshal locales: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE kg_markets SET name = $1, description = $2, locales = $3, updated_at = $4
		 WHERE workspace_id = $5 AND id = $6`,
		m.Name, m.Description, string(locales), m.UpdatedAt, m.WorkspaceID, m.ID)
	if err != nil {
		return fmt.Errorf("update market: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("market %s not found", m.ID)
	}
	return nil
}

func (s *PostgresKnowledgeStore) DeleteMarket(ctx context.Context, workspaceID, marketID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM kg_markets WHERE workspace_id = $1 AND id = $2`, workspaceID, marketID)
	if err != nil {
		return fmt.Errorf("delete market: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("market %s not found", marketID)
	}
	return nil
}

func (s *PostgresKnowledgeStore) ListMarkets(ctx context.Context, workspaceID string) ([]*Market, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, id, name, description, locales, created_at, updated_at
		 FROM kg_markets WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list markets: %w", err)
	}
	defer rows.Close()

	var result []*Market
	for rows.Next() {
		m, err := scanMarket(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func scanMarket(row scanner) (*Market, error) {
	var m Market
	var localesJSON []byte
	err := row.Scan(&m.WorkspaceID, &m.ID, &m.Name, &m.Description, &localesJSON, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("market not found")
		}
		return nil, fmt.Errorf("scan market: %w", err)
	}
	if len(localesJSON) > 0 {
		if err := json.Unmarshal(localesJSON, &m.Locales); err != nil {
			return nil, fmt.Errorf("unmarshal locales: %w", err)
		}
	}
	m.CreatedAt = m.CreatedAt.UTC()
	m.UpdatedAt = m.UpdatedAt.UTC()
	return &m, nil
}

// ---------------------------------------------------------------------------
// Observations
// ---------------------------------------------------------------------------

func (s *PostgresKnowledgeStore) AddObservation(ctx context.Context, o *Observation) error {
	if o.ID == "" {
		o.ID = id.New()
	}
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO kg_observations (workspace_id, id, concept_id, kind, quote, source, url, locale, market, note, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		o.WorkspaceID, o.ID, o.ConceptID, string(o.Kind), o.Quote, o.Source, o.URL,
		string(o.Locale), o.Market, o.Note, o.CreatedBy, o.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert observation: %w", err)
	}
	return nil
}

func (s *PostgresKnowledgeStore) DeleteObservation(ctx context.Context, workspaceID, observationID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM kg_observations WHERE workspace_id = $1 AND id = $2`, workspaceID, observationID)
	if err != nil {
		return fmt.Errorf("delete observation: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("observation %s not found", observationID)
	}
	return nil
}

func (s *PostgresKnowledgeStore) ListObservationsByConcept(ctx context.Context, workspaceID, conceptID string) ([]*Observation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, id, concept_id, kind, quote, source, url, locale, market, note, created_by, created_at
		 FROM kg_observations WHERE workspace_id = $1 AND concept_id = $2
		 ORDER BY created_at DESC, id`, workspaceID, conceptID)
	if err != nil {
		return nil, fmt.Errorf("list observations: %w", err)
	}
	defer rows.Close()

	var result []*Observation
	for rows.Next() {
		o, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

func scanObservation(row scanner) (*Observation, error) {
	var o Observation
	var kind string
	err := row.Scan(&o.WorkspaceID, &o.ID, &o.ConceptID, &kind, &o.Quote, &o.Source,
		&o.URL, &o.Locale, &o.Market, &o.Note, &o.CreatedBy, &o.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("observation not found")
		}
		return nil, fmt.Errorf("scan observation: %w", err)
	}
	o.Kind = ObservationKind(kind)
	o.CreatedAt = o.CreatedAt.UTC()
	return &o, nil
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

func (s *PostgresKnowledgeStore) AddComment(ctx context.Context, c *Comment) error {
	if c.ID == "" {
		c.ID = id.New()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO kg_comments (workspace_id, id, concept_id, parent_id, changeset_id, body, author, created_at, resolved)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.WorkspaceID, c.ID, c.ConceptID, c.ParentID, c.ChangesetID, c.Body, c.Author, c.CreatedAt, c.Resolved)
	if err != nil {
		return fmt.Errorf("insert comment: %w", err)
	}
	return nil
}

func (s *PostgresKnowledgeStore) DeleteComment(ctx context.Context, workspaceID, commentID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM kg_comments WHERE workspace_id = $1 AND id = $2`, workspaceID, commentID)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("comment %s not found", commentID)
	}
	return nil
}

func (s *PostgresKnowledgeStore) ResolveComment(ctx context.Context, workspaceID, commentID string, resolved bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE kg_comments SET resolved = $1 WHERE workspace_id = $2 AND id = $3`,
		resolved, workspaceID, commentID)
	if err != nil {
		return fmt.Errorf("resolve comment: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("comment %s not found", commentID)
	}
	return nil
}

func (s *PostgresKnowledgeStore) ListCommentsByConcept(ctx context.Context, workspaceID, conceptID string) ([]*Comment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, id, concept_id, parent_id, changeset_id, body, author, created_at, resolved
		 FROM kg_comments WHERE workspace_id = $1 AND concept_id = $2
		 ORDER BY created_at, id`, workspaceID, conceptID)
	if err != nil {
		return nil, fmt.Errorf("list comments by concept: %w", err)
	}
	return collectComments(rows)
}

func (s *PostgresKnowledgeStore) ListCommentsByChangeset(ctx context.Context, workspaceID, changesetID string) ([]*Comment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, id, concept_id, parent_id, changeset_id, body, author, created_at, resolved
		 FROM kg_comments WHERE workspace_id = $1 AND changeset_id = $2
		 ORDER BY created_at, id`, workspaceID, changesetID)
	if err != nil {
		return nil, fmt.Errorf("list comments by changeset: %w", err)
	}
	return collectComments(rows)
}

func collectComments(rows *sql.Rows) ([]*Comment, error) {
	defer rows.Close()
	var result []*Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func scanComment(row scanner) (*Comment, error) {
	var c Comment
	err := row.Scan(&c.WorkspaceID, &c.ID, &c.ConceptID, &c.ParentID, &c.ChangesetID,
		&c.Body, &c.Author, &c.CreatedAt, &c.Resolved)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("comment not found")
		}
		return nil, fmt.Errorf("scan comment: %w", err)
	}
	c.CreatedAt = c.CreatedAt.UTC()
	return &c, nil
}

// ---------------------------------------------------------------------------
// Concept revisions
// ---------------------------------------------------------------------------

// AddRevision appends an immutable revision snapshot. When the caller supplies a
// positive Rev it is honored and the (workspace_id, concept_id, rev) primary key
// guards against duplicates; when Rev is zero the next revision is assigned as
// MAX(rev)+1 under a per-concept advisory lock, which serializes concurrent
// auto-numbered inserts so two writers cannot both read the same MAX and collide
// on the primary key. (concept_id has no parent row to lock FOR UPDATE — it is
// plain TEXT, not a foreign key — so the lock is taken on a hash of the concept,
// matching the audit-chain pattern in event/audit.go.)
func (s *PostgresKnowledgeStore) AddRevision(ctx context.Context, r *ConceptRevision) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	snapshot := string(r.Snapshot)
	if snapshot == "" {
		snapshot = "null"
	}

	if r.Rev > 0 {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO kg_concept_revisions (workspace_id, concept_id, rev, snapshot, summary, actor, changeset_id, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			r.WorkspaceID, r.ConceptID, r.Rev, snapshot, r.Summary, r.Actor, r.ChangesetID, r.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert concept revision: %w", err)
		}
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin revision tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize concurrent auto-numbered inserts for this concept so the
	// MAX(rev)+1 read-then-insert below cannot race into a duplicate rev.
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('kg_concept_revisions'), hashtext($1))`,
		r.WorkspaceID+"\x1e"+r.ConceptID); err != nil {
		return fmt.Errorf("acquire revision lock: %w", err)
	}

	var next int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(rev), 0) + 1 FROM kg_concept_revisions WHERE workspace_id = $1 AND concept_id = $2`,
		r.WorkspaceID, r.ConceptID).Scan(&next); err != nil {
		return fmt.Errorf("compute next revision: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO kg_concept_revisions (workspace_id, concept_id, rev, snapshot, summary, actor, changeset_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		r.WorkspaceID, r.ConceptID, next, snapshot, r.Summary, r.Actor, r.ChangesetID, r.CreatedAt); err != nil {
		return fmt.Errorf("insert concept revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit revision: %w", err)
	}
	r.Rev = next
	return nil
}

func (s *PostgresKnowledgeStore) ListRevisions(ctx context.Context, workspaceID, conceptID string) ([]*ConceptRevision, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, concept_id, rev, snapshot, summary, actor, changeset_id, created_at
		 FROM kg_concept_revisions WHERE workspace_id = $1 AND concept_id = $2
		 ORDER BY rev`, workspaceID, conceptID)
	if err != nil {
		return nil, fmt.Errorf("list revisions: %w", err)
	}
	defer rows.Close()

	var result []*ConceptRevision
	for rows.Next() {
		r, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *PostgresKnowledgeStore) LatestRev(ctx context.Context, workspaceID, conceptID string) (int64, error) {
	var rev int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(rev), 0) FROM kg_concept_revisions WHERE workspace_id = $1 AND concept_id = $2`,
		workspaceID, conceptID).Scan(&rev)
	if err != nil {
		return 0, fmt.Errorf("latest revision: %w", err)
	}
	return rev, nil
}

func scanRevision(row scanner) (*ConceptRevision, error) {
	var r ConceptRevision
	var snapshot []byte
	err := row.Scan(&r.WorkspaceID, &r.ConceptID, &r.Rev, &snapshot, &r.Summary, &r.Actor, &r.ChangesetID, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("concept revision not found")
		}
		return nil, fmt.Errorf("scan concept revision: %w", err)
	}
	if len(snapshot) > 0 {
		r.Snapshot = json.RawMessage(snapshot)
	}
	r.CreatedAt = r.CreatedAt.UTC()
	return &r, nil
}

// ---------------------------------------------------------------------------
// Change-sets
// ---------------------------------------------------------------------------

func (s *PostgresKnowledgeStore) CreateChangeSet(ctx context.Context, cs *ChangeSet) error {
	if cs.ID == "" {
		cs.ID = id.New()
	}
	now := time.Now().UTC()
	if cs.CreatedAt.IsZero() {
		cs.CreatedAt = now
	}
	cs.UpdatedAt = now
	if cs.Status == "" {
		cs.Status = ChangeSetDraft
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO kg_changesets (workspace_id, id, name, description, status, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		cs.WorkspaceID, cs.ID, cs.Name, cs.Description, string(cs.Status), cs.CreatedBy, cs.CreatedAt, cs.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert change-set: %w", err)
	}
	return nil
}

func (s *PostgresKnowledgeStore) GetChangeSet(ctx context.Context, workspaceID, changesetID string) (*ChangeSet, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT workspace_id, id, name, description, status, created_by, created_at, updated_at, submitted_at, merged_at, merged_by
		 FROM kg_changesets WHERE workspace_id = $1 AND id = $2`, workspaceID, changesetID)
	return scanChangeSet(row)
}

func (s *PostgresKnowledgeStore) ListChangeSets(ctx context.Context, workspaceID string, status ChangeSetStatus) ([]*ChangeSet, error) {
	const cols = `workspace_id, id, name, description, status, created_by, created_at, updated_at, submitted_at, merged_at, merged_by`

	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+cols+` FROM kg_changesets WHERE workspace_id = $1 ORDER BY created_at DESC, id`, workspaceID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+cols+` FROM kg_changesets WHERE workspace_id = $1 AND status = $2 ORDER BY created_at DESC, id`,
			workspaceID, string(status))
	}
	if err != nil {
		return nil, fmt.Errorf("list change-sets: %w", err)
	}
	defer rows.Close()

	var result []*ChangeSet
	for rows.Next() {
		cs, err := scanChangeSet(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, cs)
	}
	return result, rows.Err()
}

func (s *PostgresKnowledgeStore) SetChangeSetStatus(ctx context.Context, workspaceID, changesetID string, to ChangeSetStatus) error {
	if to == ChangeSetMerged {
		return errors.New("use SetMergeResult to merge a change-set, not SetChangeSetStatus")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin status tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var fromStr string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM kg_changesets WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
		workspaceID, changesetID).Scan(&fromStr)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("change-set %s not found", changesetID)
	}
	if err != nil {
		return fmt.Errorf("load change-set status: %w", err)
	}

	from := ChangeSetStatus(fromStr)
	if err := ValidateStatusTransition(from, to); err != nil {
		return err
	}

	now := time.Now().UTC()
	if from == ChangeSetDraft && to == ChangeSetInReview {
		_, err = tx.ExecContext(ctx,
			`UPDATE kg_changesets SET status = $1, submitted_at = $2, updated_at = $2
			 WHERE workspace_id = $3 AND id = $4`,
			string(to), now, workspaceID, changesetID)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE kg_changesets SET status = $1, updated_at = $2
			 WHERE workspace_id = $3 AND id = $4`,
			string(to), now, workspaceID, changesetID)
	}
	if err != nil {
		return fmt.Errorf("update change-set status: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit change-set status: %w", err)
	}
	return nil
}

func (s *PostgresKnowledgeStore) SetMergeResult(ctx context.Context, workspaceID, changesetID, mergedBy string, mergedAt time.Time) error {
	if mergedAt.IsZero() {
		mergedAt = time.Now().UTC()
	} else {
		mergedAt = mergedAt.UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin merge tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var fromStr string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM kg_changesets WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
		workspaceID, changesetID).Scan(&fromStr)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("change-set %s not found", changesetID)
	}
	if err != nil {
		return fmt.Errorf("load change-set status: %w", err)
	}

	if err := ValidateStatusTransition(ChangeSetStatus(fromStr), ChangeSetMerged); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE kg_changesets SET status = $1, merged_by = $2, merged_at = $3, updated_at = $3
		 WHERE workspace_id = $4 AND id = $5`,
		string(ChangeSetMerged), mergedBy, mergedAt, workspaceID, changesetID); err != nil {
		return fmt.Errorf("update merge result: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit merge result: %w", err)
	}
	return nil
}

func scanChangeSet(row scanner) (*ChangeSet, error) {
	var cs ChangeSet
	var status string
	var submittedAt, mergedAt sql.NullTime
	err := row.Scan(&cs.WorkspaceID, &cs.ID, &cs.Name, &cs.Description, &status,
		&cs.CreatedBy, &cs.CreatedAt, &cs.UpdatedAt, &submittedAt, &mergedAt, &cs.MergedBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("change-set not found")
		}
		return nil, fmt.Errorf("scan change-set: %w", err)
	}
	cs.Status = ChangeSetStatus(status)
	cs.CreatedAt = cs.CreatedAt.UTC()
	cs.UpdatedAt = cs.UpdatedAt.UTC()
	if submittedAt.Valid {
		t := submittedAt.Time.UTC()
		cs.SubmittedAt = &t
	}
	if mergedAt.Valid {
		t := mergedAt.Time.UTC()
		cs.MergedAt = &t
	}
	return &cs, nil
}

// ---------------------------------------------------------------------------
// Change-set ops
// ---------------------------------------------------------------------------

// AppendOp assigns the next Seq within the change-set and persists the op; any
// Seq the caller set is ignored. It locks the parent change-set row FOR UPDATE
// before reading MAX(seq)+1, so concurrent appends to the same change-set
// serialize and cannot both read the same MAX and collide on the
// (workspace_id, changeset_id, seq) primary key.
func (s *PostgresKnowledgeStore) AppendOp(ctx context.Context, op *ChangeSetOp) error {
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now().UTC()
	}
	payload := string(op.Payload)
	if payload == "" {
		payload = "null"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin op tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the parent change-set row so concurrent appends serialize; this also
	// confirms the change-set exists before we number an orphan op.
	var locked int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM kg_changesets WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
		op.WorkspaceID, op.ChangesetID).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("change-set %s not found", op.ChangesetID)
	}
	if err != nil {
		return fmt.Errorf("lock change-set: %w", err)
	}

	var next int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM kg_changeset_ops WHERE workspace_id = $1 AND changeset_id = $2`,
		op.WorkspaceID, op.ChangesetID).Scan(&next); err != nil {
		return fmt.Errorf("compute next op seq: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO kg_changeset_ops (workspace_id, changeset_id, seq, op, payload, base_rev, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		op.WorkspaceID, op.ChangesetID, next, string(op.Op), payload, op.BaseRev, op.CreatedBy, op.CreatedAt); err != nil {
		return fmt.Errorf("insert changeset op: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit changeset op: %w", err)
	}
	op.Seq = next
	return nil
}

func (s *PostgresKnowledgeStore) RemoveOp(ctx context.Context, workspaceID, changesetID string, seq int64) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM kg_changeset_ops WHERE workspace_id = $1 AND changeset_id = $2 AND seq = $3`,
		workspaceID, changesetID, seq)
	if err != nil {
		return fmt.Errorf("delete changeset op: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("change-set op %s/%d not found", changesetID, seq)
	}
	return nil
}

func (s *PostgresKnowledgeStore) ListOps(ctx context.Context, workspaceID, changesetID string) ([]*ChangeSetOp, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, changeset_id, seq, op, payload, base_rev, created_by, created_at
		 FROM kg_changeset_ops WHERE workspace_id = $1 AND changeset_id = $2
		 ORDER BY seq`, workspaceID, changesetID)
	if err != nil {
		return nil, fmt.Errorf("list changeset ops: %w", err)
	}
	defer rows.Close()

	var result []*ChangeSetOp
	for rows.Next() {
		op, err := scanOp(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, rows.Err()
}

func scanOp(row scanner) (*ChangeSetOp, error) {
	var op ChangeSetOp
	var opType string
	var payload []byte
	err := row.Scan(&op.WorkspaceID, &op.ChangesetID, &op.Seq, &opType, &payload, &op.BaseRev, &op.CreatedBy, &op.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("change-set op not found")
		}
		return nil, fmt.Errorf("scan changeset op: %w", err)
	}
	op.Op = OpType(opType)
	if len(payload) > 0 {
		op.Payload = json.RawMessage(payload)
	}
	op.CreatedAt = op.CreatedAt.UTC()
	return &op, nil
}

// ---------------------------------------------------------------------------
// Reviews
// ---------------------------------------------------------------------------

// AddReview upserts a reviewer's verdict, keyed by (workspace_id, changeset_id,
// reviewer): a reviewer changing their mind replaces their prior verdict.
func (s *PostgresKnowledgeStore) AddReview(ctx context.Context, r *ChangeSetReview) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO kg_changeset_reviews (workspace_id, changeset_id, reviewer, verdict, comment, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (workspace_id, changeset_id, reviewer) DO UPDATE SET
		   verdict = EXCLUDED.verdict,
		   comment = EXCLUDED.comment,
		   created_at = EXCLUDED.created_at`,
		r.WorkspaceID, r.ChangesetID, r.Reviewer, string(r.Verdict), r.Comment, r.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert review: %w", err)
	}
	return nil
}

func (s *PostgresKnowledgeStore) ListReviews(ctx context.Context, workspaceID, changesetID string) ([]*ChangeSetReview, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, changeset_id, reviewer, verdict, comment, created_at
		 FROM kg_changeset_reviews WHERE workspace_id = $1 AND changeset_id = $2
		 ORDER BY created_at, reviewer`, workspaceID, changesetID)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	defer rows.Close()

	var result []*ChangeSetReview
	for rows.Next() {
		rv, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, rv)
	}
	return result, rows.Err()
}

func scanReview(row scanner) (*ChangeSetReview, error) {
	var r ChangeSetReview
	var verdict string
	err := row.Scan(&r.WorkspaceID, &r.ChangesetID, &r.Reviewer, &verdict, &r.Comment, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("review not found")
		}
		return nil, fmt.Errorf("scan review: %w", err)
	}
	r.Verdict = ReviewVerdict(verdict)
	r.CreatedAt = r.CreatedAt.UTC()
	return &r, nil
}

// ---------------------------------------------------------------------------
// Pilots
// ---------------------------------------------------------------------------

// AddPilot binds a change-set to a project stream. The binding is idempotent: a
// re-bind of the same (change-set, project, stream) leaves the existing row.
func (s *PostgresKnowledgeStore) AddPilot(ctx context.Context, p *Pilot) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO kg_pilots (workspace_id, changeset_id, project_id, stream, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (workspace_id, changeset_id, project_id, stream) DO NOTHING`,
		p.WorkspaceID, p.ChangesetID, p.ProjectID, p.Stream, p.CreatedBy, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert pilot: %w", err)
	}
	return nil
}

func (s *PostgresKnowledgeStore) RemovePilot(ctx context.Context, workspaceID, changesetID, projectID, stream string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM kg_pilots
		 WHERE workspace_id = $1 AND changeset_id = $2 AND project_id = $3 AND stream = $4`,
		workspaceID, changesetID, projectID, stream)
	if err != nil {
		return fmt.Errorf("delete pilot: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("pilot %s/%s/%s not found", changesetID, projectID, stream)
	}
	return nil
}

func (s *PostgresKnowledgeStore) ListPilots(ctx context.Context, workspaceID, changesetID string) ([]*Pilot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, changeset_id, project_id, stream, created_by, created_at
		 FROM kg_pilots WHERE workspace_id = $1 AND changeset_id = $2
		 ORDER BY created_at, project_id, stream`, workspaceID, changesetID)
	if err != nil {
		return nil, fmt.Errorf("list pilots: %w", err)
	}
	return collectPilots(rows)
}

func (s *PostgresKnowledgeStore) ListPilotsForStream(ctx context.Context, workspaceID, projectID, stream string) ([]*Pilot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, changeset_id, project_id, stream, created_by, created_at
		 FROM kg_pilots WHERE workspace_id = $1 AND project_id = $2 AND stream = $3
		 ORDER BY created_at, changeset_id`, workspaceID, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list pilots for stream: %w", err)
	}
	return collectPilots(rows)
}

func collectPilots(rows *sql.Rows) ([]*Pilot, error) {
	defer rows.Close()
	var result []*Pilot
	for rows.Next() {
		p, err := scanPilot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func scanPilot(row scanner) (*Pilot, error) {
	var p Pilot
	err := row.Scan(&p.WorkspaceID, &p.ChangesetID, &p.ProjectID, &p.Stream, &p.CreatedBy, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("pilot not found")
		}
		return nil, fmt.Errorf("scan pilot: %w", err)
	}
	p.CreatedAt = p.CreatedAt.UTC()
	return &p, nil
}
