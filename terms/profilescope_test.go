package terms

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAtPoint: a concept holds where its profile governs and where the point
// sits at every coordinate it names. An axis it leaves out is one it holds
// across.
func TestAtPoint(t *testing.T) {
	scoped := func(id, profile string, coordinates map[string]string) Concept {
		c := Concept{ID: id}
		c.ScopeToPoint(profile, coordinates)
		return c
	}
	concepts := []Concept{
		{ID: "everywhere"},
		scoped("quickcast", "quickcast", nil),
		scoped("app", "quickcast", map[string]string{"product": "quickcast", "channel": "app"}),
		scoped("docs", "quickcast", map[string]string{"product": "quickcast", "channel": "docs"}),
		scoped("product", "quickcast", map[string]string{"product": "quickcast"}),
		scoped("brand", "", map[string]string{"brand": "acme"}),
	}
	tests := []struct {
		name        string
		profile     string
		coordinates map[string]string
		want        []string
	}{
		{
			name:        "the app's strings",
			profile:     "quickcast",
			coordinates: map[string]string{"product": "quickcast", "channel": "app"},
			want:        []string{"everywhere", "quickcast", "app", "product"},
		},
		{
			name:        "the documentation",
			profile:     "quickcast",
			coordinates: map[string]string{"product": "quickcast", "channel": "docs"},
			want:        []string{"everywhere", "quickcast", "docs", "product"},
		},
		{
			name:        "another profile",
			profile:     "relaunch",
			coordinates: map[string]string{"product": "relaunch", "channel": "app", "brand": "acme"},
			want:        []string{"everywhere", "brand"},
		},
		{
			name: "the default point",
			want: []string{"everywhere"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, c := range AtPoint(concepts, tt.profile, tt.coordinates) {
				got = append(got, c.ID)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestScopeToPointRoundTrips: the coordinates read back as written, and
// unscoping clears both the profile and the coordinates.
func TestScopeToPointRoundTrips(t *testing.T) {
	var c Concept
	c.ScopeToPoint("quickcast", map[string]string{"channel": "app", "product": "quickcast"})
	assert.Equal(t, "quickcast", c.Profile())
	assert.Equal(t, "channel=app,product=quickcast", c.Properties[PropCoordinates], "written in axis order")
	assert.Equal(t, map[string]string{"channel": "app", "product": "quickcast"}, c.Coordinates())

	c.ScopeToPoint("quickcast", map[string]string{"product": "quickcast"})
	assert.Equal(t, map[string]string{"product": "quickcast"}, c.Coordinates(), "a rescope replaces the coordinates")

	c.Unscope()
	assert.Empty(t, c.Profile())
	assert.Nil(t, c.Coordinates())
}
