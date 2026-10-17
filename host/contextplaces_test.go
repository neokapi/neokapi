package host

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func placeIDs(res ContextPlaces) []string {
	out := []string{}
	for _, p := range res.Places {
		out = append(out, p.ID)
	}
	return out
}

func TestContextPlaces(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-places")
	recipe := recipeOf(root)

	declared, err := app.ContextPlaces(t.Context(), ContextPlacesRequest{Project: recipe})
	require.NoError(t, err)
	assert.True(t, declared.Declared)
	assert.Equal(t, []string{"nordics", "oslo"}, placeIDs(declared), "with neither a query nor --within, the declared places")
	assert.Equal(t, []string{"world", "europe", "northern-europe", "no", "no-03", "oslo"}, declared.Places[1].Path)

	query, err := app.ContextPlaces(t.Context(), ContextPlacesRequest{Project: recipe, Query: "oslo"})
	require.NoError(t, err)
	require.NotEmpty(t, query.Places)
	assert.Equal(t, "oslo", query.Places[0].ID, "an id match first")
	assert.True(t, query.Places[0].Declared)
	assert.Contains(t, placeIDs(query), "no-03", "then a name match")

	builtin, err := app.ContextPlaces(t.Context(), ContextPlacesRequest{Query: "oslo"})
	require.NoError(t, err)
	assert.NotContains(t, placeIDs(builtin), "oslo", "without a project, the built-in places alone")

	within, err := app.ContextPlaces(t.Context(), ContextPlacesRequest{Project: recipe, Within: "no-03"})
	require.NoError(t, err)
	assert.Equal(t, []string{"oslo"}, placeIDs(within))
	counties, err := app.ContextPlaces(t.Context(), ContextPlacesRequest{Project: recipe, Within: "no"})
	require.NoError(t, err)
	assert.Contains(t, placeIDs(counties), "no-32")
	assert.NotContains(t, placeIDs(counties), "oslo", "a child, never a grandchild")

	narrowed, err := app.ContextPlaces(t.Context(), ContextPlacesRequest{Project: recipe, Query: "a", Within: "no"})
	require.NoError(t, err)
	for _, p := range narrowed.Places {
		assert.Contains(t, p.Path, "no", p.ID)
		assert.NotEqual(t, "no", p.ID)
	}

	wide, err := app.ContextPlaces(t.Context(), ContextPlacesRequest{Query: "a"})
	require.NoError(t, err)
	assert.Len(t, wide.Places, contextPlacesLimit)
	assert.Greater(t, wide.Total, contextPlacesLimit)

	_, err = app.ContextPlaces(t.Context(), ContextPlacesRequest{Project: recipe, Within: "NO"})
	assert.ErrorContains(t, err, `--within: "NO" is not a place. Write it in lower case: no`)
}

func TestContextPlaces_FormatText(t *testing.T) {
	res := ContextPlaces{Places: []ContextPlace{
		{ID: "oslo", Name: "Oslo", Type: "locality", Path: []string{"world", "europe", "northern-europe", "no", "no-03", "oslo"}, Declared: true},
		{ID: "grunerlokka", Name: "Grünerløkka", Type: "neighbourhood", Path: []string{"world", "europe", "northern-europe", "no", "no-03", "oslo", "grunerlokka"}},
	}, Total: 2}
	var out bytes.Buffer
	require.NoError(t, res.FormatText(&out))
	assert.Equal(t, "oslo         Oslo         locality       world › europe › northern-europe › no › no-03 › oslo\n"+
		"grunerlokka  Grünerløkka  neighbourhood  world › europe › northern-europe › no › no-03 › oslo › grunerlokka\n", out.String())

	for _, tt := range []struct {
		res  ContextPlaces
		want string
	}{
		{ContextPlaces{Declared: true}, "This project declares no places. Look one up with `kapi context places <query>`.\n"},
		{ContextPlaces{Query: "atlantis"}, "No place matches \"atlantis\".\n"},
		{ContextPlaces{Within: "no-03"}, "No place sits directly within no-03.\n"},
		{ContextPlaces{Query: "a", Places: []ContextPlace{{ID: "ad", Name: "Andorra", Type: "country", Path: []string{"world", "europe", "southern-europe", "ad"}}}, Total: 40}, "ad  Andorra  country  world › europe › southern-europe › ad\n1 of 40 matches; narrow the query or add --within.\n"},
	} {
		out.Reset()
		require.NoError(t, tt.res.FormatText(&out))
		assert.Equal(t, tt.want, out.String())
	}
}
