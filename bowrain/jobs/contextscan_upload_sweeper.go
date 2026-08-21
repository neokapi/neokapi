package jobs

import (
	"context"
	"log/slog"
	"time"

	corestorage "github.com/neokapi/neokapi/core/storage"
)

// Brand-scan upload envelopes carry customer brand material (up to 40 MiB per
// scan request), so they must not accumulate in blob storage indefinitely.
// They are retained for a grace window past the job's terminal state — long
// enough that Regenerate (which reuses the original upload keys) keeps
// working across a review session — and then deleted by the sweeper below.
const (
	defaultContextScanUploadRetention     = 7 * 24 * time.Hour
	defaultContextScanUploadSweepInterval = time.Hour
	contextScanUploadSweepBatch           = 100
)

// ContextScanUploadSweepStore is the store surface the upload sweeper drives.
// The PostgreSQL ContextScanJobStore satisfies it.
type ContextScanUploadSweepStore interface {
	SweepExpiredContextScanUploads(ctx context.Context, olderThan time.Duration, limit int) ([]ContextScanUploadCleanup, error)
	ClearContextScanUploadKeys(ctx context.Context, id string) error
}

// ContextScanUploadSweeper periodically deletes the uploaded source envelopes
// of brand-scan jobs that have been terminal (completed or failed) for longer
// than the retention window. Nothing else ever deletes these blobs: the
// worker only downloads them, and completion does not clean up (Regenerate
// must be able to reuse the keys), so without this sweep every upload batch —
// including abandoned drafts and failed scans — would persist forever.
//
// Envelopes uploaded for a scan that is never started have no job row and are
// therefore outside this sweep; that residue is bounded by the endpoint's
// per-request caps and the per-IP throttle.
type ContextScanUploadSweeper struct {
	store     ContextScanUploadSweepStore
	blobs     corestorage.BlobStore
	retention time.Duration
	interval  time.Duration
}

// NewContextScanUploadSweeper builds an upload sweeper. Zero retention/interval
// use the package defaults.
func NewContextScanUploadSweeper(store ContextScanUploadSweepStore, blobs corestorage.BlobStore, retention, interval time.Duration) *ContextScanUploadSweeper {
	if retention <= 0 {
		retention = defaultContextScanUploadRetention
	}
	if interval <= 0 {
		interval = defaultContextScanUploadSweepInterval
	}
	return &ContextScanUploadSweeper{store: store, blobs: blobs, retention: retention, interval: interval}
}

// Run drives the sweep loop until ctx is cancelled, fitting the worker's
// errgroup. It returns ctx.Err() on cancellation, which the worker treats as
// a clean shutdown.
func (s *ContextScanUploadSweeper) Run(ctx context.Context) error {
	slog.InfoContext(ctx, "brand-scan upload sweeper started",
		"retention", s.retention, "interval", s.interval)
	defer slog.InfoContext(ctx, "brand-scan upload sweeper stopped")

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			jobsCleaned, blobsDeleted, err := s.sweepOnce(ctx)
			if err != nil {
				slog.WarnContext(ctx, "brand-scan upload sweep error", "error", err)
			} else if jobsCleaned > 0 || blobsDeleted > 0 {
				slog.InfoContext(ctx, "brand-scan upload sweep deleted expired uploads",
					"jobs", jobsCleaned, "blobs", blobsDeleted)
			}
		}
	}
}

// sweepOnce deletes the upload envelopes of one batch of expired jobs. A
// job's keys are cleared only once every envelope is gone (deleted now or
// already absent), so a transient blob-store failure leaves the job to the
// next sweep instead of orphaning its remaining envelopes.
func (s *ContextScanUploadSweeper) sweepOnce(ctx context.Context) (jobsCleaned, blobsDeleted int, err error) {
	candidates, err := s.store.SweepExpiredContextScanUploads(ctx, s.retention, contextScanUploadSweepBatch)
	if err != nil {
		return 0, 0, err
	}
	for _, c := range candidates {
		allGone := true
		for _, key := range c.UploadKeys {
			if key == "" {
				continue
			}
			if delErr := s.blobs.Delete(ctx, key); delErr != nil {
				// Backends differ on deleting a missing blob (localblob
				// tolerates it, azureblob errors), and keys are
				// content-addressed, so an earlier sweep of another job may
				// already have removed a shared envelope. An absent blob is a
				// success for retention purposes.
				if exists, exErr := s.blobs.Exists(ctx, key); exErr == nil && !exists {
					continue
				}
				slog.WarnContext(ctx, "brand-scan upload delete failed; leaving the job for the next sweep",
					"job_id", c.JobID, "key", key, "error", delErr)
				allGone = false
				continue
			}
			blobsDeleted++
		}
		if !allGone {
			continue
		}
		if clrErr := s.store.ClearContextScanUploadKeys(ctx, c.JobID); clrErr != nil {
			slog.WarnContext(ctx, "brand-scan upload cleanup bookkeeping failed",
				"job_id", c.JobID, "error", clrErr)
			continue
		}
		jobsCleaned++
	}
	return jobsCleaned, blobsDeleted, nil
}
