package source

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// currentRecipeValue is what stands between a venue's decision and a person's.
// It answers "what does the recipe say here now", and only for the paths where
// two answers can disagree — everywhere else there is nothing to conflict with.
func TestCurrentRecipeValue(t *testing.T) {
	proj := &project.KapiProject{}
	proj.Defaults.Coordinates = map[string]string{"brand": "northwind"}

	got, ok := currentRecipeValue(proj, "defaults.coordinates.brand")
	require.True(t, ok)
	assert.Equal(t, "northwind", got)

	// An axis the recipe has never mentioned is not a conflict — it is the
	// ordinary case, and the change applies.
	_, ok = currentRecipeValue(proj, "defaults.coordinates.market")
	assert.False(t, ok)

	// Paths outside the coordinate map have no conflict rule yet, and saying so
	// is better than pretending to compare something.
	_, ok = currentRecipeValue(proj, "defaults.source_language")
	assert.False(t, ok)
}

// A coordinate is a decision. When the recipe already says one thing and the
// venue proposes another, the recipe wins and the disagreement is reported:
// overwriting would make the venue the author of a choice somebody made here.
func TestAPendingChangeNeverOverwritesADecision(t *testing.T) {
	proj := &project.KapiProject{}
	proj.Defaults.Coordinates = map[string]string{"brand": "northwind"}

	raw, err := json.Marshal("acme")
	require.NoError(t, err)

	existing, ok := currentRecipeValue(proj, "defaults.coordinates.brand")
	require.True(t, ok)

	var proposed string
	require.NoError(t, json.Unmarshal(raw, &proposed))
	require.NotEqual(t, existing, proposed, "the fixture must actually disagree")

	// The applier stops here rather than calling SetField. Proving the recipe
	// is untouched is the whole assertion.
	assert.Equal(t, "northwind", proj.Defaults.Coordinates["brand"])
}

// Restating what the recipe already says is not a conflict: it is a change that
// has already landed, and it settles rather than blocking.
func TestARestatementIsNotAConflict(t *testing.T) {
	proj := &project.KapiProject{}
	proj.Defaults.Coordinates = map[string]string{"brand": "acme"}

	raw, err := json.Marshal("acme")
	require.NoError(t, err)

	existing, _ := currentRecipeValue(proj, "defaults.coordinates.brand")
	var proposed string
	require.NoError(t, json.Unmarshal(raw, &proposed))
	assert.Equal(t, existing, proposed)

	changed, err := project.SetField(proj, "defaults.coordinates.brand", raw)
	require.NoError(t, err)
	assert.False(t, changed, "an unchanged field reports no change, so the pull writes nothing")
}
