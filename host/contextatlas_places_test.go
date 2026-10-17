package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

func atlasPlace(t *testing.T, atlas ContextAtlas, id string) AtlasPlace {
	t.Helper()
	for _, p := range atlas.Places {
		if p.ID == id {
			return p
		}
	}
	require.FailNow(t, "no place "+id)
	return AtlasPlace{}
}

func atlasPlaceIDs(atlas ContextAtlas) []string {
	out := []string{}
	for _, p := range atlas.Places {
		out = append(out, p.ID)
	}
	return out
}

// TestContextAtlas_Places: the atlas carries the gazetteer a map draws from:
// the places records name, what contains them, the members of a place made of
// places, every declared place, the subdivisions of each country in the set
// and the countries of each region in it.
func TestContextAtlas_Places(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-atlas-places")
	_, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Text: "the Oslo pages name the districts",
		Evidence: []contextop.Evidence{{Path: "oslo/a.md"}},
	})
	require.NoError(t, err)

	atlas, err := app.ContextAtlas(t.Context(), ContextAtlasRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	ids := atlasPlaceIDs(atlas)
	assert.IsIncreasing(t, ids, "places are sorted by id")
	for _, id := range []string{"world", "europe", "northern-europe", "no", "no-03", "oslo", "nordics", "se", "dk", "fi", "is"} {
		assert.Contains(t, ids, id, "the record at oslo, its ancestors and the Nordics' members")
	}
	assert.Contains(t, ids, "no-46", "Norway brings its counties")
	assert.Contains(t, ids, "se-ab", "a member country brings its subdivisions")
	assert.Contains(t, ids, "de", "Europe brings its countries")
	assert.NotContains(t, ids, "us", "a country no record reaches stays out")
	assert.NotContains(t, ids, "de-by", "a country a region brings brings no subdivisions")

	oslo := atlasPlace(t, atlas, "oslo")
	assert.Equal(t, AtlasPlace{ID: "oslo", Name: "Oslo", Type: "locality", Within: []string{"no-03"}, Declared: true}, oslo)
	nordics := atlasPlace(t, atlas, "nordics")
	assert.Equal(t, "territory", nordics.Type)
	assert.Equal(t, []string{"no", "se", "dk", "fi", "is"}, nordics.Of)
	world := atlasPlace(t, atlas, "world")
	assert.Equal(t, AtlasPlace{ID: "world", Name: "World", Type: "world"}, world)
	no := atlasPlace(t, atlas, "no")
	assert.Equal(t, &[2]float64{60.47, 8.47}, no.Centroid)

	market, ok := atlasAxis(atlas, "market")
	require.True(t, ok)
	require.NotEmpty(t, market.Values)
	assert.Equal(t, AtlasValue{ID: "nordics", Label: "Nordics"}, market.Values[0], "a place made of places comes first")
	assert.Contains(t, market.Values, AtlasValue{ID: "oslo", Label: "Oslo"}, "a declared place is a value of a geo axis")
	assert.Less(t, indexOfValue(market, "no"), indexOfValue(market, "oslo"), "a country before a locality")
}

func indexOfValue(ax AtlasAxis, id string) int {
	for i, v := range ax.Values {
		if v.ID == id {
			return i
		}
	}
	return -1
}

// TestContextAtlas_PlaceGeometry: a declared place's shape file is read into
// its geometry, and a shape the atlas cannot draw is skipped with a note
// naming the recipe key.
func TestContextAtlas_PlaceGeometry(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	polygon := `{"type":"Polygon","coordinates":[[[5.2,60.3],[5.4,60.3],[5.4,60.4],[5.2,60.3]]]}`
	write("places/bergen.geojson", polygon)
	write("places/stavanger.geojson", `{"type":"Feature","properties":{"name":"Stavanger"},"geometry":`+polygon+`}`)
	write("places/tromso.json", `{"type":"FeatureCollection","features":[{"type":"Feature","geometry":{"type":"MultiPolygon","coordinates":[[[[18.9,69.6],[19.0,69.6],[19.0,69.7],[18.9,69.6]]]]}}]}`)
	write("places/road.geojson", `{"type":"LineString","coordinates":[[5.2,60.3],[5.4,60.4]]}`)
	write("places/two.geojson", `{"type":"FeatureCollection","features":[{"type":"Feature","geometry":`+polygon+`},{"type":"Feature","geometry":`+polygon+`}]}`)
	write("places/broken.geojson", `{not json`)
	write("places/large.geojson", `{"type":"Polygon","coordinates":[[`+strings.Repeat(`[5.2,60.3],`, 120000)+`[5.2,60.3]]]}`)
	write("kapi.yaml", `version: v1
id: `+projectIDFor("ctxops-atlas-geometry")+`
name: geometry
defaults:
  source_language: en
places:
  bergen: {within: no-46, shape: places/bergen.geojson}
  stavanger: {within: no-11, shape: places/stavanger.geojson}
  tromso: {within: no-55, shape: places/tromso.json}
  road: {within: no-46, shape: places/road.geojson}
  two: {within: no-46, shape: places/two.geojson}
  broken: {within: no-46, shape: places/broken.geojson}
  large: {within: no-46, shape: places/large.geojson}
axes:
  market: {kind: geo}
`)

	atlas, err := app.ContextAtlas(t.Context(), ContextAtlasRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	geometry := func(id string) string {
		t.Helper()
		g := atlasPlace(t, atlas, id).Geometry
		require.NotNil(t, g, id)
		data, err := json.Marshal(g)
		require.NoError(t, err)
		return string(data)
	}
	assert.JSONEq(t, polygon, geometry("bergen"), "a bare polygon")
	assert.JSONEq(t, polygon, geometry("stavanger"), "a feature's polygon, without its properties")
	assert.JSONEq(t, `{"type":"MultiPolygon","coordinates":[[[[18.9,69.6],[19.0,69.6],[19.0,69.7],[18.9,69.6]]]]}`, geometry("tromso"))
	assert.Equal(t, "places/bergen.geojson", atlasPlace(t, atlas, "bergen").Shape)
	for _, id := range []string{"road", "two", "broken", "large"} {
		assert.Nil(t, atlasPlace(t, atlas, id).Geometry, id)
	}
	assert.Contains(t, atlas.Notes, "places.road.shape: places/road.geojson holds a LineString, not a polygon, so the place has no outline.")
	assert.Contains(t, atlas.Notes, "places.two.shape: places/two.geojson holds 2 features; a place's shape is one polygon feature, so the place has no outline.")
	assert.Contains(t, atlas.Notes, "places.broken.shape: places/broken.geojson is not GeoJSON, so the place has no outline.")
	assert.Contains(t, atlas.Notes, "places.large.shape: places/large.geojson is larger than 1 MiB, so the place has no outline.")

	require.NoError(t, os.Remove(filepath.Join(root, "places", "bergen.geojson")))
	require.NoError(t, os.WriteFile(filepath.Join(root, "places", "bergen.geojson"), []byte(polygon), 0o000))
	if f, err := os.Open(filepath.Join(root, "places", "bergen.geojson")); err == nil {
		f.Close()
		t.Skip("this user reads files without read permission")
	}
	atlas, err = app.ContextAtlas(t.Context(), ContextAtlasRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Nil(t, atlasPlace(t, atlas, "bergen").Geometry)
	assert.Contains(t, atlas.Notes, "places.bergen.shape: places/bergen.geojson could not be read, so the place has no outline.")
}

// The derived region of a record's locale is the place the locale names: the
// subdivision of a -u-sd- key, else its region subtag.
func TestSetLocale_Region(t *testing.T) {
	tests := map[string]string{"nb-NO-u-sd-no03": "no-03", "nb-NO": "no", "es-419": "latin-america-and-the-caribbean", "nb": ""}
	for locale, want := range tests {
		coordinates := map[string]string{}
		setLocale(coordinates, locale)
		assert.Equal(t, locale, coordinates[AtlasAxisLocale])
		assert.Equal(t, want, coordinates[AtlasAxisRegion], locale)
	}
}

func TestPolygonGeometry(t *testing.T) {
	tests := []struct {
		name, in, want, wantErr string
	}{
		{"polygon", `{"type":"Polygon","coordinates":[[[0,0],[1,0],[0,1],[0,0]]]}`, `{"type":"Polygon","coordinates":[[[0,0],[1,0],[0,1],[0,0]]]}`, ""},
		{"multipolygon", `{"type":"MultiPolygon","coordinates":[]}`, `{"type":"MultiPolygon","coordinates":[]}`, ""},
		{"feature", `{"type":"Feature","geometry":{"type":"Polygon","coordinates":[]},"properties":{}}`, `{"type":"Polygon","coordinates":[]}`, ""},
		{"feature without geometry", `{"type":"Feature","geometry":null}`, "", "holds a feature with no geometry"},
		{"point", `{"type":"Point","coordinates":[0,0]}`, "", "holds a Point, not a polygon"},
		{"polygon without coordinates", `{"type":"Polygon"}`, "", "holds a Polygon with no coordinates"},
		{"empty collection", `{"type":"FeatureCollection","features":[]}`, "", "holds 0 features; a place's shape is one polygon feature"},
		{"not geojson", `[1,2]`, "", "is not GeoJSON"},
		{"no type", `{}`, "", "is not GeoJSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := polygonGeometry([]byte(tt.in))
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			data, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
		})
	}
}
