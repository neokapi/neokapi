package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neokapi/neokapi/core/id"
)

// LeaderElector provides database-backed leader election using a lease table.
// Only the lease holder should run singleton components (trackers, automation engine, cleanup).
type LeaderElector struct {
	db       *sql.DB
	dialect  Dialect
	name     string        // lease name (e.g., "bowrain-server")
	holderID string        // unique ID for this instance
	ttl      time.Duration // lease duration
	interval time.Duration // renewal interval (should be < ttl/2)

	leader atomic.Bool
	done   chan struct{}
	mu     sync.Mutex
}

// NewLeaderElector creates an elector for the given lease name.
// ttl is how long the lease is valid; interval is how often to renew.
func NewLeaderElector(db *sql.DB, dialect Dialect, name string, ttl, interval time.Duration) *LeaderElector {
	return &LeaderElector{
		db:       db,
		dialect:  dialect,
		name:     name,
		holderID: id.New(),
		ttl:      ttl,
		interval: interval,
		done:     make(chan struct{}),
	}
}

// Start begins the leader election loop. Call Close() to stop.
func (e *LeaderElector) Start() {
	go e.loop()
}

// Close stops the elector and releases the lease if held.
func (e *LeaderElector) Close() {
	close(e.done)
	e.release()
}

// IsLeader returns true if this instance currently holds the lease.
func (e *LeaderElector) IsLeader() bool {
	return e.leader.Load()
}

// HolderID returns this instance's unique identifier.
func (e *LeaderElector) HolderID() string {
	return e.holderID
}

func (e *LeaderElector) loop() {
	// Try to acquire immediately.
	e.tryAcquireOrRenew()

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.done:
			return
		case <-ticker.C:
			e.tryAcquireOrRenew()
		}
	}
}

func (e *LeaderElector) tryAcquireOrRenew() {
	e.mu.Lock()
	defer e.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	expiresAt := time.Now().UTC().Add(e.ttl).Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)

	// Step 1: Try to take over if the lease is expired or we already hold it.
	updateQ := Rebind(e.dialect, `UPDATE leader_leases SET holder_id = ?, expires_at = ?
		WHERE name = ? AND (expires_at < ? OR holder_id = ?)`)
	res, err := e.db.ExecContext(ctx, updateQ, e.holderID, expiresAt, e.name, now, e.holderID)
	if err != nil {
		slog.Info("leader: renew failed", "error", err)
		e.leader.Store(false)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		e.setLeader(true)
		return
	}

	// Step 2: No existing lease to update — try to insert.
	insertQ := Rebind(e.dialect, `INSERT OR IGNORE INTO leader_leases (name, holder_id, expires_at) VALUES (?, ?, ?)`)
	if e.dialect == DialectPostgres {
		insertQ = `INSERT INTO leader_leases (name, holder_id, expires_at) VALUES ($1, $2, $3) ON CONFLICT (name) DO NOTHING`
	}
	res, err = e.db.ExecContext(ctx, insertQ, e.name, e.holderID, expiresAt)
	if err != nil {
		slog.Info("leader: acquire failed", "error", err)
		e.leader.Store(false)
		return
	}
	n, _ = res.RowsAffected()
	if n > 0 {
		e.setLeader(true)
		return
	}

	// Neither update nor insert succeeded — another instance holds a valid lease.
	e.setLeader(false)
}

func (e *LeaderElector) setLeader(isLeader bool) {
	wasLeader := e.leader.Load()
	e.leader.Store(isLeader)
	if isLeader && !wasLeader {
		slog.Info("leader: acquired lease", "lease", e.name, "holder", e.holderID)
	} else if !isLeader && wasLeader {
		slog.Info("leader: lost lease", "lease", e.name)
	}
}

func (e *LeaderElector) release() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.leader.Load() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q := Rebind(e.dialect, `DELETE FROM leader_leases WHERE name = ? AND holder_id = ?`)
	_, err := e.db.ExecContext(ctx, q, e.name, e.holderID)
	if err != nil {
		slog.Info("leader: release failed", "error", err)
	} else {
		slog.Info("leader: released lease", "lease", e.name)
	}
	e.leader.Store(false)
}

// RunIfLeader executes fn only if this instance is the leader.
// Useful for gating periodic tasks.
func (e *LeaderElector) RunIfLeader(fn func()) {
	if e.IsLeader() {
		fn()
	}
}

// FormatStatus returns a human-readable status string.
func (e *LeaderElector) FormatStatus() string {
	if e.IsLeader() {
		return fmt.Sprintf("leader (holder=%s)", e.holderID)
	}
	return fmt.Sprintf("follower (holder=%s)", e.holderID)
}
