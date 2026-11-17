package server

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// establishedFrBlock is a block whose French translation is approved.
func establishedFrBlock(id, source, frTarget string) *model.Block {
	b := pendingFrBlock(id, source, frTarget)
	b.SetEditionStatus(model.Variant("fr"), model.Status(model.TargetStatusEstablished))
	return b
}

// seedCollectionsProject is a French project whose items sit in collections:
// web and docs hold pending work, emails is approved, quiet holds no item,
// and loose.txt is in no collection. No collection is the default, which
// would take loose.txt in. It returns the project
// and the collection ids by name.
func seedCollectionsProject(t *testing.T, s *Server, wsID string) (string, map[string]string, map[string]string) {
	t.Helper()
	ctx := context.Background()
	proj := &platstore.Project{
		Name: "Collections", WorkspaceID: wsID, DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"},
		Properties:      map[string]string{"translate_after": "none"},
	}
	require.NoError(t, s.ContentStore.CreateProject(ctx, proj))
	colls := map[string]string{}
	for _, name := range []string{"web", "docs", "emails", "quiet"} {
		coll := &platstore.Collection{ProjectID: proj.ID, Name: name, Kind: platstore.CollectionUploaded}
		if name == "web" {
			coll.ConnectorConfig = map[string]string{coreprofile.PropertyChannel: "acme/web"}
			coll.Context = map[string]string{"product": "acme"}
		}
		require.NoError(t, s.ContentStore.CreateCollection(ctx, coll))
		colls[name] = coll.ID
	}
	items := []struct {
		name, collection string
		blocks           []*model.Block
	}{
		{"web.json", colls["web"], []*model.Block{
			pendingFrBlock("w1", "Save", "Enregistrer"),
			pendingFrBlock("w2", "Cancel", "Annuler"),
			establishedFrBlock("w3", "Open", "Ouvrir"),
		}},
		{"docs.md", colls["docs"], []*model.Block{pendingFrBlock("d1", "Read the guide", "Lisez le guide")}},
		{"emails.txt", colls["emails"], []*model.Block{establishedFrBlock("e1", "Welcome", "Bienvenue")}},
		{"loose.txt", "", []*model.Block{pendingFrBlock("l1", "Loose", "Libre")}},
	}
	ids := map[string]string{}
	for _, it := range items {
		require.NoError(t, s.ContentStore.StoreItem(ctx, proj.ID, "main", &platstore.Item{
			Name: it.name, Format: "txt", ItemType: "file", CollectionID: it.collection,
		}))
		require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, proj.ID, "main", it.name, it.blocks))
		stored, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: proj.ID, Stream: "main", ItemName: it.name})
		require.NoError(t, err)
		for _, sb := range stored {
			ids[sb.Block.SourceText()] = sb.Block.ID
		}
	}
	return proj.ID, colls, ids
}

func TestReviewDeckCollections_CountsAndOrdersEachCollection(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, colls, ids := seedCollectionsProject(t, s, wsID)
	recordScore(t, s, projID, ids["Save"], 95)

	rec := deckCall(t, s, s.HandleGetReviewDeckCollections, wsID, projID, "", "u-reviewer", testReviewer, nil, url.Values{"locale": {"fr"}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := decodeJSON[reviewDeckCollectionsResponse](t, rec)

	assert.Equal(t, "fr", got.Locale)
	assert.Equal(t, "main", got.Stream)
	assert.Equal(t, deckCollection{Waiting: 4, Fast: 1, Closer: 3, Translations: 6, Decided: 2}, got.All)

	want := []deckCollection{
		{CollectionID: colls["docs"], Name: "docs", Waiting: 1, Closer: 1, Translations: 1},
		{CollectionID: colls["web"], Name: "web", Channel: "acme/web", Coordinates: map[string]string{"product": "acme"}, Waiting: 2, Fast: 1, Closer: 1, Translations: 3, Decided: 1},
		{CollectionID: "", Name: "", Waiting: 1, Closer: 1, Translations: 1},
		{CollectionID: colls["emails"], Name: "emails", Translations: 1, Decided: 1},
	}
	require.Len(t, got.Collections, len(want), "%+v", got.Collections)
	for i, w := range want {
		t.Run(w.Name, func(t *testing.T) {
			assert.Equal(t, w, got.Collections[i])
		})
	}
}

func TestReviewDeckCollections_DeckNarrowsToACollection(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, colls, ids := seedCollectionsProject(t, s, wsID)

	tests := []struct {
		name       string
		collection *string
		want       []string
	}{
		{"whole language", nil, []string{ids["Save"], ids["Cancel"], ids["Read the guide"], ids["Loose"]}},
		{"one collection", new(colls["web"]), []string{ids["Save"], ids["Cancel"]}},
		{"no collection", new(""), []string{ids["Loose"]}},
		{"nothing waiting", new(colls["emails"]), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := url.Values{"locale": {"fr"}}
			if tt.collection != nil {
				q.Set("collection", *tt.collection)
			}
			deck := readDeck(t, s, wsID, projID, q)
			var got []string
			for _, c := range deck.Cards {
				got = append(got, c.BlockID)
			}
			assert.ElementsMatch(t, tt.want, got)
			assert.Equal(t, len(tt.want), deck.Counts.Total)
		})
	}
}

func TestReviewDeckCollections_NeedsTheReviewPermissionForTheLanguage(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _, _ := seedCollectionsProject(t, s, wsID)

	tests := []struct {
		name  string
		user  string
		langs []string
		query url.Values
		want  int
	}{
		{"translator", "u-translator", nil, url.Values{"locale": {"fr"}}, http.StatusForbidden},
		{"reviewer of another language", "u-reviewer", []string{"de"}, url.Values{"locale": {"fr"}}, http.StatusForbidden},
		{"reviewer of the language", "u-reviewer", []string{"fr"}, url.Values{"locale": {"fr"}}, http.StatusOK},
		{"no locale", "u-reviewer", nil, url.Values{}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perms := testReviewer
			if tt.user == "u-translator" {
				perms = testTranslator
			}
			rec := deckCall(t, s, s.HandleGetReviewDeckCollections, wsID, projID, "", tt.user, perms, tt.langs, tt.query)
			assert.Equal(t, tt.want, rec.Code, rec.Body.String())
		})
	}
}
