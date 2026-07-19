package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/neokapi/neokapi/core/id"
)

// This is the durable store behind back-to-source review (RV-F). Today no review
// action can change the source — every review effect is target/locale-scoped
// ("reviewing French never touches German"). A reviewer who catches a source-TEXT
// problem (a typo, wrong wording, missing DNT) while reviewing ONE locale records
// a ProposedSourceChange here; a source owner (PermEditSource) later approves it —
// applying the fix to the block's source, which re-drafts EVERY locale — or
// rejects it. The proposal is the reviewed hand-off between "any reviewer may
// propose" and "only a source owner may change the source".

// SourceProposalKind classifies what a proposal asks for.
type SourceProposalKind string

const (
	// SourceProposalTextFix rewrites the block's source text (typo/wording).
	SourceProposalTextFix SourceProposalKind = "text-fix"
	// SourceProposalMarkDNT marks the source string do-not-translate in addition
	// to writing the proposed source.
	SourceProposalMarkDNT SourceProposalKind = "mark-dnt"
)

// SourceProposalStatus tracks the proposal lifecycle.
type SourceProposalStatus string

const (
	SourceProposalOpen     SourceProposalStatus = "open"
	SourceProposalApproved SourceProposalStatus = "approved"
	SourceProposalRejected SourceProposalStatus = "rejected"
)

// ProposedSourceChange is a reviewer's proposal to change a block's source. It is
// created open, then resolved to approved (applied + fanned out) or rejected.
type ProposedSourceChange struct {
	ID             string               `json:"id"`
	WorkspaceID    string               `json:"workspace_id"`
	ProjectID      string               `json:"project_id"`
	Stream         string               `json:"stream,omitempty"`
	ItemName       string               `json:"item_name,omitempty"`
	BlockID        string               `json:"block_id"`
	Kind           SourceProposalKind   `json:"kind"`
	OriginalSource string               `json:"original_source"`
	ProposedSource string               `json:"proposed_source"`
	Rationale      string               `json:"rationale,omitempty"`
	// FoundInLocale is the target locale the finder was reviewing when they caught
	// the source problem — the attribution "found while reviewing fr".
	FoundInLocale  string               `json:"found_in_locale,omitempty"`
	FinderUser     string               `json:"finder_user,omitempty"`
	Status         SourceProposalStatus `json:"status"`
	DecidedBy      string               `json:"decided_by,omitempty"`
	DecisionReason string               `json:"decision_reason,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
	DecidedAt      *time.Time           `json:"decided_at,omitempty"`
}

// SourceProposalStore persists proposed source changes. Concrete *sql.DB store
// (mirrors TaskStore/ReviewQueueStore); the table exists on both SQLite and
// Postgres, so one store scans identically on either backend.
type SourceProposalStore struct {
	db *sql.DB
}

// NewSourceProposalStore creates a proposed-source-change store over db.
func NewSourceProposalStore(db *sql.DB) *SourceProposalStore {
	return &SourceProposalStore{db: db}
}

// Create inserts a new open proposal.
func (s *SourceProposalStore) Create(ctx context.Context, p *ProposedSourceChange) error {
	if p.ID == "" {
		p.ID = id.New()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = SourceProposalOpen
	}
	if p.Kind == "" {
		p.Kind = SourceProposalTextFix
	}
	if p.Stream == "" {
		p.Stream = "main"
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO proposed_source_changes (id, workspace_id, project_id, stream,
		 item_name, block_id, kind, original_source, proposed_source, rationale,
		 found_in_locale, finder_user, status, decided_by, decision_reason,
		 created_at, updated_at, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		p.ID, p.WorkspaceID, p.ProjectID, p.Stream,
		p.ItemName, p.BlockID, string(p.Kind), p.OriginalSource, p.ProposedSource, p.Rationale,
		p.FoundInLocale, p.FinderUser, string(p.Status), p.DecidedBy, p.DecisionReason,
		p.CreatedAt.UTC().Format(time.RFC3339Nano),
		p.UpdatedAt.UTC().Format(time.RFC3339Nano),
		nil)
	return err
}

// Get retrieves a proposal by ID.
func (s *SourceProposalStore) Get(ctx context.Context, proposalID string) (*ProposedSourceChange, error) {
	row := s.db.QueryRowContext(ctx, sourceProposalSelect+` WHERE id = $1`, proposalID)
	return scanSourceProposal(row)
}

// ListOpen returns the open proposals for a project, newest first.
func (s *SourceProposalStore) ListOpen(ctx context.Context, projectID string) ([]ProposedSourceChange, error) {
	rows, err := s.db.QueryContext(ctx,
		sourceProposalSelect+` WHERE project_id = $1 AND status = 'open' ORDER BY created_at DESC`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProposedSourceChange
	for rows.Next() {
		p, err := scanSourceProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// Resolve moves an open proposal to approved or rejected, stamping who decided it
// and (optionally) why. It is a no-op if the proposal is already resolved, so a
// duplicate decide races to exactly one terminal outcome.
func (s *SourceProposalStore) Resolve(ctx context.Context, proposalID string, status SourceProposalStatus, decidedBy, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE proposed_source_changes
		 SET status = $1, decided_by = $2, decision_reason = $3, decided_at = $4, updated_at = $4
		 WHERE id = $5 AND status = 'open'`,
		string(status), decidedBy, reason, now, proposalID)
	return err
}

const sourceProposalSelect = `SELECT id, workspace_id, project_id, stream, item_name,
	 block_id, kind, original_source, proposed_source, rationale, found_in_locale,
	 finder_user, status, decided_by, decision_reason, created_at, updated_at, decided_at
	 FROM proposed_source_changes`

func scanSourceProposal(row scanner) (*ProposedSourceChange, error) {
	var p ProposedSourceChange
	var kind, status string
	var decidedAt sql.NullTime

	err := row.Scan(
		&p.ID, &p.WorkspaceID, &p.ProjectID, &p.Stream, &p.ItemName,
		&p.BlockID, &kind, &p.OriginalSource, &p.ProposedSource, &p.Rationale, &p.FoundInLocale,
		&p.FinderUser, &status, &p.DecidedBy, &p.DecisionReason, &p.CreatedAt, &p.UpdatedAt, &decidedAt,
	)
	if err != nil {
		return nil, err
	}
	p.Kind = SourceProposalKind(kind)
	p.Status = SourceProposalStatus(status)
	if decidedAt.Valid {
		d := decidedAt.Time.UTC()
		p.DecidedAt = &d
	}
	return &p, nil
}
