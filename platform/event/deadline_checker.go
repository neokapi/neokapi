package event

import (
	"context"
	"log"
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
		Status:    string(bstore.TaskStatusOpen),
		DueBefore: &horizon,
		Limit:     200,
	})
	if err != nil {
		log.Printf("WARNING: deadline checker failed to list tasks: %v", err)
		return
	}

	// Also check in-progress tasks.
	inProgress, err := dc.taskStore.List(ctx, bstore.TaskQuery{
		Status:    string(bstore.TaskStatusInProgress),
		DueBefore: &horizon,
		Limit:     200,
	})
	if err != nil {
		log.Printf("WARNING: deadline checker failed to list in-progress tasks: %v", err)
	} else {
		result.Tasks = append(result.Tasks, inProgress.Tasks...)
	}

	for i := range result.Tasks {
		task := &result.Tasks[i]
		if task.AssigneeID == "" || task.DueAt == nil {
			continue
		}
		dc.dispatcher.DispatchDeadlineApproaching(ctx, task)
	}
}
