package knowledge

import (
	"context"
	"time"
)

// Store persists the governance and collaboration layer of the brand knowledge
// graph (AD-021): markets, observations, comments, concept revisions,
// change-sets with their ops, reviews, and pilots. It is workspace-scoped —
// every method takes a workspace ID and operates only within it — and is backed
// by PostgreSQL on the platform (NewPostgresKnowledgeStore). The pure
// change-set state machine, op validation, governed/ordinary classification,
// and conflict detection live in changeset.go and are exercised independently
// of any Store implementation.
//
// Methods are context-first and return an error. Get-style lookups for a single
// row return a nil pointer (or the zero value) with a non-nil error when the
// row is absent, following the surrounding store conventions.
type Store interface {
	// --- Markets ---------------------------------------------------------

	// CreateMarket inserts a new market. The implementation assigns ID and
	// timestamps when they are empty.
	CreateMarket(ctx context.Context, m *Market) error
	// GetMarket returns a market by ID within the workspace.
	GetMarket(ctx context.Context, workspaceID, marketID string) (*Market, error)
	// UpdateMarket updates a market's mutable fields (name, description,
	// locales) and bumps its updated timestamp.
	UpdateMarket(ctx context.Context, m *Market) error
	// DeleteMarket removes a market by ID.
	DeleteMarket(ctx context.Context, workspaceID, marketID string) error
	// ListMarkets returns all markets in the workspace, ordered by name.
	ListMarkets(ctx context.Context, workspaceID string) ([]*Market, error)

	// --- Observations ----------------------------------------------------

	// AddObservation inserts an observation. The implementation assigns ID and
	// CreatedAt when empty.
	AddObservation(ctx context.Context, o *Observation) error
	// DeleteObservation removes an observation by ID.
	DeleteObservation(ctx context.Context, workspaceID, observationID string) error
	// ListObservationsByConcept returns a concept's observations, newest first.
	ListObservationsByConcept(ctx context.Context, workspaceID, conceptID string) ([]*Observation, error)

	// --- Comments --------------------------------------------------------

	// AddComment inserts a comment on a concept or change-set thread. The
	// implementation assigns ID and CreatedAt when empty.
	AddComment(ctx context.Context, c *Comment) error
	// DeleteComment removes a comment by ID.
	DeleteComment(ctx context.Context, workspaceID, commentID string) error
	// ResolveComment sets a comment's resolved flag.
	ResolveComment(ctx context.Context, workspaceID, commentID string, resolved bool) error
	// ListCommentsByConcept returns a concept's comments in thread order.
	ListCommentsByConcept(ctx context.Context, workspaceID, conceptID string) ([]*Comment, error)
	// ListCommentsByChangeset returns the comments attached to a change-set.
	ListCommentsByChangeset(ctx context.Context, workspaceID, changesetID string) ([]*Comment, error)

	// --- Concept revisions ----------------------------------------------

	// AddRevision appends an immutable revision snapshot for a concept. Callers
	// supply Rev (typically LatestRev+1); the implementation enforces the
	// (workspace_id, concept_id, rev) primary key.
	AddRevision(ctx context.Context, r *ConceptRevision) error
	// ListRevisions returns a concept's revisions in ascending Rev order.
	ListRevisions(ctx context.Context, workspaceID, conceptID string) ([]*ConceptRevision, error)
	// LatestRev returns the highest revision number recorded for a concept, or
	// 0 when the concept has no revisions yet.
	LatestRev(ctx context.Context, workspaceID, conceptID string) (int64, error)

	// --- Change-sets -----------------------------------------------------

	// CreateChangeSet inserts a new change-set in the draft status. The
	// implementation assigns ID and timestamps when empty and defaults Status
	// to draft.
	CreateChangeSet(ctx context.Context, cs *ChangeSet) error
	// GetChangeSet returns a change-set by ID.
	GetChangeSet(ctx context.Context, workspaceID, changesetID string) (*ChangeSet, error)
	// ListChangeSets returns the workspace's change-sets. A non-empty status
	// filters to that status; an empty status returns all, newest first.
	ListChangeSets(ctx context.Context, workspaceID string, status ChangeSetStatus) ([]*ChangeSet, error)
	// SetChangeSetStatus moves a change-set to a new non-merge status. The
	// implementation loads the current status, validates the edge with
	// ValidateStatusTransition (returning its error on a disallowed edge),
	// records SubmittedAt on the draft → in_review transition, and bumps
	// UpdatedAt. Finalizing a merge is done through SetMergeResult, not here.
	SetChangeSetStatus(ctx context.Context, workspaceID, changesetID string, to ChangeSetStatus) error
	// SetMergeResult finalizes a merge: it validates the approved → merged
	// transition via ValidateStatusTransition, sets Status to merged, and
	// records MergedBy and MergedAt atomically.
	SetMergeResult(ctx context.Context, workspaceID, changesetID, mergedBy string, mergedAt time.Time) error

	// --- Change-set ops --------------------------------------------------

	// AppendOp appends an op to a change-set. The implementation assigns the
	// next Seq within the change-set and CreatedAt when empty.
	AppendOp(ctx context.Context, op *ChangeSetOp) error
	// RemoveOp removes an op by Seq (draft change-sets only at the call site).
	RemoveOp(ctx context.Context, workspaceID, changesetID string, seq int64) error
	// ListOps returns a change-set's ops in ascending Seq order.
	ListOps(ctx context.Context, workspaceID, changesetID string) ([]*ChangeSetOp, error)

	// --- Reviews ---------------------------------------------------------

	// AddReview records (or replaces, by reviewer) a reviewer's verdict on a
	// change-set. CreatedAt is assigned when empty.
	AddReview(ctx context.Context, r *ChangeSetReview) error
	// ListReviews returns a change-set's reviews.
	ListReviews(ctx context.Context, workspaceID, changesetID string) ([]*ChangeSetReview, error)

	// --- Pilots ----------------------------------------------------------

	// AddPilot binds a change-set to a project stream. CreatedAt is assigned
	// when empty.
	AddPilot(ctx context.Context, p *Pilot) error
	// RemovePilot removes a pilot binding.
	RemovePilot(ctx context.Context, workspaceID, changesetID, projectID, stream string) error
	// ListPilots returns the pilots of a change-set.
	ListPilots(ctx context.Context, workspaceID, changesetID string) ([]*Pilot, error)
	// ListPilotsForStream returns the pilots bound to a given project stream
	// (used to resolve a stream's active shadow over the workspace graph).
	ListPilotsForStream(ctx context.Context, workspaceID, projectID, stream string) ([]*Pilot, error)

	// Close releases resources held by the store. Implementations that do not
	// own the connection return nil.
	Close() error
}
