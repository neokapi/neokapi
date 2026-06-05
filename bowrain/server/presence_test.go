package server

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPresenceStoreUpdate(t *testing.T) {
	ps := newPresenceStore()

	ps.Update("proj-1", &presenceEntry{
		UserID:   "u1",
		UserName: "Alice",
		ItemName: "file.html",
		BlockID:  "b1",
	})

	entries := ps.List("proj-1")
	require.Len(t, entries, 1)
	assert.Equal(t, "Alice", entries[0].UserName)
	assert.Equal(t, "file.html", entries[0].ItemName)
	assert.Equal(t, "b1", entries[0].BlockID)
}

func TestPresenceStoreUpdateOverwrite(t *testing.T) {
	ps := newPresenceStore()

	ps.Update("proj-1", &presenceEntry{UserID: "u1", UserName: "Alice", ItemName: "a.html"})
	ps.Update("proj-1", &presenceEntry{UserID: "u1", UserName: "Alice", ItemName: "b.html"})

	entries := ps.List("proj-1")
	require.Len(t, entries, 1)
	assert.Equal(t, "b.html", entries[0].ItemName)
}

func TestPresenceStoreMultipleUsers(t *testing.T) {
	ps := newPresenceStore()

	ps.Update("proj-1", &presenceEntry{UserID: "u1", UserName: "Alice"})
	ps.Update("proj-1", &presenceEntry{UserID: "u2", UserName: "Bob"})

	entries := ps.List("proj-1")
	assert.Len(t, entries, 2)

	names := map[string]bool{}
	for _, e := range entries {
		names[e.UserName] = true
	}
	assert.True(t, names["Alice"])
	assert.True(t, names["Bob"])
}

func TestPresenceStoreMultipleProjects(t *testing.T) {
	ps := newPresenceStore()

	ps.Update("proj-1", &presenceEntry{UserID: "u1", UserName: "Alice"})
	ps.Update("proj-2", &presenceEntry{UserID: "u2", UserName: "Bob"})

	assert.Len(t, ps.List("proj-1"), 1)
	assert.Len(t, ps.List("proj-2"), 1)
	assert.Empty(t, ps.List("proj-3"))
}

func TestPresenceStoreRemove(t *testing.T) {
	ps := newPresenceStore()

	ps.Update("proj-1", &presenceEntry{UserID: "u1", UserName: "Alice"})
	ps.Update("proj-1", &presenceEntry{UserID: "u2", UserName: "Bob"})

	ps.Remove("proj-1", "u1")
	entries := ps.List("proj-1")
	require.Len(t, entries, 1)
	assert.Equal(t, "Bob", entries[0].UserName)
}

func TestPresenceStoreRemoveLastUser(t *testing.T) {
	ps := newPresenceStore()

	ps.Update("proj-1", &presenceEntry{UserID: "u1", UserName: "Alice"})
	ps.Remove("proj-1", "u1")

	entries := ps.List("proj-1")
	assert.Empty(t, entries)
}

func TestPresenceStoreRemoveNonexistent(t *testing.T) {
	ps := newPresenceStore()

	// Should not panic.
	ps.Remove("proj-1", "u1")
	ps.Remove("nonexistent", "u1")

	assert.Empty(t, ps.List("proj-1"))
}

func TestPresenceStoreListEmpty(t *testing.T) {
	ps := newPresenceStore()
	entries := ps.List("proj-1")
	assert.NotNil(t, entries)
	assert.Empty(t, entries)
}

func TestPresenceStoreConcurrent(t *testing.T) {
	ps := newPresenceStore()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			uid := fmt.Sprintf("u%d", i)
			ps.Update("proj-1", &presenceEntry{UserID: uid, UserName: uid})
			ps.List("proj-1")
			if i%2 == 0 {
				ps.Remove("proj-1", uid)
			}
		})
	}
	wg.Wait()

	// Should not panic, and at least some entries remain.
	entries := ps.List("proj-1")
	assert.NotNil(t, entries)
}
