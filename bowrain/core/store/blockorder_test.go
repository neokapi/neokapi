package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The precedence between the two cursors and the Order field lives in one
// place, so the stores cannot each decide it for themselves. What each case
// pins is which of the three won.
func TestOrderingOf(t *testing.T) {
	cases := []struct {
		name  string
		query BlockQuery
		want  BlockOrdering
	}{
		{
			name:  "a bare query reads in id order",
			query: BlockQuery{ProjectID: "p"},
			want:  BlockOrdering{},
		},
		{
			name:  "a listing asks for document order",
			query: BlockQuery{ProjectID: "p", Order: BlockOrderDocument},
			want:  BlockOrdering{Document: true},
		},
		{
			name:  "the walk's cursor stays in id order",
			query: BlockQuery{ProjectID: "p", AfterID: "b3"},
			want:  BlockOrdering{},
		},
		{
			name:  "the backwards id cursor reads away from itself",
			query: BlockQuery{ProjectID: "p", BeforeID: "b3"},
			want:  BlockOrdering{Backward: true},
		},
		{
			name:  "an id cursor beats a listing's order, so a walk cannot skip a row",
			query: BlockQuery{ProjectID: "p", AfterID: "b3", Order: BlockOrderDocument},
			want:  BlockOrdering{},
		},
		{
			name:  "a positional cursor brings document order with it",
			query: BlockQuery{ProjectID: "p", DocumentAfter: "b3"},
			want:  BlockOrdering{Document: true, After: "b3"},
		},
		{
			name:  "the backwards positional cursor reads away from itself",
			query: BlockQuery{ProjectID: "p", DocumentBefore: "b3"},
			want:  BlockOrdering{Document: true, Backward: true, Before: "b3"},
		},
		{
			name:  "an id cursor beats a positional one and drops it",
			query: BlockQuery{ProjectID: "p", AfterID: "b1", DocumentBefore: "b3"},
			want:  BlockOrdering{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, OrderingOf(tc.query))
		})
	}
}
