package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mapPoint returns the row for one point ref, failing the test when it is absent.
func mapPoint(t *testing.T, res *ProjectPointsResult, ref string) ProjectPointDTO {
	t.Helper()
	for _, p := range res.Points {
		if p.Ref == ref {
			return p
		}
	}
	require.Failf(t, "point missing", "no point %q", ref)
	return ProjectPointDTO{}
}

func TestProjectPointsListsTheDeclaredCrossProduct(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectPoints(tab.ID)
	require.NoError(t, err)

	refs := make([]string, 0, len(res.Points))
	for _, p := range res.Points {
		refs = append(refs, p.Ref)
	}
	// The project's own point leads, then each profile's channels in name order.
	assert.Equal(t, []string{"", "campaign/promo", "support/docs"}, refs)
	assert.True(t, res.Points[0].Default)
}

func TestProjectPointsPlacesCollectionsWhereTheyAreGoverned(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectPoints(tab.ID)
	require.NoError(t, err)

	assert.Equal(t, []string{"Docs"}, mapPoint(t, res, "support/docs").Collections)

	// Promo names campaign/promo, whose window has closed, so it is governed at
	// the default point and is listed there rather than where it was written.
	def := mapPoint(t, res, "")
	assert.Contains(t, def.Collections, "App")
	assert.Contains(t, def.Collections, "Promo")
	assert.Empty(t, mapPoint(t, res, "campaign/promo").Collections)
}

func TestProjectPointsNamesTheVoiceInForce(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectPoints(tab.ID)
	require.NoError(t, err)

	assert.Equal(t, "Northsea", mapPoint(t, res, "").Voice)
	assert.Equal(t, "Northsea Support", mapPoint(t, res, "support/docs").Voice)

	// A point whose binding the instant excluded reports the fall-through and
	// the voice that governs in its place.
	campaign := mapPoint(t, res, "campaign/promo")
	require.NotNil(t, campaign.Fallback)
	assert.True(t, campaign.Fallback.Expired)
	assert.Equal(t, "Northsea", campaign.Voice)
	assert.Equal(t, project.DefaultVoiceField, campaign.VoiceField)
}

func TestProjectPointsCarriesTheCoordinates(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectPoints(tab.ID)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{project.BrandAxis: "northsea"},
		mapPoint(t, res, "").Coordinates)
	assert.Equal(t, map[string]string{
		project.BrandAxis:   "northsea",
		project.ProductAxis: "support",
		project.ChannelAxis: "docs",
	}, mapPoint(t, res, "support/docs").Coordinates)
}

func TestProjectPointsRejectsAnUnknownTab(t *testing.T) {
	app := NewApp()

	_, err := app.ProjectPoints("nope")
	assert.Error(t, err)
}
