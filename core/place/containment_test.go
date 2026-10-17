package place_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/place"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// containmentFixture is testdata/containment.json, the conformance cases that
// core/place and @neokapi/context-atlas both run.
type containmentFixture struct {
	Places []struct {
		ID       string    `json:"id"`
		Name     string    `json:"name"`
		Type     string    `json:"type"`
		Declared bool      `json:"declared"`
		Within   []string  `json:"within"`
		Of       []string  `json:"of"`
		Centroid []float64 `json:"centroid"`
	} `json:"places"`
	Contains []struct {
		Outer, Inner string
		Want         bool
	} `json:"contains"`
	Overlaps []struct {
		A, B string
		Want bool
	} `json:"overlaps"`
	Paths []struct {
		ID   string   `json:"id"`
		Want []string `json:"want"`
	} `json:"paths"`
}

func loadFixture(t *testing.T) (containmentFixture, *place.Gazetteer) {
	t.Helper()
	data, err := os.ReadFile("testdata/containment.json")
	require.NoError(t, err)
	var fx containmentFixture
	require.NoError(t, json.Unmarshal(data, &fx))
	declared := map[string]place.Place{}
	for _, p := range fx.Places {
		if !p.Declared {
			continue
		}
		d := place.Place{Name: p.Name, Type: place.Type(p.Type), Within: p.Within, Of: p.Of}
		if len(p.Centroid) == 2 {
			d.Centroid = &place.LatLon{Lat: p.Centroid[0], Lon: p.Centroid[1]}
		}
		declared[p.ID] = d
	}
	g, err := place.New(declared)
	require.NoError(t, err)
	return fx, g
}

// TestContainmentFixture_BuiltinsMatchTheGazetteer holds the fixture's
// built-in entries to the built-in gazetteer, so the cases the map runs are
// cases about the real places.
func TestContainmentFixture_BuiltinsMatchTheGazetteer(t *testing.T) {
	fx, g := loadFixture(t)
	for _, want := range fx.Places {
		got, ok := g.Place(want.ID)
		require.True(t, ok, want.ID)
		assert.Equal(t, place.Type(want.Type), got.Type, want.ID)
		assert.Equal(t, want.Within, got.Within, want.ID)
		assert.Equal(t, want.Of, got.Of, want.ID)
		assert.Equal(t, want.Declared, got.Declared, want.ID)
		if !want.Declared {
			assert.Equal(t, want.Name, got.Name, want.ID)
		}
	}
}

func TestContainmentFixture_Contains(t *testing.T) {
	fx, g := loadFixture(t)
	for _, c := range fx.Contains {
		t.Run(c.Outer+"⊇"+c.Inner, func(t *testing.T) {
			assert.Equal(t, c.Want, g.Contains(c.Outer, c.Inner))
		})
	}
}

func TestContainmentFixture_Overlaps(t *testing.T) {
	fx, g := loadFixture(t)
	for _, c := range fx.Overlaps {
		t.Run(c.A+"∩"+c.B, func(t *testing.T) {
			assert.Equal(t, c.Want, g.Overlaps(c.A, c.B))
			assert.Equal(t, c.Want, g.Overlaps(c.B, c.A), "overlap is symmetric")
		})
	}
}

func TestContainmentFixture_Paths(t *testing.T) {
	fx, g := loadFixture(t)
	for _, c := range fx.Paths {
		t.Run(c.ID, func(t *testing.T) {
			assert.Equal(t, c.Want, g.Path(c.ID))
		})
	}
}
