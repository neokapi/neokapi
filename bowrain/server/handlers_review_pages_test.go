package server

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readPages(t *testing.T, s *Server, wsID, projID string) reviewDeckPagesResponse {
	t.Helper()
	rec := deckCall(t, s, s.HandleGetReviewDeckPages, wsID, projID, "", "u-reviewer", testReviewer, nil, url.Values{"locale": {"fr"}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSON[reviewDeckPagesResponse](t, rec)
}

func TestReviewDeckPages_ComponentsAndDocuments(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := containerProject(t, s, wsID)

	got := readPages(t, s, wsID, projID)
	type row struct {
		Kind, Title string
		Waiting     int
		First       string
	}
	var rows []row
	for _, p := range got.Pages {
		rows = append(rows, row{p.Kind, p.Title, p.Waiting, p.FirstBlockID})
	}
	assert.Equal(t, []row{
		{"component", "Header", 1, ids["Bowrain"]},
		{"component", "OfflineLaunch", 3, ids["Can't reach your server"]},
		{"component", "Footer", 1, ids["Made in Oslo"]},
		{"document", "guide.md", 4, ids["Intro text."]},
	}, rows, "items in name order, a page by its first waiting block; approved headings are not counted")
	assert.Equal(t, 9, got.Waiting)
	assert.False(t, got.More)
}

func TestReviewContainer_NamesTheNextWaitingBlock(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := containerProject(t, s, wsID)

	install := readContainer(t, s, wsID, projID, ids["Restart."])
	assert.Equal(t, ids["Open the app."], install.NextWaiting, "past the section, skipping the approved heading")

	use := readContainer(t, s, wsID, projID, ids["Open the app."])
	assert.Empty(t, use.NextWaiting, "nothing waits after the last section")
}

func TestReviewDeckPages_NeedsTheReviewPermissionForTheLanguage(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	rec := deckCall(t, s, s.HandleGetReviewDeckPages, wsID, projID, "", "u-reviewer", testReviewer, []string{"de"}, url.Values{"locale": {"fr"}})
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestReviewDeckPages_SampleKeepsTheFirstPageOfEachCollection(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)

	all := readPages(t, s, wsID, projID)
	first := map[string]string{}
	var want []string
	for _, p := range all.Pages {
		if _, ok := first[p.CollectionID]; !ok && p.Findings == 0 {
			first[p.CollectionID] = p.Title
			want = append(want, p.Title)
		}
	}
	rec := deckCall(t, s, s.HandleGetReviewDeckPages, wsID, projID, "", "u-reviewer", testReviewer, nil,
		url.Values{"locale": {"fr"}, "sample": {"true"}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got []string
	for _, p := range decodeJSON[reviewDeckPagesResponse](t, rec).Pages {
		got = append(got, p.Title)
	}
	assert.Equal(t, want, got, "fewer than ten pages per collection: the sample is each collection's first")
}
