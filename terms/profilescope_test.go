package terms

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/place"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestAtPointIn: a concept scoped to a typed axis holds at the values inside
// its own, read through the project's axis space.
func TestAtPointIn(t *testing.T) {
	places, err := place.New(map[string]place.Place{
		"nordics": {Name: "Nordics", Of: []string{"no", "se", "dk", "fi", "is"}},
		"oslo":    {Name: "Oslo", Type: place.TypeLocality, Within: []string{"no-03"}},
	})
	require.NoError(t, err)
	space := axis.NewSpace(map[string]axis.Spec{
		"market": {Kind: axis.KindGeo},
		"age":    {Kind: axis.KindRange},
	}, places)
	nordic := Concept{ID: "nordic"}
	nordic.ScopeToPoint("", map[string]string{"market": "nordics"})
	teens := Concept{ID: "teens"}
	teens.ScopeToPoint("", map[string]string{"age": "13-17"})
	ids := func(cs []Concept) []string {
		out := []string{}
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return out
	}
	all := []Concept{nordic, teens}
	assert.Equal(t, []string{"nordic"}, ids(AtPointIn(all, "", map[string]string{"market": "no"}, space)))
	assert.Equal(t, []string{"nordic"}, ids(AtPointIn(all, "", map[string]string{"market": "oslo"}, space)), "a declared city sits within the Nordics")
	assert.Empty(t, ids(AtPointIn(all, "", map[string]string{"market": "de"}, space)))
	assert.Equal(t, []string{"teens"}, ids(AtPointIn(all, "", map[string]string{"age": "14-16"}, space)))
	assert.Empty(t, ids(AtPointIn(all, "", map[string]string{"age": "18-"}, space)))
	assert.Empty(t, ids(AtPoint(all, "", map[string]string{"market": "no"})), "without a space every axis is equality")
}

// TestInForceAt: a term past its window stops answering, and a concept none of
// whose terms answer is dropped.
func TestInForceAt(t *testing.T) {
	end := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	concepts := []Concept{
		{ID: "sale", Terms: []Term{
			{Text: "Spring sale", Status: model.TermForbidden, Validity: &graph.Validity{ValidTo: &end}},
			{Text: "Summer sale", Status: model.TermPreferred},
		}},
		{ID: "gone", Terms: []Term{{Text: "Old", Validity: &graph.Validity{ValidTo: &end}}}},
		{ID: "always", Terms: []Term{{Text: "kapi"}}},
	}
	before := InForceAt(concepts, end.Add(-time.Hour))
	assert.Len(t, before, 3)
	assert.Len(t, before[0].Terms, 2)

	after := InForceAt(concepts, end)
	require.Len(t, after, 2)
	assert.Equal(t, "sale", after[0].ID)
	assert.Equal(t, []string{"Summer sale"}, []string{after[0].Terms[0].Text})
	assert.Equal(t, "always", after[1].ID)
	assert.Len(t, concepts[0].Terms, 2, "the input is left as it was")

	assert.Len(t, InForceAt(concepts, time.Time{}), 3)
}
