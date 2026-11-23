package main

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// A failed start is captured before the buffered events are flushed, so the
// refusal reaches Sentry; a clean exit only flushes.
func TestExitCode_CapturesAFailedStartBeforeFlushing(t *testing.T) {
	var order []string
	capture := func(err error) { order = append(order, "capture: "+err.Error()) }
	flush := func(time.Duration) { order = append(order, "flush") }

	assert.Equal(t, 1, exitCode(errors.New("refusing to migrate"), capture, flush))
	assert.Equal(t, []string{"capture: refusing to migrate", "flush"}, order)

	order = nil
	assert.Equal(t, 0, exitCode(nil, capture, flush))
	assert.Equal(t, []string{"flush"}, order)
}
