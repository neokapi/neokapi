package host

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/axis"
)

// A group or region carries the countries it holds, so a map can draw where a
// rule scoped to it holds; a country and the whole world carry none.
func TestGeoMembers(t *testing.T) {
	space := axis.Space{"v": {Kind: axis.KindGeo, Groups: map[string][]string{
		"nordics":     {"no", "se", "dk", "fi", "is"},
		"africa-asia": {"africa", "asia"},
	}}}

	var ids []string
	for _, m := range geoMembers(space, "nordics") {
		ids = append(ids, m.ID)
		assert.NotEmpty(t, m.Label)
	}
	assert.ElementsMatch(t, []string{"no", "se", "dk", "fi", "is"}, ids)

	assert.Empty(t, geoMembers(space, "no"), "a country is its own tile")
	assert.Empty(t, geoMembers(space, axis.World), "the world is never listed whole")

	northern := geoMembers(space, "northern-europe")
	assert.NotEmpty(t, northern)
	assert.LessOrEqual(t, len(northern), atlasMaxMembers)
	assert.NotEmpty(t, geoMembers(space, "africa"))
	assert.Empty(t, geoMembers(space, "africa-asia"), "a group past the cap lists nothing")
}
