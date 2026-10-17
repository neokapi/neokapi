package place_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/neokapi/neokapi/core/place"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNew_Refusals(t *testing.T) {
	within := func(ids ...string) place.Place { return place.Place{Name: "X", Within: ids} }
	tests := []struct {
		name     string
		declared map[string]place.Place
		wantErr  string
	}{
		{"id not a slug", map[string]place.Place{"Oslo": within("no-03")}, `places: place id "Oslo" is not a slug`},
		{"built-in id", map[string]place.Place{"no-03": within("no")}, "places.no-03: no-03 is a built-in place (Oslo, Norway); a declared place takes an id of its own"},
		{"built-in country id", map[string]place.Place{"no": within("europe")}, "places.no: no is a built-in place (Norway)"},
		{"both within and of", map[string]place.Place{"x": {Within: []string{"no"}, Of: []string{"se"}}}, "places.x: a place sits within places or is made of places, never both"},
		{"neither within nor of", map[string]place.Place{"x": {Name: "X"}}, "places.x: a place names the places it sits within (`within:`) or is made of (`of:`)"},
		{"unknown within", map[string]place.Place{"x": within("atlantis")}, `places.x.within: "atlantis" is not a place.`},
		{"unknown within teaches the lookup", map[string]place.Place{"x": within("atlantis")}, "kapi context places atlantis"},
		{"upper-case within", map[string]place.Place{"x": within("NO-03")}, "Write it in lower case: no-03"},
		{"within without hyphen", map[string]place.Place{"x": within("no03")}, "Write it with a hyphen: no-03"},
		{"unknown of member", map[string]place.Place{"x": {Of: []string{"no", "narnia"}}}, `places.x.of: "narnia" is not a place.`},
		{"within itself", map[string]place.Place{"x": within("x")}, "places.x.within: a place cannot sit within or be made of itself"},
		{"named twice", map[string]place.Place{"x": {Of: []string{"no", "no"}}}, `places.x.of: "no" is named twice`},
		{"cycle", map[string]place.Place{"a": within("b"), "b": within("c"), "c": within("a")}, "places.a: the place sits within itself through a › b › c › a"},
		{"cycle through of", map[string]place.Place{"a": {Of: []string{"b", "no"}}, "b": within("a")}, "sits within itself"},
		{"built-in type", map[string]place.Place{"x": {Within: []string{"no"}, Type: place.TypeCountry}}, `places.x.type: "country" is not a type a declared place takes. Use one of subdivision, territory, metro, locality, district, neighbourhood, postal-area, area`},
		{"unknown type", map[string]place.Place{"x": {Within: []string{"no"}, Type: "city"}}, `places.x.type: "city" is not a type`},
		{"names key not a locale", map[string]place.Place{"x": {Within: []string{"no"}, Names: map[string]string{"norwegian!": "Oslo"}}}, `places.x.names: "norwegian!" is not a locale`},
		{"names key twice", map[string]place.Place{"x": {Within: []string{"no"}, Names: map[string]string{"nb": "A", "NB": "B"}}}, "name the same locale nb"},
		{"empty name in names", map[string]place.Place{"x": {Within: []string{"no"}, Names: map[string]string{"nb": " "}}}, "places.x.names.nb: an empty name"},
		{"ref without scheme", map[string]place.Place{"x": {Within: []string{"no"}, Refs: []string{"Q585"}}}, `places.x.refs: "Q585" is not a reference. Write scheme:value`},
		{"ref unknown scheme", map[string]place.Place{"x": {Within: []string{"no"}, Refs: []string{"wiki:Q585"}}}, `"wiki:Q585" is not a reference`},
		{"wikidata malformed", map[string]place.Place{"x": {Within: []string{"no"}, Refs: []string{"wikidata:585"}}}, `"wikidata:585" does not follow the wikidata scheme (write it as wikidata:Q585)`},
		{"geonames malformed", map[string]place.Place{"x": {Within: []string{"no"}, Refs: []string{"geonames:oslo"}}}, "does not follow the geonames scheme"},
		{"nuts malformed", map[string]place.Place{"x": {Within: []string{"no"}, Refs: []string{"nuts:no081"}}}, "does not follow the nuts scheme"},
		{"osm malformed", map[string]place.Place{"x": {Within: []string{"no"}, Refs: []string{"osm:406091"}}}, `"osm:406091" does not follow the osm scheme`},
		{"wof malformed", map[string]place.Place{"x": {Within: []string{"no"}, Refs: []string{"wof:x1"}}}, "does not follow the wof scheme"},
		{"ref twice", map[string]place.Place{"x": {Within: []string{"no"}, Refs: []string{"wikidata:Q585", "wikidata:Q585"}}}, "is listed twice"},
		{"latitude off the globe", map[string]place.Place{"x": {Within: []string{"no"}, Centroid: &place.LatLon{Lat: 91, Lon: 10}}}, "places.x.centroid: [91, 10] is not on the globe"},
		{"longitude off the globe", map[string]place.Place{"x": {Within: []string{"no"}, Centroid: &place.LatLon{Lat: 59, Lon: 181}}}, "is not on the globe"},
		{"NaN centroid", map[string]place.Place{"x": {Within: []string{"no"}, Centroid: &place.LatLon{Lat: math.NaN(), Lon: 0}}}, "is not on the globe"},
		{"absolute shape", map[string]place.Place{"x": {Within: []string{"no"}, Shape: "/tmp/x.geojson"}}, "places.x.shape: \"/tmp/x.geojson\": write the path relative to the project"},
		{"shape leaving the project", map[string]place.Place{"x": {Within: []string{"no"}, Shape: "../x.geojson"}}, "the path leaves the project"},
		{"backslash shape", map[string]place.Place{"x": {Within: []string{"no"}, Shape: `places\x.geojson`}}, "forward slashes"},
		{"unclean shape", map[string]place.Place{"x": {Within: []string{"no"}, Shape: "places/./x.geojson"}}, "write the path as places/x.geojson"},
		{"shape not GeoJSON", map[string]place.Place{"x": {Within: []string{"no"}, Shape: "places/x.kml"}}, "a shape is a GeoJSON file ending .geojson or .json"},
		{"id differs from key", map[string]place.Place{"x": {ID: "y", Within: []string{"no"}}}, `places.x: the place's id "y" differs from its key`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := place.New(tt.declared)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNew_Defaults(t *testing.T) {
	g, err := place.New(map[string]place.Place{
		"nordics": {Name: "Nordics", Of: []string{"no", "se", "dk", "fi", "is"}},
		"oslo": {
			Name: "Oslo", Within: []string{"no-03"}, Names: map[string]string{"NB": "Oslo", "se": "Oslo"},
			Refs:     []string{"wikidata:Q585", "geonames:3143244", "nuts:NO081", "osm:relation/406091", "wof:101752073"},
			Centroid: &place.LatLon{Lat: 59.91, Lon: 10.75}, Shape: "places/oslo.geojson",
		},
		"campus": {Within: []string{"oslo"}, Type: place.TypeDistrict},
	})
	require.NoError(t, err)

	nordics, ok := g.Place("nordics")
	require.True(t, ok)
	assert.Equal(t, place.TypeTerritory, nordics.Type, "a place made of places is a territory unless typed")
	assert.True(t, nordics.Declared)
	oslo, _ := g.Place("oslo")
	assert.Equal(t, place.TypeArea, oslo.Type, "a place within places is an area unless typed")
	assert.Equal(t, map[string]string{"nb": "Oslo", "se": "Oslo"}, oslo.Names, "names keys are canonical locales")
	campus, _ := g.Place("campus")
	assert.Equal(t, "campus", campus.Name, "a place with no name shows its id")

	var ids []string
	for _, p := range g.Declared() {
		ids = append(ids, p.ID)
	}
	assert.Equal(t, []string{"campus", "nordics", "oslo"}, ids)
}

func TestGazetteer_Ancestors(t *testing.T) {
	_, g := loadFixture(t)
	assert.Equal(t, []string{"no-03", "greater-oslo", "no", "scandinavia", "nordics", "northern-europe", "europe", "world"}, g.Ancestors("oslo"))
	assert.Equal(t, []string{"oslo", "uni-district", "no-03", "greater-oslo", "no", "scandinavia", "nordics", "northern-europe", "europe", "world"}, g.Ancestors("campus"))
	assert.Equal(t, []string{"nordics", "northern-europe", "europe", "world"}, g.Ancestors("scandinavia"))
	assert.Equal(t, []string{"no", "scandinavia", "nordics", "northern-europe", "europe", "world"}, g.Ancestors("greater-oslo"))
	assert.Empty(t, g.Ancestors("world"))
	assert.Nil(t, g.Ancestors("atlantis"))
	for _, id := range []string{"oslo", "campus", "scandinavia", "greater-oslo", "bergen"} {
		for _, a := range g.Ancestors(id) {
			assert.True(t, g.Contains(a, id), "%s lists %s, which does not contain it", id, a)
		}
	}
}

func TestGazetteer_Children(t *testing.T) {
	_, g := loadFixture(t)
	assert.Equal(t, []string{"campus", "grunerlokka"}, g.Children("oslo"), "sorted by name: Blindern campus, Grünerløkka")
	assert.Equal(t, []string{"dk", "fi", "is", "no", "se"}, g.Children("nordics"), "a made-of place's members, by name")
	assert.Contains(t, g.Children("northern-europe"), "nordics", "a made-of place hangs under its Path parent")
	assert.Contains(t, g.Children("no"), "greater-oslo")
	assert.Contains(t, g.Children("no-03"), "oslo")
	assert.Contains(t, g.Children("no-03"), "uni-district")
	assert.Nil(t, g.Children("atlantis"))
}

func TestGazetteer_Describe(t *testing.T) {
	_, g := loadFixture(t)
	assert.Equal(t, "Oslo, Norway", g.Describe("no-03"))
	assert.Equal(t, "Grünerløkka, Norway", g.Describe("grunerlokka"))
	assert.Equal(t, "Norway", g.Describe("no"))
	assert.Equal(t, "Nordics", g.Describe("nordics"))
	assert.Equal(t, "atlantis", g.Describe("atlantis"))
}

func TestOfLocale(t *testing.T) {
	tests := map[string]string{
		"nb-NO-u-sd-no03":     "no-03",
		"nb-no-u-sd-no03":     "no-03",
		"en-US-u-sd-usca":     "us-ca",
		"en-u-sd-gbsct":       "gb-sct",
		"nb-NO-u-sd-no99":     "",
		"nb-NO-u-ca-gregory":  "no",
		"en-u-ca-gregory":     "",
		"fr-CA":               "ca",
		"nb-NO":               "no",
		"pt_BR":               "br",
		"zh-Hant-TW":          "tw",
		"ca-ES-valencia":      "es",
		"es-419":              "latin-america-and-the-caribbean",
		"en-x-ca":             "",
		"nb":                  "",
		"en-XX":               "",
		"sr-Latn":             "",
		"":                    "",
		"de-DE-u-co-phonebk":  "de",
		"de-u-co-phonebk-sd1": "",
	}
	for tag, want := range tests {
		assert.Equal(t, want, place.OfLocale(tag), tag)
	}
}

func TestGazetteer_Search(t *testing.T) {
	_, g := loadFixture(t)
	ids := func(ps []place.Place) []string {
		var out []string
		for _, p := range ps {
			out = append(out, p.ID)
		}
		return out
	}
	got := ids(g.Search("oslo", 0))
	require.NotEmpty(t, got)
	assert.Equal(t, "oslo", got[0], "an id prefix comes first")
	assert.Contains(t, got, "no-03", "then a name prefix")
	assert.Contains(t, got, "greater-oslo", "then a name that holds the query")
	assert.Less(t, indexOf(got, "no-03"), indexOf(got, "greater-oslo"))

	assert.Equal(t, []string{"grunerlokka"}, ids(g.Search("Grünerløkka", 0)), "accents and case are ignored")
	assert.Equal(t, []string{"grunerlokka"}, ids(g.Search("GRUNERLOKKA", 0)))
	assert.Equal(t, []string{"no-03"}, ids(g.Search("no-03", 0)))
	assert.Len(t, g.Search("a", 5), 5, "a limit caps the matches")
	assert.Empty(t, g.Search("  ", 0))

	nb, err := place.New(map[string]place.Place{"gamle-oslo": {Name: "Old Oslo", Within: []string{"no-03"}, Names: map[string]string{"nb": "Gamlebyen"}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"gamle-oslo"}, ids(nb.Search("gamleby", 0)), "a declared place's other names count")

	exactFirst := ids(place.Builtin().Search("no", 3))
	assert.Equal(t, []string{"no", "northern-africa", "northern-america"}, exactFirst, "the exact id first, then id prefixes, wider kinds first")
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}

func TestPlace_YAML(t *testing.T) {
	src := `
oslo:
  name: Oslo
  type: locality
  within: no-03
  centroid: [59.91, 10.75]
  refs: [wikidata:Q585, geonames:3143244]
campus:
  within: [oslo, uni]
nordics:
  name: Nordics
  of: [no, se, dk, fi, is]
`
	var got map[string]place.Place
	require.NoError(t, yaml.Unmarshal([]byte(src), &got))
	assert.Equal(t, []string{"no-03"}, got["oslo"].Within, "a single within is a string")
	assert.Equal(t, &place.LatLon{Lat: 59.91, Lon: 10.75}, got["oslo"].Centroid)
	assert.Equal(t, []string{"oslo", "uni"}, got["campus"].Within)
	assert.Equal(t, []string{"no", "se", "dk", "fi", "is"}, got["nordics"].Of)

	out, err := yaml.Marshal(map[string]place.Place{"oslo": got["oslo"]})
	require.NoError(t, err)
	assert.Equal(t, "oslo:\n    name: Oslo\n    type: locality\n    within: no-03\n    refs:\n        - wikidata:Q585\n        - geonames:3143244\n    centroid: [59.91, 10.75]\n", string(out))
	var back map[string]place.Place
	require.NoError(t, yaml.Unmarshal(out, &back))
	assert.Equal(t, got["oslo"], back["oslo"], "a place round-trips")

	for name, bad := range map[string]string{
		"unknown key":     "x:\n  within: no\n  parent: no\n",
		"centroid of one": "x:\n  within: no\n  centroid: [59]\n",
		"within a map":    "x:\n  within: {a: b}\n",
		"not a mapping":   "x: no\n",
	} {
		t.Run(name, func(t *testing.T) {
			var m map[string]place.Place
			assert.Error(t, yaml.Unmarshal([]byte(bad), &m))
		})
	}
}

func TestLatLon_JSON(t *testing.T) {
	data, err := json.Marshal(place.Place{ID: "oslo", Name: "Oslo", Type: place.TypeLocality, Centroid: &place.LatLon{Lat: 59.91, Lon: 10.75}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"oslo","name":"Oslo","type":"locality","centroid":[59.91,10.75]}`, string(data))
	var p place.Place
	require.NoError(t, json.Unmarshal(data, &p))
	assert.Equal(t, place.LatLon{Lat: 59.91, Lon: 10.75}, *p.Centroid)
	assert.Error(t, json.Unmarshal([]byte(`{"centroid":[1]}`), &p))
}

func TestType_Rank(t *testing.T) {
	order := []place.Type{place.TypeWorld, place.TypeRegion, place.TypeCountry, place.TypeSubdivision, place.TypeTerritory, place.TypeArea, "unknown"}
	for i := 1; i < len(order); i++ {
		assert.Less(t, order[i-1].Rank(), order[i].Rank(), order[i])
	}
}
