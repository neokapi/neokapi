package billing

import (
	"context"
	"log/slog"
	"time"
)

const (
	defaultTrialSweepInterval = time.Hour
	trialSweepBatch           = 100
)

// TrialSweepStore is the store surface the trial sweeper drives. PgBillingStore
// satisfies it. It is deliberately narrower than BillingStore: the sweeper needs
// exactly two operations, and keeping it separate means an in-memory BillingStore
// (self-hosted, tests) does not have to grow a trial-expiry implementation.
type TrialSweepStore interface {
	ExpireTrials(ctx context.Context, now time.Time, limit int) ([]ExpiredTrial, error)
	RecordBillingEvent(ctx context.Context, event *BillingEvent) error
}

// TrialSweeper ends local Pro trials that have run out.
//
// Nothing else can end them: the trial is card-free, so there is no Stripe
// subscription whose lifecycle events would downgrade the workspace. Without
// this loop every signup keeps Pro limits forever, which is precisely the
// state the platform was in before epic 005.
//
// A trial that converts needs no sweeping — the checkout webhook overwrites plan
// and status, and clears trial_ends_at, so the row no longer matches.
//
// The sweeper does not carry a plan-cache syncer: ExpireTrials updates the
// workspace plan cache atomically with the downgrade (a separate sync step could
// drift, see its doc), so the sweeper only has to record the audit event.
type TrialSweeper struct {
	store    TrialSweepStore
	interval time.Duration
}

// NewTrialSweeper builds a trial sweeper. A zero interval uses the default
// (hourly): the trial boundary is a day-scale deadline, so hourly precision is
// far finer than the product promises and costs one indexed query per hour.
func NewTrialSweeper(store TrialSweepStore, interval time.Duration) *TrialSweeper {
	if interval <= 0 {
		interval = defaultTrialSweepInterval
	}
	return &TrialSweeper{store: store, interval: interval}
}

// Run drives the sweep loop until ctx is cancelled, fitting the worker's
// errgroup. It returns ctx.Err() on cancellation, which the worker treats as a
// clean shutdown. A sweep that fails is logged and retried on the next tick —
// an expired trial staying up one more hour is not worth crashing the worker.
func (s *TrialSweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Sweep once at startup so a worker that was down over a trial boundary
	// catches up immediately rather than one interval late.
	s.sweep(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep downgrades one batch of expired trials.
func (s *TrialSweeper) sweep(ctx context.Context) {
	expired, err := s.store.ExpireTrials(ctx, time.Now().UTC(), trialSweepBatch)
	if err != nil {
		slog.Warn("trial sweep failed", "error", err)
		return
	}
	for _, e := range expired {
		// ExpireTrials already downgraded the subscription and the plan cache
		// atomically; the billing event is audit only. Losing it on a transient
		// error has no money impact, and the downgrade stands regardless.
		if err := s.store.RecordBillingEvent(ctx, &BillingEvent{
			WorkspaceID: e.WorkspaceID,
			EventType:   "trial_expired",
			Detail:      "14-day Pro trial ended, downgraded to free",
		}); err != nil {
			slog.Warn("trial sweep: failed to record event", "workspace", e.WorkspaceID, "error", err)
		}
		slog.Info("trial expired, workspace downgraded to free", "workspace", e.WorkspaceID)
	}
}
