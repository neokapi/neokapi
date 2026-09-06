package format_test

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collect(ch <-chan model.PartResult) []model.PartResult {
	var out []model.PartResult
	for res := range ch {
		out = append(out, res)
	}
	return out
}

// TestStreamPartsDeliversPartsAndErrors pins the ordinary contract: the parts a
// reader sends arrive in order, a returned error arrives last, and the channel
// closes either way.
func TestStreamPartsDeliversPartsAndErrors(t *testing.T) {
	t.Parallel()

	got := collect(format.StreamParts(t.Context(), func(_ context.Context, ch chan<- model.PartResult) error {
		ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock}}
		return nil
	}))
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Part)
	require.NoError(t, got[0].Error)

	sentinel := errors.New("read failed")
	got = collect(format.StreamParts(t.Context(), func(_ context.Context, ch chan<- model.PartResult) error {
		ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock}}
		return sentinel
	}))
	require.Len(t, got, 2)
	assert.ErrorIs(t, got[1].Error, sentinel)
}

// TestStreamPartsTurnsAPanicIntoAnError is the guard #2444 asked for. A reader
// does its work on a goroutine the caller does not own, so a panic there has
// nowhere to surface and kills the process: an emphasis inside an excluded
// inline HTML span took kapi down that way. The panic now arrives as a
// PartResult error carrying the stack, the parts sent before it are kept, and
// the range loop ends normally.
func TestStreamPartsTurnsAPanicIntoAnError(t *testing.T) {
	t.Parallel()

	got := collect(format.StreamParts(t.Context(), func(_ context.Context, ch chan<- model.PartResult) error {
		ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock}}
		panic("can not call with inline nodes.")
	}))

	require.Len(t, got, 2)
	require.NotNil(t, got[0].Part)
	require.Error(t, got[1].Error)
	assert.Contains(t, got[1].Error.Error(), "reader panicked")
	assert.Contains(t, got[1].Error.Error(), "can not call with inline nodes.")
	assert.Contains(t, got[1].Error.Error(), "core/format_test.TestStreamPartsTurnsAPanicIntoAnError",
		"the stack has to name the reader line")
}

// TestStreamPartsPanicWithNoConsumerLeft covers a caller that walked away: the
// context is cancelled, the send is abandoned, and the goroutine still closes
// the channel rather than blocking on a buffer nobody drains.
func TestStreamPartsPanicWithNoConsumerLeft(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	ch := format.StreamParts(ctx, func(_ context.Context, out chan<- model.PartResult) error {
		for range cap(out) + 1 {
			select {
			case out <- model.PartResult{Part: &model.Part{Type: model.PartBlock}}:
			case <-ctx.Done():
				panic("abandoned")
			}
		}
		return nil
	})
	cancel()
	for range ch { //nolint:revive // draining to prove the channel closes
	}
}
