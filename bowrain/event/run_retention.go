package event

import (
	"context"
	"log"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// RunRetentionCleaner periodically deletes old automation runs.
type RunRetentionCleaner struct {
	store    *bstore.AutomationRunStore
	maxAge   time.Duration
	interval time.Duration
	done     chan struct{}
}

// NewRunRetentionCleaner creates a cleaner that runs on the given interval.
func NewRunRetentionCleaner(store *bstore.AutomationRunStore, maxAge, interval time.Duration) *RunRetentionCleaner {
	c := &RunRetentionCleaner{
		store:    store,
		maxAge:   maxAge,
		interval: interval,
		done:     make(chan struct{}),
	}
	go c.loop()
	return c
}

// Close stops the cleaner.
func (c *RunRetentionCleaner) Close() {
	close(c.done)
}

func (c *RunRetentionCleaner) loop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			n, err := c.store.DeleteRunsOlderThan(context.Background(), c.maxAge)
			if err != nil {
				log.Printf("run-retention: cleanup error: %v", err)
			} else if n > 0 {
				log.Printf("run-retention: deleted %d old runs", n)
			}
		}
	}
}
