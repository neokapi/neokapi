package terms

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Writing a concept the store already holds, with no timestamp of its own,
// leaves its updated_at where it was; a changed concept moves it.
func TestAddConcept_UnchangedKeepsItsTimestamp(t *testing.T) {
	type conceptStore interface {
		AddConcept(context.Context, Concept) error
		GetConcept(context.Context, string) (Concept, bool, error)
	}
	stores := map[string]conceptStore{"memory": NewInMemoryStore(), "sqlite": dntStore(t)}
	for name, store := range stores {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			c := Concept{ID: "c1", Definition: "The product", Terms: []Term{{Text: "Fernwell Ledger", Locale: "en", Status: model.TermPreferred}}}
			require.NoError(t, store.AddConcept(ctx, c))
			first, _, err := store.GetConcept(ctx, "c1")
			require.NoError(t, err)

			time.Sleep(1100 * time.Millisecond) // past the second the store stamps at
			require.NoError(t, store.AddConcept(ctx, c))
			again, _, err := store.GetConcept(ctx, "c1")
			require.NoError(t, err)
			assert.Equal(t, first.UpdatedAt, again.UpdatedAt, "the same concept again is not a change")

			c.Definition = "The product, always in full"
			require.NoError(t, store.AddConcept(ctx, c))
			changed, _, err := store.GetConcept(ctx, "c1")
			require.NoError(t, err)
			assert.True(t, changed.UpdatedAt.After(first.UpdatedAt), "a changed concept moves updated_at")
		})
	}
}
