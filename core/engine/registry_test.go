package engine_test

import (
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEngine struct{ name string }

func TestRegistry(t *testing.T) {
	errNone := errors.New("demo: no engine registered")
	r := engine.NewRegistry[*fakeEngine]("demo", errNone)

	// Empty: nothing available, Open returns the noEngine error.
	assert.False(t, r.Available(""))
	_, err := r.Open("")
	require.ErrorIs(t, err, errNone)

	// The first registered engine becomes the default.
	r.Register("a", func() (*fakeEngine, error) { return &fakeEngine{name: "a"}, nil })
	r.Register("b", func() (*fakeEngine, error) { return &fakeEngine{name: "b"}, nil })

	assert.True(t, r.Available(""))
	assert.True(t, r.Available("a"))
	assert.True(t, r.Available("b"))
	assert.False(t, r.Available("missing"))

	got, err := r.Open("")
	require.NoError(t, err)
	assert.Equal(t, "a", got.name, "the first registered engine is the default")

	got, err = r.Open("b")
	require.NoError(t, err)
	assert.Equal(t, "b", got.name)

	// An unknown name errors with the label and wraps noEngine.
	_, err = r.Open("missing")
	require.ErrorIs(t, err, errNone)
	assert.Contains(t, err.Error(), "demo: engine \"missing\"")

	// A nil factory is ignored.
	r.Register("nil", nil)
	assert.False(t, r.Available("nil"))

	// ResetForTest clears everything.
	r.ResetForTest()
	assert.False(t, r.Available(""))
	assert.False(t, r.Available("a"))
}
