package event

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// PruneOlderThan deletes audit rows older than maxAge. It is the only sanctioned
// way to remove audit rows: it opts into deletion for its transaction via the
// session flag the append-only trigger checks, so ad-hoc deletes elsewhere are
// still blocked. Pruning the oldest rows truncates the verifiable window;
// VerifyChain anchors on the oldest retained row so the remaining window still
// verifies.
func (a *AuditLogger) PruneOlderThan(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin prune tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SET LOCAL bowrain.audit_allow_delete = 'on'`); err != nil {
		return 0, fmt.Errorf("enable audit delete: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM audit_log WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune audit_log: %w", err)
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit prune: %w", err)
	}
	return n, nil
}

// AuditRetentionCleaner periodically prunes audit rows past the retention window.
type AuditRetentionCleaner struct {
	logger   *AuditLogger
	maxAge   time.Duration
	interval time.Duration
	done     chan struct{}
}

// NewAuditRetentionCleaner starts a cleaner that prunes on the given interval.
// A non-positive maxAge disables pruning (returns nil).
func NewAuditRetentionCleaner(logger *AuditLogger, maxAge, interval time.Duration) *AuditRetentionCleaner {
	if logger == nil || maxAge <= 0 {
		return nil
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	c := &AuditRetentionCleaner{logger: logger, maxAge: maxAge, interval: interval, done: make(chan struct{})}
	go c.loop()
	return c
}

// Close stops the cleaner.
func (c *AuditRetentionCleaner) Close() {
	if c != nil {
		close(c.done)
	}
}

func (c *AuditRetentionCleaner) loop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			n, err := c.logger.PruneOlderThan(context.Background(), c.maxAge)
			if err != nil {
				slog.Info("audit-retention: prune error", "error", err)
			} else if n > 0 {
				slog.Info("audit-retention: pruned old audit rows", "count", n)
			}
		}
	}
}
