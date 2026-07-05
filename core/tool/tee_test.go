package tool

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestTee_TwoOutputs(t *testing.T) {
	in := make(chan *model.Part, 3)
	out1 := make(chan *model.Part, 3)
	out2 := make(chan *model.Part, 3)

	parts := []*model.Part{
		{Type: model.PartBlock},
		{Type: model.PartData},
		{Type: model.PartMedia},
	}
	for _, p := range parts {
		in <- p
	}
	close(in)

	Tee(t.Context(), in, out1, out2)

	// Both outputs should receive all 3 parts
	var got1, got2 []*model.Part
	for p := range out1 {
		got1 = append(got1, p)
	}
	for p := range out2 {
		got2 = append(got2, p)
	}

	assert.Len(t, got1, 3)
	assert.Len(t, got2, 3)
	assert.Equal(t, model.PartBlock, got1[0].Type)
	assert.Equal(t, model.PartBlock, got2[0].Type)
}

func TestTee_SingleOutput(t *testing.T) {
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	in <- &model.Part{Type: model.PartBlock}
	close(in)

	Tee(t.Context(), in, out)

	p := <-out
	assert.Equal(t, model.PartBlock, p.Type)

	// Channel should be closed
	_, ok := <-out
	assert.False(t, ok)
}

func TestTee_Empty(t *testing.T) {
	in := make(chan *model.Part)
	out := make(chan *model.Part, 1)
	close(in)

	Tee(t.Context(), in, out)

	_, ok := <-out
	assert.False(t, ok) // output should be closed
}

// TestTee_CancelUnblocksBlockedSend verifies Tee returns (and closes its
// outputs) when the context is cancelled while a send is blocked on an
// unread output channel — previously a permanent goroutine leak.
func TestTee_CancelUnblocksBlockedSend(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	in := make(chan *model.Part, 2)
	out := make(chan *model.Part) // unbuffered, never read

	in <- &model.Part{Type: model.PartBlock}
	in <- &model.Part{Type: model.PartData}
	close(in)

	done := make(chan struct{})
	go func() {
		defer close(done)
		Tee(ctx, in, out)
	}()

	cancel()
	<-done // returns instead of blocking forever

	_, ok := <-out
	assert.False(t, ok, "output must be closed after cancellation")
}
