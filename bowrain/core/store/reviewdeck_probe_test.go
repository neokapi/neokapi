package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deckContentStore is a content store that keeps the review deck.
type deckContentStore struct {
	ContentStore
	ReviewDeckStore
}

// decorator wraps a content store and names it, as the event-emitting store
// does.
type decorator struct {
	ContentStore
	inner ContentStore
}

func (d decorator) Unwrap() ContentStore { return d.inner }

func TestReviewDeckStoresLooksThroughDecorators(t *testing.T) {
	inner := deckContentStore{}
	got, ok := ReviewDeckStores(decorator{inner: inner})
	require.True(t, ok)
	assert.Equal(t, inner, got)

	_, ok = ReviewDeckStores(decorator{inner: decorator{}})
	assert.False(t, ok)

	_, ok = ReviewDeckStores(nil)
	assert.False(t, ok)
}
