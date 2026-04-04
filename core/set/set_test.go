package set_test

import (
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/set"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	t.Parallel()
	s := set.New("a", "b", "c", "a")
	assert.Equal(t, 3, s.Len())
	assert.True(t, s.Contains("a"))
	assert.True(t, s.Contains("b"))
	assert.True(t, s.Contains("c"))
}

func TestAdd(t *testing.T) {
	t.Parallel()
	s := set.New[string]()
	s.Add("x")
	s.Add("x") // duplicate
	assert.Equal(t, 1, s.Len())
	assert.True(t, s.Contains("x"))
}

func TestRemove(t *testing.T) {
	t.Parallel()
	s := set.New("a", "b")
	s.Remove("a")
	assert.False(t, s.Contains("a"))
	assert.Equal(t, 1, s.Len())
	s.Remove("nonexistent") // no panic
}

func TestContains(t *testing.T) {
	t.Parallel()
	s := set.New(1, 2, 3)
	assert.True(t, s.Contains(2))
	assert.False(t, s.Contains(4))
}

func TestItems(t *testing.T) {
	t.Parallel()
	s := set.New("c", "a", "b")
	items := s.Items()
	sort.Strings(items)
	assert.Equal(t, []string{"a", "b", "c"}, items)
}

func TestEmpty(t *testing.T) {
	t.Parallel()
	s := set.New[int]()
	assert.Equal(t, 0, s.Len())
	assert.False(t, s.Contains(0))
	assert.Empty(t, s.Items())
}
