//go:build js && wasm

package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The entry points that act on the App run in their turn: while a change
// call holds the engine, a command, a completion, a reset and another call
// wait for it, and each runs once the call is done. `make test-wasm-stores`
// runs it in Node.
func TestEngine_RunsCommandsCallsAndResetsInTurn(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{name: "a command", run: func(*testing.T) { runOnce([]string{"version"}) }},
		{name: "a completion", run: func(*testing.T) { runOnce([]string{"__complete", ""}) }},
		{name: "a reset", run: func(t *testing.T) {
			assert.NoError(t, resetInTurn(context.Background(), t.TempDir()))
		}},
		{name: "another call", run: func(*testing.T) {
			_, _ = serveChange(func(context.Context, string, []byte, []byte) ([]byte, error) { return nil, nil }, nil, nil)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Run it once first, so the workspace a first reset opens is open
			// and the wait below measures the turn rather than the opening.
			tc.run(t)
			release := holdEngine(t)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				tc.run(t)
			}()
			select {
			case <-finished:
				release()
				t.Fatalf("%s ran while a change call held the engine", tc.name)
			case <-time.After(200 * time.Millisecond):
			}
			release()
			select {
			case <-finished:
			case <-time.After(time.Minute):
				t.Fatalf("%s did not run once the call was done", tc.name)
			}
		})
	}
}

// holdEngine starts a change call that holds the engine until release is
// called, and returns once the call is inside.
func holdEngine(t *testing.T) (release func()) {
	t.Helper()
	inside, done := make(chan struct{}), make(chan struct{})
	go func() {
		_, err := serveChange(func(context.Context, string, []byte, []byte) ([]byte, error) {
			close(inside)
			<-done
			return nil, nil
		}, nil, nil)
		if !assert.NoError(t, err, "hold the engine") {
			close(inside)
		}
	}()
	<-inside
	return func() { close(done) }
}
