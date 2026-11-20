package store

import (
	"context"
	"time"
)

// ReviewPlanSnapshot is the stored part of a project's review plan on one
// stream: the counts that read the project's translations, as JSON the
// server writes and reads. The server recomputes it in the background after
// changes, so the plan and the review inbox read one row instead of counting.
type ReviewPlanSnapshot struct {
	ProjectID string
	Stream    string
	// Data is the counts as the server encoded them; empty before the first
	// compute.
	Data []byte
	// ComputedAt is when the counts were taken; zero before the first
	// compute.
	ComputedAt time.Time
	// Changed is set while a change waits for the next compute.
	Changed bool
}

// ReviewPlanSnapshotClaim is the right to recompute one snapshot. The
// instance holding it is the only one computing that snapshot until the claim
// completes, is released or its lease runs out.
type ReviewPlanSnapshotClaim struct {
	ProjectID   string
	Stream      string
	WorkspaceID string
	// Seq is the change sequence the claim was taken at. A change after it
	// leaves the snapshot changed when the claim completes.
	Seq int64
	// Token identifies the claim; a claim whose lease another instance took
	// over completes nothing.
	Token string
}

// ReviewPlanSnapshotStore keeps the review plan's snapshots and the changes
// that make them due. The PostgreSQL store implements it. Each snapshot has a
// change sequence: a change bumps it, and a compute records the sequence it
// read at, so a change during a compute leaves the snapshot due again.
type ReviewPlanSnapshotStore interface {
	// GetReviewPlanSnapshot returns the project's snapshot on stream, or nil
	// when it has none.
	GetReviewPlanSnapshot(ctx context.Context, projectID, stream string) (*ReviewPlanSnapshot, error)
	// ClaimReviewPlanSnapshot claims the project's snapshot on stream for a
	// compute now, creating it when missing, whatever its changes. ok is
	// false while another claim holds it.
	ClaimReviewPlanSnapshot(ctx context.Context, projectID, stream, workspaceID, workspaceSlug string, lease time.Duration) (claim ReviewPlanSnapshotClaim, ok bool, err error)
	// ClaimDueReviewPlanSnapshots claims up to limit snapshots that are due:
	// changed, held by no claim, and either quiet (no change for quiet) or
	// changed first at least maxWait ago. The oldest change comes first.
	ClaimDueReviewPlanSnapshots(ctx context.Context, quiet, maxWait, lease time.Duration, limit int) ([]ReviewPlanSnapshotClaim, error)
	// CompleteReviewPlanSnapshot stores the counts a claim computed and
	// releases it. A change after the claim was taken leaves the snapshot
	// changed, due again as if first changed when the claim was taken.
	CompleteReviewPlanSnapshot(ctx context.Context, claim ReviewPlanSnapshotClaim, data []byte) error
	// ReleaseReviewPlanSnapshot gives up a claim that computed nothing. The
	// snapshot stays changed, and no claim takes it again before retry.
	ReleaseReviewPlanSnapshot(ctx context.Context, claim ReviewPlanSnapshotClaim, retry time.Duration) error
	// MarkReviewPlanSnapshotsChanged records a change to every snapshot of
	// the projects named, and of the projects in the workspaces named (by id
	// or slug).
	MarkReviewPlanSnapshotsChanged(ctx context.Context, projectIDs, workspaces []string) error
}

// ReviewPlanSnapshots finds the review plan snapshot store behind a content
// store, looking through any decorator that names what it wraps.
func ReviewPlanSnapshots(cs ContentStore) (ReviewPlanSnapshotStore, bool) {
	for cs != nil {
		if s, ok := cs.(ReviewPlanSnapshotStore); ok {
			return s, true
		}
		inner, ok := cs.(interface{ Unwrap() ContentStore })
		if !ok {
			return nil, false
		}
		cs = inner.Unwrap()
	}
	return nil, false
}
