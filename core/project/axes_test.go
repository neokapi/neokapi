package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/place"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const axesRecipe = `version: v1
name: market-app
defaults:
  source_language: en
  target_languages: [nb-NO, de-DE]
  coordinates:
    market: nordics
    audience: general
    brand: kapimart
profiles:
  kapimart:
    channels: [web]
places:
  nordics:
    name: Nordics
    of: [no, se, dk, fi, is]
  oslo:
    name: Oslo
    type: locality
    within: no-03
    centroid: [59.91, 10.75]
    refs: [wikidata:Q585, geonames:3143244]
    names: {nb: Oslo}
  grunerlokka:
    name: Grünerløkka
    type: neighbourhood
    within: oslo
axes:
  market:
    kind: geo
    label: Market
  audience:
    values: [general, new-customers, specialists]
    labels:
      new-customers: New customers
  age:
    kind: range
    unit: years
  lifecycle:
    kind: ordered
    values: [preview, current, sunset]
collections:
  - name: teens
    channel: kapimart/web
    coordinates:
      age: 13-17
      market: oslo
    content:
      - path: campaigns/oslo/en/grunerlokka.md
        coordinates:
          market: grunerlokka
          campaign: spring
      - path: campaigns/oslo/en/*.md
  - name: docs
    content:
      - path: docs/*.md
`

func TestAxes_LoadAndType(t *testing.T) {
	p := loadRecipe(t, axesRecipe)
	require.Len(t, p.Axes, 4)
	assert.Equal(t, axis.KindGeo, p.Axes["market"].Kind)
	assert.True(t, p.AxisSpace().Contains("market", "nordics", "no"))
	assert.True(t, p.AxisSpace().Contains("market", "no", "grunerlokka"), "a declared neighbourhood sits within the country its city's county is in")
	assert.True(t, p.AxisSpace().Contains("market", "nordics", "oslo"))
	assert.False(t, p.AxisSpace().Contains("market", "oslo", "no"))
	assert.Equal(t, "Oslo", p.AxisSpace().Label("market", "oslo"))
	assert.Equal(t, []string{"age", "audience", "brand", "campaign", "lifecycle", "market"}, p.DeclaredAxisNames(),
		"an axis a content item names is a declared axis")
}

func TestAxes_PlacesLoad(t *testing.T) {
	p := loadRecipe(t, axesRecipe)
	require.Len(t, p.Places, 3)
	g := p.Gazetteer()
	assert.Same(t, g, p.Gazetteer(), "the gazetteer is built once, at load")
	oslo, ok := g.Place("oslo")
	require.True(t, ok)
	assert.True(t, oslo.Declared)
	assert.Equal(t, place.TypeLocality, oslo.Type)
	assert.Equal(t, []string{"no-03"}, oslo.Within)
	assert.Equal(t, &place.LatLon{Lat: 59.91, Lon: 10.75}, oslo.Centroid)
	assert.Equal(t, []string{"wikidata:Q585", "geonames:3143244"}, oslo.Refs)
	nordics, _ := g.Place("nordics")
	assert.Equal(t, place.TypeTerritory, nordics.Type)
	assert.Equal(t, []string{"world", "europe", "northern-europe", "no", "no-03", "oslo", "grunerlokka"}, g.Path("grunerlokka"))
}

func TestAxes_GazetteerOfAProjectBuiltInCode(t *testing.T) {
	var nilProject *KapiProject
	assert.Same(t, place.Builtin(), nilProject.Gazetteer())
	assert.Equal(t, axis.Space{}, nilProject.AxisSpace())

	p := &KapiProject{Places: map[string]place.Place{"oslo": {Name: "Oslo", Within: []string{"no-03"}}}}
	assert.True(t, p.Gazetteer().Has("oslo"), "a project assembled in code builds its gazetteer on demand")
	broken := &KapiProject{Places: map[string]place.Place{"no": {Within: []string{"europe"}}}}
	assert.Same(t, place.Builtin(), broken.Gazetteer(), "places that do not validate leave the built-in gazetteer")
}

// TestCoordinatesAt_Precedence: a point's coordinates are the defaults,
// overlaid with what its governance derives, with its collection's, then with
// those of the item that claims its path.
func TestCoordinatesAt_Precedence(t *testing.T) {
	p := loadRecipe(t, axesRecipe)
	at := func(pt GovernancePoint) map[string]string {
		t.Helper()
		rc, err := p.ResolveGovernanceFor(pt)
		require.NoError(t, err)
		return p.CoordinatesAt(pt, rc)
	}

	assert.Equal(t, map[string]string{
		"market": "grunerlokka", "campaign": "spring", "age": "13-17",
		"audience": "general", "brand": "kapimart", ProductAxis: "kapimart", ChannelAxis: "web",
	}, at(GovernancePoint{Path: "campaigns/oslo/en/grunerlokka.md"}), "the item's market wins over its collection's, which wins over the default")

	assert.Equal(t, map[string]string{
		"market": "oslo", "age": "13-17", "audience": "general", "brand": "kapimart", ProductAxis: "kapimart", ChannelAxis: "web",
	}, at(GovernancePoint{Path: "campaigns/oslo/en/launch.md"}), "a sibling the item does not claim sits at the collection's point")

	assert.Equal(t, map[string]string{
		"market": "oslo", "age": "13-17", "audience": "general", "brand": "kapimart", ProductAxis: "kapimart", ChannelAxis: "web",
	}, at(GovernancePoint{Collection: "teens"}), "a collection point has no item")

	assert.Equal(t, map[string]string{"market": "nordics", "audience": "general", "brand": "kapimart"},
		at(GovernancePoint{Path: "docs/intro.md"}), "a collection with no coordinates sits at the defaults")

	assert.Equal(t, map[string]string{"market": "nordics", "audience": "general", "brand": "kapimart", ProductAxis: "kapimart"},
		at(GovernancePoint{Profile: "kapimart"}), "a profile point has no location: defaults and what the profile derives")

	assert.Equal(t, map[string]string{"market": "nordics", "audience": "general", "brand": "kapimart"},
		at(GovernancePoint{Path: "elsewhere/readme.md"}), "a path nothing claims sits at the defaults")

	var nilProject *KapiProject
	assert.Nil(t, nilProject.CoordinatesAt(GovernancePoint{}, nil))
}

// TestCoordinatesAt_EachLayerOutranksTheOneBefore holds the order on one
// shared axis: a recipe that loaded never names a structural axis in its
// coordinates, so the order is set on a project assembled in code.
func TestCoordinatesAt_EachLayerOutranksTheOneBefore(t *testing.T) {
	p := loadRecipe(t, axesRecipe)
	pt := GovernancePoint{Path: "campaigns/oslo/en/grunerlokka.md"}
	rc, err := p.ResolveGovernanceFor(pt)
	require.NoError(t, err)

	p.Defaults.Coordinates[ProductAxis] = "from-defaults"
	assert.Equal(t, "kapimart", p.CoordinatesAt(pt, rc)[ProductAxis], "governance over defaults")
	p.Collections[0].Coordinates[ProductAxis] = "from-collection"
	assert.Equal(t, "from-collection", p.CoordinatesAt(pt, rc)[ProductAxis], "collection over governance")
	p.Collections[0].Content[0].Coordinates[ProductAxis] = "from-item"
	assert.Equal(t, "from-item", p.CoordinatesAt(pt, rc)[ProductAxis], "item over collection")
	p.Collections[0].Content[0].Coordinates["market"] = ""
	assert.Equal(t, "oslo", p.CoordinatesAt(pt, rc)["market"], "an empty value never erases the layer below")
}

// A recipe that types nothing loads as it did: every axis is an open
// category, whatever its values.
func TestAxes_UntypedRecipeLoadsUnchanged(t *testing.T) {
	p := loadRecipe(t, `version: v1
name: plain
defaults:
  coordinates:
    brand: Northsea Ltd
    market: anything at all
`)
	assert.Nil(t, p.Axes)
	assert.Equal(t, []string{"brand", "market"}, p.DeclaredAxisNames())
	assert.Same(t, place.Builtin(), p.Gazetteer())
}

func TestAxes_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		recipe  string
		wantErr string
	}{
		{
			name:    "structural axis declared",
			recipe:  "version: v1\nname: x\naxes:\n  channel: {}\n",
			wantErr: `axes.channel: "channel" is derived from a collection's channel`,
		},
		{
			name:    "unknown kind",
			recipe:  "version: v1\nname: x\naxes:\n  market: {kind: map}\n",
			wantErr: `axes.market.kind: "map" is not an axis kind`,
		},
		{
			name:    "default outside a closed axis",
			recipe:  "version: v1\nname: x\naxes:\n  audience: {values: [general]}\ndefaults:\n  coordinates:\n    audience: children\n",
			wantErr: `defaults.coordinates.audience: axis audience: "children" is not one of its values (general)`,
		},
		{
			name:    "collection geo value unknown",
			recipe:  "version: v1\nname: x\naxes:\n  market: {kind: geo}\ncollections:\n  - name: docs\n    coordinates: {market: atlantis}\n    content:\n      - path: docs/*.md\n",
			wantErr: `collections[0]: collection "docs": coordinates.market: axis market: "atlantis" is not a place.`,
		},
		{
			name:    "a geo hint names the lookup",
			recipe:  "version: v1\nname: x\naxes:\n  market: {kind: geo}\ndefaults:\n  coordinates: {market: atlantis}\n",
			wantErr: "`kapi context places atlantis`",
		},
		{
			name:    "item geo value unknown",
			recipe:  "version: v1\nname: x\naxes:\n  market: {kind: geo}\ncollections:\n  - name: docs\n    content:\n      - path: docs/*.md\n        coordinates: {market: atlantis}\n",
			wantErr: `collections[0]: collection "docs": content[0]: coordinates.market: axis market: "atlantis" is not a place.`,
		},
		{
			name:    "item structural axis",
			recipe:  "version: v1\nname: x\ncollections:\n  - name: docs\n    content:\n      - path: docs/*.md\n        coordinates: {channel: web}\n",
			wantErr: `content[0]: coordinates.channel: "channel" is derived from the item's channel`,
		},
		{
			name:    "range inverted",
			recipe:  "version: v1\nname: x\naxes:\n  age: {kind: range}\ndefaults:\n  coordinates: {age: 17-13}\n",
			wantErr: "lower bound is above its upper bound",
		},
		{
			name:    "geo labels",
			recipe:  "version: v1\nname: x\naxes:\n  market:\n    kind: geo\n    labels: {no: Norge}\n",
			wantErr: "axes.market.labels: a geo axis takes no labels",
		},
		{
			name:    "declared place takes a built-in id",
			recipe:  "version: v1\nname: x\nplaces:\n  no-03: {within: no}\n",
			wantErr: "places.no-03: no-03 is a built-in place (Oslo, Norway)",
		},
		{
			name:    "declared place within an unknown place",
			recipe:  "version: v1\nname: x\nplaces:\n  oslo: {within: no-3}\n",
			wantErr: `places.oslo.within: "no-3" is not a place.`,
		},
		{
			name:    "declared place takes an unknown key",
			recipe:  "version: v1\nname: x\nplaces:\n  oslo: {within: no-03, parent: no}\n",
			wantErr: `a place takes no key "parent"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p KapiProject
			err := yaml.Unmarshal([]byte(tt.recipe), &p)
			if err == nil {
				err = p.Validate()
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAxes_ShapeFileIsCheckedAtLoad(t *testing.T) {
	dir := t.TempDir()
	recipe := "version: v1\nname: x\nplaces:\n  bergen: {within: no-46, shape: places/bergen.geojson}\n"
	path := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(path, []byte(recipe), 0o644))
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "places.bergen.shape: places/bergen.geojson: no such file beside the recipe")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "places"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "places", "bergen.geojson"), []byte(`{"type":"Polygon","coordinates":[]}`), 0o644))
	p, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "places/bergen.geojson", p.Places["bergen"].Shape)
}

// A target language may name a subdivision with the -u-sd- key, and the
// recipe loads it as written.
func TestAxes_SubdivisionLocaleLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(path, []byte("version: v1\nname: x\ndefaults:\n  source_language: en\n  target_languages: [nb-NO-u-sd-no03]\n"), 0o644))
	p, err := Load(path)
	require.NoError(t, err)
	got := p.Defaults.TargetLanguages
	require.Len(t, got, 1)
	assert.Equal(t, "no-03", place.OfLocale(string(got[0])))
	assert.Equal(t, []model.LocaleID{"nb-NO-u-sd-no03"}, (&ContentItem{}).ResolvedTargetLanguages(nil, p.Defaults))
}

func TestAxes_SetFieldValidatesTheValue(t *testing.T) {
	p := loadRecipe(t, axesRecipe)
	_, err := SetField(p, "defaults.coordinates.audience", json.RawMessage(`"children"`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"children" is not one of its values`)

	changed, err := SetField(p, "defaults.coordinates.audience", json.RawMessage(`"specialists"`))
	require.NoError(t, err)
	assert.True(t, changed)

	changed, err = SetField(p, "defaults.coordinates.market", json.RawMessage(`"oslo"`))
	require.NoError(t, err, "a declared place is a geo value")
	assert.True(t, changed)
	_, err = SetField(p, "defaults.coordinates.market", json.RawMessage(`"bergen"`))
	assert.ErrorContains(t, err, `"bergen" is not a place`)
}

func TestAxes_RoundTrip(t *testing.T) {
	p := loadRecipe(t, axesRecipe)
	out, err := yaml.Marshal(p)
	require.NoError(t, err)
	var back KapiProject
	require.NoError(t, yaml.Unmarshal(out, &back))
	require.NoError(t, back.Validate())
	assert.Equal(t, p.Axes, back.Axes)
	assert.Equal(t, p.Places, back.Places)
	assert.Equal(t, p.Collections[0].Content[0].Coordinates, back.Collections[0].Content[0].Coordinates)
}
