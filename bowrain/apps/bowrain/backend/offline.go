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

// MarkFailed records a replay failure, incrementing the attempt count.
func (q *OfflineQueue) MarkFailed(id int64, errMsg string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.db.Exec(
		`UPDATE pending_changes SET attempts = attempts + 1, last_error = ? WHERE id = ?`,
		errMsg, id)
	return err
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
