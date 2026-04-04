package event

import (
	"context"
	"log/slog"
	"sync"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// DeadlineChecker periodically scans for tasks approaching their deadline
// and dispatches notifications via the NotificationDispatcher.
type DeadlineChecker struct {
	taskStore  *bstore.TaskStore
	dispatcher *NotificationDispatcher
	interval   time.Duration
	stop       chan struct{}
	done       chan struct{}

	// notified tracks task IDs that have already been notified this cycle.
	// Reset when a task is no longer in the due-soon window.
	mu       sync.Mutex
	notified map[string]bool
}

// NewDeadlineChecker creates a checker that runs at the given interval.
func NewDeadlineChecker(
	taskStore *bstore.TaskStore,
	dispatcher *NotificationDispatcher,
	interval time.Duration,
) *DeadlineChecker {
	return &DeadlineChecker{
		taskStore:  taskStore,
		dispatcher: dispatcher,
		interval:   interval,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		notified:   make(map[string]bool),
	}
}

// Start begins the periodic deadline check loop.
func (dc *DeadlineChecker) Start() {
	go dc.loop()
}

// Close stops the deadline checker and waits for it to finish.
func (dc *DeadlineChecker) Close() {
	close(dc.stop)
	<-dc.done
}

func (dc *DeadlineChecker) loop() {
	defer close(dc.done)

	ticker := time.NewTicker(dc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-dc.stop:
			return
		case <-ticker.C:
			dc.check()
		}
	}
}

func (dc *DeadlineChecker) check() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Find open/in-progress tasks due within the next 24 hours.
	horizon := time.Now().UTC().Add(24 * time.Hour)
	result, err := dc.taskStore.List(ctx, bstore.TaskQuery{
		Statuses:  []string{string(bstore.TaskStatusOpen), string(bstore.TaskStatusInProgress)},
		DueBefore: &horizon,
		Limit:     200,
	})
	if err != nil {
		slog.Warn("deadline checker failed to list tasks", "error", err)
		return
	}

	// Build set of current due task IDs for pruning stale entries.
	currentDue := make(map[string]bool, len(result.Tasks))
	for i := range result.Tasks {
		task := &result.Tasks[i]
		if task.AssigneeID == "" || task.DueAt == nil {
			continue
		}
		currentDue[task.ID] = true

		dc.mu.Lock()
		alreadyNotified := dc.notified[task.ID]
		dc.mu.Unlock()

		if alreadyNotified {
			continue
		}

		dc.dispatcher.DispatchDeadlineApproaching(ctx, task)

		dc.mu.Lock()
		dc.notified[task.ID] = true
		dc.mu.Unlock()
	}

	// Prune tasks no longer in the due-soon window (completed, deleted, or past due).
	dc.mu.Lock()
	for id := range dc.notified {
		if !currentDue[id] {
			delete(dc.notified, id)
		}
	}
	dc.mu.Unlock()
}
