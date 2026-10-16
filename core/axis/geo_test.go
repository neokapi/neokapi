package axis_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeoTable_Complete holds the built-in geo table to ISO 3166-1: every
// assigned alpha-2 code once, in lower case and in order, each with a name, a
// centroid on the globe, and a region that resolves to World.
func TestGeoTable_Complete(t *testing.T) {
	places := axis.Places()
	assert.Len(t, places, 249, "ISO 3166-1 assigns 249 alpha-2 codes")

	seen := map[string]bool{}
	prev := ""
	for _, p := range places {
		assert.Len(t, p.Code, 2, p.Code)
		assert.Regexp(t, `^[a-z]{2}$`, p.Code)
		assert.False(t, seen[p.Code], "%s listed twice", p.Code)
		seen[p.Code] = true
		assert.Less(t, prev, p.Code, "codes are in order")
		prev = p.Code
		assert.NotEmpty(t, p.Name, p.Code)
		assert.True(t, p.Lat >= -90 && p.Lat <= 90, "%s latitude %v", p.Code, p.Lat)
		assert.True(t, p.Lon >= -180 && p.Lon <= 180, "%s longitude %v", p.Code, p.Lon)
		assert.True(t, reachesWorld(p.Region), "%s region %q does not reach world", p.Code, p.Region)
	}
	for _, code := range []string{"no", "se", "dk", "fi", "is", "de", "fr", "us", "ca", "br", "jp", "cn", "in", "za", "au", "nz"} {
		assert.True(t, seen[code], code)
	}
}

func TestGeoTable_RegionsResolve(t *testing.T) {
	slugs := map[string]bool{}
	codes := map[string]bool{}
	for _, r := range axis.Regions() {
		assert.Regexp(t, `^[a-z][a-z-]*$`, r.Slug)
		assert.Regexp(t, `^[0-9]{3}$`, r.Code)
		assert.False(t, slugs[r.Slug], r.Slug)
		assert.False(t, codes[r.Code], r.Code)
		slugs[r.Slug], codes[r.Code] = true, true
		assert.True(t, reachesWorld(r.Parent), "%s parent %q", r.Slug, r.Parent)
		assert.NotEmpty(t, r.Name)
	}
	// Every region holds at least one country, so none is a dead end on a map.
	for slug := range slugs {
		held := false
		for _, p := range axis.Places() {
			if (axis.Space{"m": {Kind: axis.KindGeo}}).Contains("m", slug, p.Code) {
				held = true
				break
			}
		}
		assert.True(t, held, "region %s holds no country", slug)
	}
}

func reachesWorld(v string) bool {
	for range 8 {
		if v == axis.World {
			return true
		}
		if v == "" {
			return false
		}
		v = axis.GeoParent(v)
	}
	return false
}

func TestRegionOfLocale(t *testing.T) {
	tests := map[string]string{
		"fr-CA":      "ca",
		"nb-NO":      "no",
		"pt_BR":      "br",
		"zh-Hant-TW": "tw",
		"es-419":     "latin-america-and-the-caribbean",
		"en":         "",
		"de":         "",
		"en-XX":      "",
		"sr-Latn":    "",
		"":           "",
	}
	for locale, want := range tests {
		assert.Equal(t, want, axis.RegionOfLocale(locale), locale)
	}
}

func TestPlaceOf(t *testing.T) {
	p, ok := axis.PlaceOf("NO")
	require.True(t, ok)
	assert.Equal(t, "Norway", p.Name)
	assert.Equal(t, "northern-europe", p.Region)
	_, ok = axis.PlaceOf("xx")
	assert.False(t, ok)
	r, ok := axis.RegionOf("northern-europe")
	require.True(t, ok)
	assert.Equal(t, "europe", r.Parent)
	assert.Equal(t, "World", axis.GeoLabel("world"))
}
