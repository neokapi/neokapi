package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunAction_ContainsAPanic: automation actions run in goroutines started
// from an event-bus callback, so nothing above them can recover. A panicking
// action used to take the whole server down with it.
func TestRunAction_ContainsAPanic(t *testing.T) {
	s := &Server{}
	_, cancel := context.WithCancel(t.Context())

	s.runAction("write_overlay", cancel, func() { panic("nil map write") })

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.actions.Wait()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the panicking action never released the wait group")
	}
}

// TestAwaitActions_WaitsForRunningActions: an action that outlives Shutdown
// writes into stores that are closing under it.
func TestAwaitActions_WaitsForRunningActions(t *testing.T) {
	s := &Server{}
	_, cancel := context.WithCancel(t.Context())

	var finished atomic.Bool
	release := make(chan struct{})
	s.runAction("auto_extract", cancel, func() {
		<-release
		finished.Store(true)
	})

	waited := make(chan struct{})
	go func() {
		defer close(waited)
		s.awaitActions(context.Background())
	}()

	select {
	case <-waited:
		t.Fatal("shutdown returned while an action was still running")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown never observed the action finishing")
	}
	assert.True(t, finished.Load())
}

// TestAwaitActions_GivesUpWhenTheBudgetRunsOut: waiting is bounded by the
// shutdown context, so one stuck action cannot hold the process open.
func TestAwaitActions_GivesUpWhenTheBudgetRunsOut(t *testing.T) {
	s := &Server{}
	_, cancel := context.WithCancel(t.Context())

	release := make(chan struct{})
	defer close(release)
	s.runAction("auto_translate", cancel, func() { <-release })

	ctx, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()

	start := time.Now()
	s.awaitActions(ctx)
	require.Less(t, time.Since(start), 5*time.Second, "the wait is bounded by the budget")
}
