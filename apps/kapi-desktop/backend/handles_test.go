package backend

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closerStub tracks Close calls.
type closerStub struct {
	closed bool
}

func (c *closerStub) Close() error {
	c.closed = true
	return nil
}

func TestHandleStore_OpenAndGet(t *testing.T) {
	s := newHandleStore[*closerStub]()
	stub := &closerStub{}
	h := s.Open(stub)

	assert.NotEmpty(t, h)
	assert.Equal(t, 1, s.Count())

	got, ok := s.Get(h)
	require.True(t, ok)
	assert.Same(t, stub, got)
}

func TestHandleStore_GetMissing(t *testing.T) {
	s := newHandleStore[*closerStub]()
	_, ok := s.Get("nonexistent")
	assert.False(t, ok)
}

func TestHandleStore_Close(t *testing.T) {
	s := newHandleStore[*closerStub]()
	stub := &closerStub{}
	h := s.Open(stub)

	err := s.Close(h)
	require.NoError(t, err)
	assert.True(t, stub.closed)
	assert.Equal(t, 0, s.Count())

	_, ok := s.Get(h)
	assert.False(t, ok)
}

func TestHandleStore_CloseNonexistent(t *testing.T) {
	s := newHandleStore[*closerStub]()
	err := s.Close("nonexistent")
	assert.NoError(t, err)
}

func TestHandleStore_CloseAll(t *testing.T) {
	s := newHandleStore[*closerStub]()
	stubs := make([]*closerStub, 5)
	for i := range stubs {
		stubs[i] = &closerStub{}
		s.Open(stubs[i])
	}

	assert.Equal(t, 5, s.Count())
	s.CloseAll()
	assert.Equal(t, 0, s.Count())

	for _, stub := range stubs {
		assert.True(t, stub.closed)
	}
}

func TestHandleStore_ConcurrentAccess(t *testing.T) {
	s := newHandleStore[*closerStub]()
	var wg sync.WaitGroup

	// Concurrent opens.
	handles := make([]string, 50)
	for i := range 50 {
		wg.Go(func() {
			handles[i] = s.Open(&closerStub{})
		})
	}
	wg.Wait()
	assert.Equal(t, 50, s.Count())

	// Concurrent gets.
	for i := range 50 {
		wg.Go(func() {
			_, ok := s.Get(handles[i])
			assert.True(t, ok)
		})
	}
	wg.Wait()

	// Concurrent closes.
	for i := range 50 {
		wg.Go(func() {
			_ = s.Close(handles[i])
		})
	}
	wg.Wait()
	assert.Equal(t, 0, s.Count())
}
