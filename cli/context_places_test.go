package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// writePlacesProject is a project that declares Oslo within its county and
// Grünerløkka within Oslo.
func writePlacesProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(`version: v1
name: places
defaults:
  source_language: en
places:
  oslo:
    name: Oslo
    type: locality
    within: no-03
  grunerlokka:
    name: Grünerløkka
    type: neighbourhood
    within: oslo
axes:
  market: {kind: geo}
`), 0o644))
	return filepath.Join(root, "kapi.yaml")
}

func TestContextPlaces_LooksUpByQuery(t *testing.T) {
	recipe := writePlacesProject(t)
	a := &App{}

	out := runContext(t, a, "places", "grunerlokka", "-p", recipe)
	assert.Equal(t, "grunerlokka  Grünerløkka  neighbourhood  world › europe › northern-europe › no › no-03 › oslo › grunerlokka\n", out,
		"one line per place: id, name, type and path")

	out = runContext(t, a, "places", "Grünerløkka", "-p", recipe)
	assert.Contains(t, out, "grunerlokka", "names match without case or accents")
}

func TestContextPlaces_ListsWithinAPlace(t *testing.T) {
	recipe := writePlacesProject(t)
	a := &App{}

	out := runContext(t, a, "places", "--within", "no", "-p", recipe)
	assert.Contains(t, out, "no-32  Akershus")
	assert.Contains(t, out, "world › europe › northern-europe › no › no-03\n")
	assert.NotContains(t, out, "oslo ", "a county's own places sit below the county")

	out = runContext(t, a, "places", "--within", "no-03", "-p", recipe)
	assert.Contains(t, out, "oslo  Oslo  locality")
}

func TestContextPlaces_ListsTheDeclaredPlaces(t *testing.T) {
	recipe := writePlacesProject(t)
	a := &App{}

	out := runContext(t, a, "places", "-p", recipe, "--json")
	var res host.ContextPlaces
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.Declared)
	require.Len(t, res.Places, 2)
	assert.Equal(t, "grunerlokka", res.Places[0].ID)
	assert.Equal(t, "oslo", res.Places[1].ID)
	assert.Equal(t, []string{"no-03"}, res.Places[1].Within)
	assert.True(t, res.Places[1].Declared)
}

func TestContextPlaces_RefusesAnUnknownPlace(t *testing.T) {
	recipe := writePlacesProject(t)
	_, err := runContextE(t, &App{}, "places", "--within", "no03", "-p", recipe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `--within: "no03" is not a place. Write it with a hyphen: no-03`)
}

func TestContextPlaces_HintsLeadToTheLookup(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: x\naxes:\n  market: {kind: geo}\ndefaults:\n  coordinates: {market: lofoten}\n"), 0o644))
	_, err := runContextE(t, &App{}, "places", "-p", recipe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Look a place up with `kapi context places lofoten`")
}
