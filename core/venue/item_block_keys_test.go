package venue_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
)

// ItemBlockKeys is how a push says a string is GONE. What it carries is what
// CHANGED, and a deletion changes nothing it can send — so the declaration is
// the only channel a removal has.
func TestItemBlockKeys(t *testing.T) {
	block := func(id string) *model.Block { return model.NewBlock(id, "text") }
	named := func(id, name string) *model.Block {
		b := model.NewBlock(id, "text")
		b.Name = name
		return b
	}

	t.Run("names every block of every item it read", func(t *testing.T) {
		got := venue.ItemBlockKeys(map[string][]*model.Block{
			"en.json":  {block("greeting"), block("farewell")},
			"app.yaml": {block("title")},
		})
		assert.Equal(t, map[string][]string{
			"en.json":  {"farewell", "greeting"},
			"app.yaml": {"title"},
		}, got)
	})

	t.Run("uses the key the far side stores blocks under", func(t *testing.T) {
		// convergence.BlockKey prefers Name — the structural key a format
		// reader gives — and falls back to the id. The declaration has to name
		// blocks the same way the store recorded them or it would prune every
		// one of them.
		got := venue.ItemBlockKeys(map[string][]*model.Block{
			"en.json": {named("tu1", "$.greeting")},
		})
		assert.Equal(t, map[string][]string{"en.json": {"$.greeting"}}, got)
	})

	// An item that read to nothing declares an empty set, not absence: a file
	// whose last translatable string was deleted is exactly the case this
	// exists for, and dropping it would leave the emptiest item the only one
	// that never gets cleaned.
	t.Run("an item that reads to nothing still declares itself", func(t *testing.T) {
		got := venue.ItemBlockKeys(map[string][]*model.Block{"en.json": {}})
		assert.Equal(t, map[string][]string{"en.json": {}}, got)
	})

	// Absence is silence. A producer that read nothing declares nothing, and
	// the far side removes nothing — the rule BlockPropertyKeys follows one
	// level down.
	t.Run("a scan of nothing declares nothing", func(t *testing.T) {
		assert.Nil(t, venue.ItemBlockKeys(nil))
		assert.Nil(t, venue.ItemBlockKeys(map[string][]*model.Block{}))
	})

	t.Run("skips blocks and items with no key to name", func(t *testing.T) {
		got := venue.ItemBlockKeys(map[string][]*model.Block{
			"en.json": {block("greeting"), nil, block("")},
			"":        {block("orphan")},
		})
		assert.Equal(t, map[string][]string{"en.json": {"greeting"}}, got)
	})

	t.Run("names a repeated key once", func(t *testing.T) {
		got := venue.ItemBlockKeys(map[string][]*model.Block{
			"en.json": {block("greeting"), block("greeting")},
		})
		assert.Equal(t, map[string][]string{"en.json": {"greeting"}}, got)
	})
}
