package place_test

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/place"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubdivisionData holds the generated file to its shape: every line
// parses, ids are unique and well formed, every parent is a country or
// another subdivision, and no id collides with a region or a country.
func TestSubdivisionData(t *testing.T) {
	f, err := os.Open("subdivisions.tsv")
	require.NoError(t, err)
	defer f.Close()
	g := place.Builtin()
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	rows := 0
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		require.Len(t, fields, 3, line)
		id, parent, name := fields[0], fields[1], fields[2]
		rows++
		assert.Regexp(t, `^[a-z]{2}-[a-z0-9]{1,4}$`, id)
		assert.False(t, seen[id], "%s listed twice", id)
		seen[id] = true
		assert.NotEmpty(t, name, id)
		p, ok := g.Place(parent)
		require.True(t, ok, "%s sits within %s, which is not a place", id, parent)
		assert.Contains(t, []place.Type{place.TypeCountry, place.TypeSubdivision}, p.Type, id)
		assert.True(t, strings.HasPrefix(id, parent[:2]), "%s sits in another country's %s", id, parent)
		got, ok := g.Place(id)
		require.True(t, ok)
		assert.Equal(t, place.TypeSubdivision, got.Type, "%s collides with a region or a country", id)
	}
	require.NoError(t, sc.Err())
	assert.Greater(t, rows, 4000, "the file holds every ISO 3166-2 subdivision")
}

// TestBuiltins_FormATree checks that every built-in place but world sits
// within exactly one place and reaches world, so Path, Ancestors and the map
// read one chain per place.
func TestBuiltins_FormATree(t *testing.T) {
	g := place.Builtin()
	all := g.Search("", 0)
	assert.Empty(t, all, "an empty query finds nothing")
	var walk func(id string, depth int)
	count := 0
	walk = func(id string, depth int) {
		count++
		require.Less(t, depth, 10, id)
		p, ok := g.Place(id)
		require.True(t, ok, id)
		if id == place.World {
			assert.Empty(t, p.Within)
		} else {
			assert.Len(t, p.Within, 1, id)
			assert.Empty(t, p.Of, id)
			path := g.Path(id)
			assert.Equal(t, place.World, path[0], id)
			assert.Equal(t, id, path[len(path)-1], id)
		}
		for _, c := range g.Children(id) {
			walk(c, depth+1)
		}
	}
	walk(place.World, 0)
	assert.Greater(t, count, 5000, "every built-in place is reached from world")
}

func TestBuiltins_Countries(t *testing.T) {
	g := place.Builtin()
	var countries, regions []string
	var walk func(id string)
	walk = func(id string) {
		p, _ := g.Place(id)
		switch p.Type {
		case place.TypeCountry:
			countries = append(countries, id)
			require.NotNil(t, p.Centroid, id)
			assert.True(t, p.Centroid.Valid(), id)
			assert.Regexp(t, `^[a-z]{2}$`, id)
			return
		case place.TypeRegion:
			regions = append(regions, id)
		}
		for _, c := range g.Children(id) {
			walk(c)
		}
	}
	walk(place.World)
	assert.Len(t, countries, 249, "ISO 3166-1 assigns 249 alpha-2 codes")
	assert.Len(t, regions, 29, "UN M49 names 29 regions, subregions and intermediate regions")
	for _, r := range regions {
		held := false
		for _, c := range countries {
			held = held || g.Contains(r, c)
		}
		assert.True(t, held, "region %s holds no country, so it is a dead end on a map", r)
	}
}

// TestBuiltins_Norway pins the counties in force from 2024, which the
// amendments add over the CLDR release.
func TestBuiltins_Norway(t *testing.T) {
	g := place.Builtin()
	counties := g.Children("no")
	assert.ElementsMatch(t, []string{
		"no-03", "no-11", "no-15", "no-18", "no-21", "no-22", "no-31", "no-32", "no-33",
		"no-34", "no-39", "no-40", "no-42", "no-46", "no-50", "no-55", "no-56",
	}, counties)
	for _, retired := range []string{"no-30", "no-38", "no-54"} {
		assert.False(t, g.Has(retired), retired)
	}
	assert.Equal(t, "Akershus", g.Name("no-32"))
}

func TestBuiltins_SecondLevels(t *testing.T) {
	g := place.Builtin()
	assert.Equal(t, []string{"world", "europe", "western-europe", "fr", "fr-ara", "fr-01"}, g.Path("fr-01"))
	assert.Equal(t, []string{"world", "europe", "northern-europe", "gb", "gb-sct"}, g.Path("gb-sct"))
	assert.Equal(t, "California", g.Name("us-ca"))
	assert.True(t, g.Contains("europe", "fr-01"))
	assert.False(t, g.Contains("fr-01", "fr-ara"))
}

func TestNilGazetteerIsBuiltin(t *testing.T) {
	var g *place.Gazetteer
	assert.True(t, g.Has("no-03"))
	assert.True(t, g.Contains("no", "no-03"))
	assert.Equal(t, []string{"world", "europe", "northern-europe", "no"}, g.Path("no"))
	assert.Empty(t, g.Declared())
	b, err := place.New(nil)
	require.NoError(t, err)
	assert.Same(t, place.Builtin(), b)
}
