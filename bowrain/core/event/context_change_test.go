package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A context change survives the round trip through an event's Data map, and
// names its item kind and operation when every count shares them.
func TestContextChange_RoundTrip(t *testing.T) {
	counts := ContextCounts{}
	counts.Add(ContextItemNote, ContextOpAdd, 2)
	counts.Add(ContextItemNote, ContextOpAdd, 0)
	c := ContextChange{
		WorkspaceSlug: "acme", ActorKind: ContextActorPerson, ActorName: "Ada",
		Source: "push", Counts: counts, Extra: map[string]string{"segment": "log/x"},
	}
	data := c.Data()
	assert.Equal(t, map[string]string{
		"workspace_slug": "acme", "actor_kind": "person", "actor_name": "Ada", "source": "push",
		"item_kind": "note", "operation": "add", "count": "2", "counts": `{"note":{"add":2}}`,
		"segment": "log/x",
	}, data)

	back := ParseContextChange(data)
	assert.Equal(t, counts, back.Counts)
	assert.Equal(t, ContextItemNote, back.ItemKind)
	assert.Equal(t, ContextOpAdd, back.Operation)

	counts.Add(ContextItemTermRule, ContextOpKeep, 1)
	data = ContextChange{Counts: counts}.Data()
	assert.NotContains(t, data, "item_kind")
	assert.NotContains(t, data, "operation")
	assert.Equal(t, "3", data["count"])
	assert.Equal(t, "added 2 notes · kept 1 term rule", counts.Describe())

	assert.Empty(t, ParseContextChange(map[string]string{"counts": "not json"}).Counts)
}
