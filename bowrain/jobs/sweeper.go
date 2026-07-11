package jobs

import (
	"context"
	"log/slog"
	"time"
)

// Default cadence and threshold for the stale-job sweeper. The threshold is set
// comfortably above the NATS AckWait (natsAckWait, 5m) so the sweeper does not
// race the broker's own redelivery of a still-in-flight message; it only acts
// once a job has clearly been abandoned by a dead worker.
const (
	defaultStaleJobThreshold = 15 * time.Minute
	defaultSweepInterval     = 5 * time.Minute
)

// StaleSweepStore is the store surface the sweeper drives: sweeping stalled
// 'processing' rows back to 'queued' (or failing them), and rolling back a
// requeue whose re-enqueue failed. Both the translation JobStore and the
// BrandScanJobStore satisfy it, so one sweeper implementation recovers every
// leased job type.
type StaleSweepStore interface {
	SweepStaleProcessing(ctx context.Context, olderThan time.Duration, maxAttempts int) (requeued []string, failed int, err error)
	RevertSweepRequeue(ctx context.Context, id string, staleThreshold time.Duration) error
}

// StaleJobSweeper periodically recovers jobs stuck in 'processing' after a
// worker crashed between claim (queued→processing) and completion. Such a job
// has no NAK pending, so nothing else would ever recover it: the sweeper
// resets it to 'queued' (with attempt tracking) and re-enqueues it, or fails
// it once its retry budget is exhausted. It is the crash backstop to the
// in-flight NAK retry in the worker loop.
type StaleJobSweeper struct {
	store       StaleSweepStore
	queue       Queue
	threshold   time.Duration
	interval    time.Duration
	maxAttempts int
}

// NewStaleJobSweeper builds a sweeper. Zero threshold/interval/maxAttempts use
// the package defaults.
func NewStaleJobSweeper(store StaleSweepStore, queue Queue, threshold, interval time.Duration, maxAttempts int) *StaleJobSweeper {
	if threshold <= 0 {
		threshold = defaultStaleJobThreshold
	}
	if interval <= 0 {
		interval = defaultSweepInterval
	}
	if maxAttempts < 1 {
		maxAttempts = defaultMaxJobAttempts
	}
	return &StaleJobSweeper{
		store:       store,
		queue:       queue,
		threshold:   threshold,
		interval:    interval,
		maxAttempts: maxAttempts,
	}
}

// Run drives the sweep loop until ctx is cancelled, fitting the worker's
// errgroup (mirrors RunWorkerWithDeps). It returns ctx.Err() on cancellation,
// which the worker treats as a clean shutdown.
func (s *StaleJobSweeper) Run(ctx context.Context) error {
	slog.InfoContext(ctx, "stale-job sweeper started",
		"threshold", s.threshold, "interval", s.interval, "max_attempts", s.maxAttempts)
	defer slog.InfoContext(ctx, "stale-job sweeper stopped")

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			requeued, failed, err := s.sweepOnce(ctx)
			if err != nil {
				slog.WarnContext(ctx, "stale-job sweep error", "error", err)
			} else if requeued > 0 || failed > 0 {
				slog.InfoContext(ctx, "stale-job sweep recovered jobs",
					"requeued", requeued, "failed", failed)
			}
		}
	}
}

// sweepOnce runs a single sweep: it resets stalled jobs (re-enqueueing each
// requeued ID so a worker actually picks it up again — the crashed worker's
// broker message is gone) and fails those out of budget. It returns the number
// of jobs successfully re-enqueued and the number failed.
func (s *StaleJobSweeper) sweepOnce(ctx context.Context) (requeued, failed int, err error) {
	ids, failed, err := s.store.SweepStaleProcessing(ctx, s.threshold, s.maxAttempts)
	if err != nil {
		return 0, 0, err
	}
	for _, id := range ids {
		if eqErr := s.queue.Enqueue(ctx, id); eqErr != nil {
			// SweepStaleProcessing already flipped the row to 'queued', but the
			// enqueue failed so no live broker message exists — and nothing scans
			// 'queued' orphans (the sweep's own WHERE is status='processing'). Left
			// as-is the job is stranded forever. Roll it back to 'processing' with a
			// stale heartbeat so the NEXT sweep re-selects and re-enqueues it.
			slog.WarnContext(ctx, "re-enqueue of requeued job failed; reverting to processing for the next sweep",
				"job_id", id, "error", eqErr)
			if rerr := s.store.RevertSweepRequeue(ctx, id, s.threshold); rerr != nil {
				slog.ErrorContext(ctx, "failed to revert stranded requeued job; it may be stuck in queued",
					"job_id", id, "error", rerr)
			}
			continue
		}
		requeued++
	}
	return requeued, failed, nil
}
