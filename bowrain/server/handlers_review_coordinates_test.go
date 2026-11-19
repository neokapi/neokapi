package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// confirmAs confirms the project's coordinates as one user.
func confirmAs(t *testing.T, s *Server, wsID, projID, user string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("rc", projID)
	c.Set("workspace_id", wsID)
	c.Set("user_id", user)
	c.Set("project_permissions", platauth.PermAll)
	require.NoError(t, s.HandleConfirmReviewCoordinates(c))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

// coordinatesProject is the container project with its guide in a docs
// collection under the product neokapi.
func coordinatesProject(t *testing.T, s *Server, wsID string) string {
	t.Helper()
	ctx := t.Context()
	projID, _ := containerProject(t, s, wsID)
	docs := &platstore.Collection{
		ID: "c-docs", ProjectID: projID, Name: "neokapi-docs",
		Context: map[string]string{"product": "neokapi", "channel": "docs"},
	}
	require.NoError(t, s.ContentStore.CreateCollection(ctx, docs))
	require.NoError(t, s.ContentStore.StoreItem(ctx, projID, "main", &platstore.Item{Name: "guide.md", Format: "txt", ItemType: "file", CollectionID: docs.ID}))
	return projID
}

func TestReviewCoordinates_ConfirmRecordsTheMap(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID := coordinatesProject(t, s, wsID)

	confirmAs(t, s, wsID, projID, "u-owner")
	proj, err := s.ContentStore.GetProject(t.Context(), projID)
	require.NoError(t, err)
	snap, ok := coordinatesSnapshotOf(proj)
	require.True(t, ok, "confirming records the map")
	assert.Equal(t, []string{"fr"}, snap.TargetLanguages)
	require.Len(t, snap.Points, 1)
	assert.Equal(t, "neokapi-docs", snap.Points[0].Collection)
	assert.Equal(t, map[string]string{"product": "neokapi", "channel": "docs"}, snap.Points[0].Coordinates)

	plan := readPlan(t, s, wsID, projID)
	assert.True(t, plan.Coordinates.Confirmed)
	assert.Zero(t, plan.Coordinates.Changed)
	assert.NotEqual(t, planCoordinates, plan.Next)
}

func TestReviewCoordinates_AChangeReopensTheStepForEveryone(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID := coordinatesProject(t, s, wsID)
	confirmAs(t, s, wsID, projID, "u-owner")

	require.NoError(t, s.ContentStore.CreateCollection(t.Context(), &platstore.Collection{
		ID: "c-mobile", ProjectID: projID, Name: "neokapi-mobile",
		Context: map[string]string{"product": "neokapi", "channel": "mobile"},
	}))
	plan := readPlan(t, s, wsID, projID)
	require.Equal(t, 1, plan.Coordinates.Changed)
	assert.Equal(t, changePointAdded, plan.Coordinates.Changes[0].Kind)
	assert.Equal(t, "neokapi-mobile", plan.Coordinates.Changes[0].Collection)
	assert.Equal(t, planCoordinates, plan.Next, "a change puts the coordinates first again")

	confirmAs(t, s, wsID, projID, "u-second")
	plan = readPlan(t, s, wsID, projID)
	assert.Zero(t, plan.Coordinates.Changed, "one confirmation settles the step for the reviewer too")
	assert.Equal(t, "u-second", plan.Coordinates.ConfirmedBy)
	assert.NotEqual(t, planCoordinates, plan.Next)
}

func TestReviewCoordinates_AnOlderConfirmationHasNoChanges(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID := coordinatesProject(t, s, wsID)
	require.NoError(t, s.updatePlanProperties(t.Context(), projID, map[string]string{
		propCoordinatesConfirmedBy: "u-owner",
		propCoordinatesConfirmedAt: "2026-11-01T09:00:00Z",
	}))

	plan := readPlan(t, s, wsID, projID)
	assert.True(t, plan.Coordinates.Confirmed)
	assert.Empty(t, plan.Coordinates.Changes, "with no recorded map, nothing reads as new")
	assert.NotEqual(t, planCoordinates, plan.Next)
}

func TestReviewCoordinates_ListsVoiceAndLanguageChanges(t *testing.T) {
	snap := coordinatesSnapshot{
		TargetLanguages: []string{"fr", "de"},
		Points: []snapshotPoint{
			{Collection: "docs", Coordinates: map[string]string{"product": "p", "channel": "docs"}, VoiceID: "v1", Voice: "Docs"},
			{Collection: "gone", Coordinates: map[string]string{"product": "p", "channel": "old"}},
		},
	}
	cur := planCoordinatesDTO{
		TargetLanguages: []string{"fr", "nb"},
		Points: []planPoint{
			{Collection: "docs", Coordinates: map[string]string{"product": "p", "channel": "docs"}, VoiceID: "v2", Voice: "Product"},
		},
	}
	kinds := []string{}
	for _, ch := range coordinateChanges(snap, cur) {
		kinds = append(kinds, ch.Kind+":"+ch.Collection+ch.Locale)
	}
	assert.Equal(t, []string{
		"voice_changed:docs", "point_removed:gone", "language_added:nb", "language_removed:de",
	}, kinds)

	renamed := cur
	renamed.Points = []planPoint{{Collection: "docs", Coordinates: map[string]string{"product": "p", "channel": "docs"}, VoiceID: "v1", Voice: "Docs, renamed"}}
	renamed.TargetLanguages = []string{"fr", "de"}
	snap.Points = snap.Points[:1]
	assert.Empty(t, coordinateChanges(snap, renamed), "renaming a voice profile is not a change of voice")
}

func TestReviewCoordinates_CellsCountBlocksAndCoverage(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID := coordinatesProject(t, s, wsID)
	confirmAs(t, s, wsID, projID, "u-owner")
	require.NoError(t, s.ContentStore.CreateCollection(t.Context(), &platstore.Collection{
		ID: "c-mobile", ProjectID: projID, Name: "neokapi-mobile",
		Context: map[string]string{"product": "neokapi", "channel": "mobile"},
	}))

	rec := deckCall(t, s, s.HandleGetReviewCoordinates, wsID, projID, "", "u-reviewer", testReviewer, nil, url.Values{})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	out := decodeJSON[reviewCoordinatesResponse](t, rec)
	require.Len(t, out.Cells, 2)
	docs, mobile := out.Cells[0], out.Cells[1]
	assert.Equal(t, "docs", docs.Channel)
	assert.Equal(t, 7, docs.Blocks, "the guide's blocks")
	assert.InDelta(t, 3.0/7, docs.Coverage["fr"], 0.001, "the three headings are established")
	assert.Empty(t, docs.Changes)
	assert.Equal(t, "mobile", mobile.Channel)
	assert.Zero(t, mobile.Blocks)
	assert.Equal(t, []string{changePointAdded}, mobile.Changes)
	assert.Equal(t, 1, out.Coordinates.Changed)
}
