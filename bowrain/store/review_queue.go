package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gokapi/gokapi/core/id"
)

// ReviewItemType classifies review queue items.
type ReviewItemType string

const (
	ReviewItemTermCandidate ReviewItemType = "term_candidate"
	ReviewItemEntityReview  ReviewItemType = "entity_review"
)

// ReviewItemStatus tracks review lifecycle.
type ReviewItemStatus string

const (
	ReviewItemPending  ReviewItemStatus = "pending"
	ReviewItemAssigned ReviewItemStatus = "assigned"
	ReviewItemApproved ReviewItemStatus = "approved"
	ReviewItemRejected ReviewItemStatus = "rejected"
)

// Occurrence records where a term/entity appears.
type Occurrence struct {
	BlockID  string `json:"block_id"`
	FileID   string `json:"file_id,omitempty"`
	FilePath string `json:"file_path,omitempty"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Context  string `json:"context,omitempty"` // surrounding text snippet
}

// ReviewItem is a term candidate or entity awaiting review.
type ReviewItem struct {
	ID          string           `json:"id"`
	ProjectID   string           `json:"project_id"`
	Type        ReviewItemType   `json:"type"`
	Status      ReviewItemStatus `json:"status"`
	PushID      string           `json:"push_id,omitempty"`
	Data        json.RawMessage  `json:"data"`        // TermCandidateAnnotation or EntityAnnotation JSON
	Occurrences []Occurrence     `json:"occurrences"`
	AssignedTo  string           `json:"assigned_to,omitempty"`
	DecidedBy   string           `json:"decided_by,omitempty"`
	DecidedAt   *time.Time       `json:"decided_at,omitempty"`
	Comment     string           `json:"comment,omitempty"`
	Edits       json.RawMessage  `json:"edits,omitempty"` // user-applied edits on approval
	Confidence  float64          `json:"confidence"`
	Locale      string           `json:"locale"`
	CreatedAt   time.Time        `json:"created_at"`
}

// ReviewQueueQuery defines filters for listing review items.
type ReviewQueueQuery struct {
	ProjectID  string
	Type       ReviewItemType   // empty = all
	Status     ReviewItemStatus // empty = all
	AssignedTo string           // "me" or user ID; empty = all
	Confidence string           // "high" or "low"; empty = all
	Locale     string           // empty = all
	Limit      int
	Cursor     string // created_at cursor for pagination
}

// ReviewQueueResult is a paginated result set.
type ReviewQueueResult struct {
	Items      []ReviewItem `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
	Total      int          `json:"total"`
	Remaining  int          `json:"remaining"`
}

// DecideRequest captures a review decision.
type DecideRequest struct {
	Decision string          `json:"decision"` // "approve" or "reject"
	Comment  string          `json:"comment,omitempty"`
	Edits    json.RawMessage `json:"edits,omitempty"`
	UserID   string          `json:"user_id"`
}

// ReviewQueueStore persists review queue items.
type ReviewQueueStore struct {
	db      *sql.DB
	dialect Dialect
}

// NewReviewQueueStore creates a new review queue store sharing the given database (SQLite).
func NewReviewQueueStore(db *sql.DB) *ReviewQueueStore {
	return &ReviewQueueStore{db: db, dialect: DialectSQLite}
}

// NewPostgresReviewQueueStore creates a review queue store backed by PostgreSQL.
func NewPostgresReviewQueueStore(db *sql.DB) *ReviewQueueStore {
	return &ReviewQueueStore{db: db, dialect: DialectPostgres}
}

func (s *ReviewQueueStore) q(query string) string {
	return Rebind(s.dialect, query)
}

// CreateItem inserts a new review item. ID and CreatedAt are auto-set if empty/zero.
func (s *ReviewQueueStore) CreateItem(ctx context.Context, item *ReviewItem) error {
	if item.ID == "" {
		item.ID = id.New()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if item.Status == "" {
		item.Status = ReviewItemPending
	}

	occJSON, _ := json.Marshal(item.Occurrences)
	edits := "{}"
	if len(item.Edits) > 0 {
		edits = string(item.Edits)
	}

	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO review_items (id, project_id, type, status, push_id, data, occurrences,
		 assigned_to, decided_by, decided_at, comment, edits, confidence, locale, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, ?, ?, ?, ?)`),
		item.ID, item.ProjectID, string(item.Type), string(item.Status),
		item.PushID, string(item.Data), string(occJSON),
		item.AssignedTo, item.Comment, edits,
		item.Confidence, item.Locale,
		item.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

// GetItem retrieves a single review item by ID.
func (s *ReviewQueueStore) GetItem(ctx context.Context, id string) (*ReviewItem, error) {
	row := s.db.QueryRowContext(ctx, s.q(
		`SELECT id, project_id, type, status, push_id, data, occurrences,
		 assigned_to, decided_by, decided_at, comment, edits, confidence, locale, created_at
		 FROM review_items WHERE id = ?`), id)
	return s.scanReviewItem(row)
}

// ListItems returns review items matching the query.
func (s *ReviewQueueStore) ListItems(ctx context.Context, q ReviewQueueQuery) (*ReviewQueueResult, error) {
	where := "project_id = ?"
	args := []any{q.ProjectID}

	if q.Type != "" {
		where += " AND type = ?"
		args = append(args, string(q.Type))
	}
	if q.Status != "" {
		where += " AND status = ?"
		args = append(args, string(q.Status))
	}
	if q.AssignedTo != "" {
		where += " AND assigned_to = ?"
		args = append(args, q.AssignedTo)
	}
	if q.Locale != "" {
		where += " AND locale = ?"
		args = append(args, q.Locale)
	}
	if q.Cursor != "" {
		where += " AND created_at > ?"
		args = append(args, q.Cursor)
	}

	// Get total count for this query (without pagination).
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM review_items WHERE %s", where)
	if err := s.db.QueryRowContext(ctx, s.q(countQuery), args...).Scan(&total); err != nil {
		return nil, err
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(
		`SELECT id, project_id, type, status, push_id, data, occurrences,
		 assigned_to, decided_by, decided_at, comment, edits, confidence, locale, created_at
		 FROM review_items WHERE %s ORDER BY created_at ASC LIMIT ?`, where)
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ReviewItem
	for rows.Next() {
		item, err := s.scanReviewItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &ReviewQueueResult{Total: total}
	if len(items) > limit {
		result.Items = items[:limit]
		result.NextCursor = items[limit-1].CreatedAt.Format(time.RFC3339)
	} else {
		result.Items = items
	}

	// Count remaining pending items for this project.
	countArgs := []any{q.ProjectID}
	remainQuery := "SELECT COUNT(*) FROM review_items WHERE project_id = ? AND status = 'pending'"
	if err := s.db.QueryRowContext(ctx, s.q(remainQuery), countArgs...).Scan(&result.Remaining); err != nil {
		return nil, err
	}

	return result, nil
}

// Decide applies a review decision to an item.
func (s *ReviewQueueStore) Decide(ctx context.Context, itemID string, req DecideRequest) error {
	status := ReviewItemApproved
	if req.Decision == "reject" {
		status = ReviewItemRejected
	}

	edits := "{}"
	if len(req.Edits) > 0 {
		edits = string(req.Edits)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, s.q(
		`UPDATE review_items SET status = ?, decided_by = ?, decided_at = ?, comment = ?, edits = ?
		 WHERE id = ? AND status IN ('pending', 'assigned')`),
		string(status), req.UserID, now, req.Comment, edits, itemID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("review item %s not found or already decided", itemID)
	}
	return nil
}

// BatchDecide applies the same decision to multiple items.
func (s *ReviewQueueStore) BatchDecide(ctx context.Context, itemIDs []string, req DecideRequest) (int, error) {
	if len(itemIDs) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	status := ReviewItemApproved
	if req.Decision == "reject" {
		status = ReviewItemRejected
	}

	edits := "{}"
	if len(req.Edits) > 0 {
		edits = string(req.Edits)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var decided int
	for _, id := range itemIDs {
		result, err := tx.ExecContext(ctx, s.q(
			`UPDATE review_items SET status = ?, decided_by = ?, decided_at = ?, comment = ?, edits = ?
			 WHERE id = ? AND status IN ('pending', 'assigned')`),
			string(status), req.UserID, now, req.Comment, edits, id)
		if err != nil {
			return 0, err
		}
		affected, _ := result.RowsAffected()
		decided += int(affected)
	}

	return decided, tx.Commit()
}

// Assign assigns a review item to a user.
func (s *ReviewQueueStore) Assign(ctx context.Context, itemID, userID string) error {
	result, err := s.db.ExecContext(ctx, s.q(
		`UPDATE review_items SET assigned_to = ?, status = 'assigned'
		 WHERE id = ? AND status IN ('pending', 'assigned')`),
		userID, itemID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("review item %s not found or already decided", itemID)
	}
	return nil
}

// SplitItem removes specified occurrences from an existing item and creates a new item with those occurrences.
func (s *ReviewQueueStore) SplitItem(ctx context.Context, itemID string, occurrenceBlockIDs []string) (*ReviewItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Load existing item.
	row := tx.QueryRowContext(ctx, s.q(
		`SELECT id, project_id, type, status, push_id, data, occurrences,
		 assigned_to, decided_by, decided_at, comment, edits, confidence, locale, created_at
		 FROM review_items WHERE id = ?`), itemID)
	original, err := s.scanReviewItem(row)
	if err != nil {
		return nil, err
	}

	// Split occurrences.
	splitSet := make(map[string]bool, len(occurrenceBlockIDs))
	for _, bid := range occurrenceBlockIDs {
		splitSet[bid] = true
	}

	var keep, split []Occurrence
	for _, occ := range original.Occurrences {
		if splitSet[occ.BlockID] {
			split = append(split, occ)
		} else {
			keep = append(keep, occ)
		}
	}

	if len(split) == 0 || len(keep) == 0 {
		return nil, fmt.Errorf("split must leave occurrences on both items")
	}

	// Update original.
	keepJSON, _ := json.Marshal(keep)
	if _, err := tx.ExecContext(ctx, s.q(
		`UPDATE review_items SET occurrences = ? WHERE id = ?`),
		string(keepJSON), itemID); err != nil {
		return nil, err
	}

	// Create new item.
	newItem := &ReviewItem{
		ID:         id.New(),
		ProjectID:  original.ProjectID,
		Type:       original.Type,
		Status:     ReviewItemPending,
		PushID:     original.PushID,
		Data:       original.Data,
		Occurrences: split,
		Confidence: original.Confidence,
		Locale:     original.Locale,
		CreatedAt:  time.Now().UTC(),
	}

	splitJSON, _ := json.Marshal(split)
	if _, err := tx.ExecContext(ctx, s.q(
		`INSERT INTO review_items (id, project_id, type, status, push_id, data, occurrences,
		 assigned_to, decided_by, decided_at, comment, edits, confidence, locale, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '', '', '', '', '{}', ?, ?, ?)`),
		newItem.ID, newItem.ProjectID, string(newItem.Type), string(newItem.Status),
		newItem.PushID, string(newItem.Data), string(splitJSON),
		newItem.Confidence, newItem.Locale,
		newItem.CreatedAt.UTC().Format(time.RFC3339),
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return newItem, nil
}

// AddRejectedTerm records a rejected term to prevent re-proposal.
func (s *ReviewQueueStore) AddRejectedTerm(ctx context.Context, projectID, termText, locale string) error {
	q := `INSERT INTO rejected_terms (project_id, term_text, locale) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`
	_, err := s.db.ExecContext(ctx, s.q(q), projectID, termText, locale)
	return err
}

// IsRejected checks if a term was previously rejected.
func (s *ReviewQueueStore) IsRejected(ctx context.Context, projectID, termText, locale string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, s.q(
		`SELECT COUNT(*) FROM rejected_terms WHERE project_id = ? AND term_text = ? AND locale = ?`),
		projectID, termText, locale).Scan(&count)
	return count > 0, err
}

// AddDNTEntry adds an entry to the do-not-translate list.
func (s *ReviewQueueStore) AddDNTEntry(ctx context.Context, projectID, text, entityType, locale, source string) error {
	q := `INSERT INTO dnt_entries (project_id, text, entity_type, locale, source) VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`
	_, err := s.db.ExecContext(ctx, s.q(q), projectID, text, entityType, locale, source)
	return err
}

// ListDNTEntries returns all DNT entries for a project.
func (s *ReviewQueueStore) ListDNTEntries(ctx context.Context, projectID string) ([]DNTEntry, error) {
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT project_id, text, entity_type, locale, source, created_at FROM dnt_entries WHERE project_id = ? ORDER BY text`),
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []DNTEntry
	for rows.Next() {
		var e DNTEntry
		var createdAt string
		if err := rows.Scan(&e.ProjectID, &e.Text, &e.EntityType, &e.Locale, &e.Source, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = parseTime(createdAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// DNTEntry represents a do-not-translate entry.
type DNTEntry struct {
	ProjectID  string    `json:"project_id"`
	Text       string    `json:"text"`
	EntityType string    `json:"entity_type,omitempty"`
	Locale     string    `json:"locale"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

func (s *ReviewQueueStore) scanReviewItem(row scanner) (*ReviewItem, error) {
	var item ReviewItem
	var typ, status, pushID, occJSON, assignedTo, decidedBy, decidedAtStr, comment, editsStr, locale, createdAtStr string
	var data string

	err := row.Scan(
		&item.ID, &item.ProjectID, &typ, &status, &pushID,
		&data, &occJSON, &assignedTo, &decidedBy, &decidedAtStr,
		&comment, &editsStr, &item.Confidence, &locale, &createdAtStr,
	)
	if err != nil {
		return nil, err
	}

	item.Type = ReviewItemType(typ)
	item.Status = ReviewItemStatus(status)
	item.PushID = pushID
	item.Data = json.RawMessage(data)
	item.AssignedTo = assignedTo
	item.DecidedBy = decidedBy
	item.Comment = comment
	item.Locale = locale

	if decidedAtStr != "" {
		if t, err := parseTime(decidedAtStr); err == nil {
			item.DecidedAt = &t
		}
	}
	if editsStr != "" && editsStr != "{}" {
		item.Edits = json.RawMessage(editsStr)
	}

	_ = json.Unmarshal([]byte(occJSON), &item.Occurrences)
	item.CreatedAt, _ = parseTime(createdAtStr)

	return &item, nil
}

// parseTime parses a timestamp string in various formats (RFC3339, SQLite datetime).
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z07:00", s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", s)
}
