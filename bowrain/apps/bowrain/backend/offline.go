package backend

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
)

// PendingChange represents a queued mutation that couldn't be sent to the server.
type PendingChange struct {
	ID        int64     `json:"id"`
	Operation string    `json:"operation"`
	Payload   string    `json:"payload"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error"`
}

// OfflineQueue manages pending changes that are queued when the server is unreachable.
// Changes are persisted in a SQLite database and replayed when the connection is restored.
type OfflineQueue struct {
	db *storage.DB
	mu sync.Mutex
}

// NewOfflineQueue opens (or creates) the offline queue database.
func NewOfflineQueue(dbPath string) (*OfflineQueue, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create queue dir: %w", err)
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open queue db: %w", err)
	}

	if err := initQueueSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init queue schema: %w", err)
	}

	return &OfflineQueue{db: db}, nil
}

func initQueueSchema(db *storage.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pending_changes (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			operation  TEXT NOT NULL,
			payload    TEXT NOT NULL DEFAULT '{}',
			status     TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			attempts   INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_pending_status ON pending_changes(status, id);
	`)
	return err
}

// Enqueue adds a pending change to the queue. The payload is serialized as JSON.
func (q *OfflineQueue) Enqueue(operation string, payload any) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	_, err = q.db.Exec(
		`INSERT INTO pending_changes (operation, payload) VALUES (?, ?)`,
		operation, string(data),
	)
	return err
}

// PendingCount returns the number of pending (not yet replayed) changes.
func (q *OfflineQueue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	var count int
	_ = q.db.QueryRow(`SELECT COUNT(*) FROM pending_changes WHERE status = 'pending'`).Scan(&count)
	return count
}

// PeekPending returns up to `limit` pending changes in FIFO order.
func (q *OfflineQueue) PeekPending(limit int) ([]PendingChange, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	rows, err := q.db.Query(
		`SELECT id, operation, payload, status, created_at, attempts, last_error
		 FROM pending_changes WHERE status = 'pending' ORDER BY id ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []PendingChange
	for rows.Next() {
		var c PendingChange
		var createdAt string
		if err := rows.Scan(&c.ID, &c.Operation, &c.Payload, &c.Status, &createdAt, &c.Attempts, &c.LastError); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

// MarkCompleted marks a change as successfully replayed.
func (q *OfflineQueue) MarkCompleted(id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.db.Exec(`UPDATE pending_changes SET status = 'completed' WHERE id = ?`, id)
	return err
}

// maxReplayAttempts caps how many replay attempts a pending change gets before
// it is marked terminally failed. Without a cap, a change that keeps failing
// while the connection stays up would be re-selected by every replay pass
// forever and the queue could never drain past it.
const maxReplayAttempts = 5

// MarkFailed records a transient replay failure, incrementing the attempt
// count. A change that reaches maxReplayAttempts is marked terminally failed
// ('failed', excluded from PeekPending) so the queue drains past it.
func (q *OfflineQueue) MarkFailed(id int64, errMsg string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.db.Exec(
		`UPDATE pending_changes
		 SET attempts = attempts + 1, last_error = ?,
		     status = CASE WHEN attempts + 1 >= ? THEN 'failed' ELSE status END
		 WHERE id = ?`,
		errMsg, maxReplayAttempts, id)
	return err
}

// MarkFailedPermanent marks a change as terminally failed: the server rejected
// it with a permanent (4xx) error, so retrying the identical request can never
// succeed. The change leaves the pending set immediately (status 'failed') —
// replay must not busy-loop on it — but stays in the queue for inspection
// until Clear.
func (q *OfflineQueue) MarkFailedPermanent(id int64, errMsg string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.db.Exec(
		`UPDATE pending_changes SET attempts = attempts + 1, last_error = ?, status = 'failed' WHERE id = ?`,
		errMsg, id)
	return err
}

// MarkDropped retires an entry of a kind this version no longer sends. It
// leaves the pending set and the failed count, and stays listed with its
// notice until a person dismisses it.
func (q *OfflineQueue) MarkDropped(id int64, notice string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.db.Exec(`UPDATE pending_changes SET last_error = ?, status = 'dropped' WHERE id = ?`, notice, id)
	return err
}

// Failed returns the changes that did not reach the server, failed or
// dropped, oldest first.
func (q *OfflineQueue) Failed() ([]PendingChange, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	rows, err := q.db.Query(
		`SELECT id, operation, payload, status, created_at, attempts, last_error
		 FROM pending_changes WHERE status IN ('failed', 'dropped') ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []PendingChange
	for rows.Next() {
		var c PendingChange
		var createdAt string
		if err := rows.Scan(&c.ID, &c.Operation, &c.Payload, &c.Status, &createdAt, &c.Attempts, &c.LastError); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

// FailedIDs returns the ids of the changes that did not reach the server,
// failed or dropped, oldest first. It reads no payload, so the chrome can poll
// it and read the list itself only when the ids change.
func (q *OfflineQueue) FailedIDs() ([]int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	rows, err := q.db.Query(`SELECT id FROM pending_changes WHERE status IN ('failed', 'dropped') ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Dismiss removes one failed or dropped change. A pending change stays.
func (q *OfflineQueue) Dismiss(id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.db.Exec(`DELETE FROM pending_changes WHERE id = ? AND status IN ('failed', 'dropped')`, id)
	return err
}

// DismissAll removes every failed and dropped change.
func (q *OfflineQueue) DismissAll() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.db.Exec(`DELETE FROM pending_changes WHERE status IN ('failed', 'dropped')`)
	return err
}

// FailedCount returns the number of terminally failed changes — queued offline
// mutations the server permanently rejected on replay. Exposed so the UI can
// surface that some offline edits did not apply.
func (q *OfflineQueue) FailedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	var count int
	_ = q.db.QueryRow(`SELECT COUNT(*) FROM pending_changes WHERE status = 'failed'`).Scan(&count)
	return count
}

// PurgeCompleted removes all completed changes from the queue.
func (q *OfflineQueue) PurgeCompleted() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.db.Exec(`DELETE FROM pending_changes WHERE status = 'completed'`)
	return err
}

// Clear removes all changes from the queue.
func (q *OfflineQueue) Clear() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.db.Exec(`DELETE FROM pending_changes`)
	return err
}

// Close closes the underlying database.
func (q *OfflineQueue) Close() error {
	return q.db.Close()
}

// defaultQueuePath returns the default path for the offline queue database.
func defaultQueuePath() string {
	dir := desktopConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Info("bowrain: failed to create config dir at", "id", dir, "error", err)
	}
	return filepath.Join(dir, "offline-queue.db")
}
