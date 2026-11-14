package event_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/bowrain/core/event"
)

// A recipe change survives the trip through an event's Data map, and counts
// each kind of change as the feed counts it.
func TestRecipeChange_RoundTripAndCounts(t *testing.T) {
	change := event.RecipeChange{
		WorkspaceSlug: "acme", Source: "push", Stream: "main",
		Collections: []event.CollectionChange{
			{Name: "site", Change: event.CollectionAdded, After: map[string]string{"product": "kapi"}},
			{Name: "docs", Change: event.CollectionCoordinates, Before: map[string]string{"product": "kapi"}, After: map[string]string{"product": "bowrain"}},
		},
		Settings: []event.SettingChange{{Name: "converge_policy", Before: "on-push", After: "manual"}},
		Targets:  &event.TargetsChange{Before: []string{"nb", "de"}, After: []string{"nb", "fr", "sv"}},
	}
	back := event.ParseRecipeChange(change.Data())
	assert.Equal(t, change, back)
	assert.Equal(t, map[string]int{
		"collections_added": 1, "coordinates_changed": 1, "settings_changed": 1,
		"targets_added": 2, "targets_removed": 1,
	}, back.Counts())
	assert.Equal(t, []string{"fr", "sv", "de"}, back.Locales())
	assert.Nil(t, back.Coordinates(), "two collections have no one point")
	assert.False(t, back.Empty())
}

func TestRecipeChange_Empty(t *testing.T) {
	assert.True(t, event.RecipeChange{}.Empty())
	assert.True(t, event.RecipeChange{Targets: &event.TargetsChange{Before: []string{"nb"}, After: []string{"nb"}}}.Empty(),
		"the same languages in another request change nothing")
	assert.True(t, event.ParseRecipeChange(map[string]string{"recipe_diff": "{"}).Empty())
}
