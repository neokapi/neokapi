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
		{CollectionID: colls["docs"], Name: "docs", Priority: "normal", Waiting: 1, Closer: 1, Translations: 1},
		{CollectionID: colls["web"], Name: "web", Priority: "normal", Channel: "acme/web", Coordinates: map[string]string{"product": "acme"}, Waiting: 2, Fast: 1, Closer: 1, Translations: 3, Decided: 1},
		{CollectionID: "", Name: "", Waiting: 1, Closer: 1, Translations: 1},
		{CollectionID: colls["emails"], Name: "emails", Priority: "normal", Translations: 1, Decided: 1},
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

func TestReviewDeck_LeavesOutSourceOnlyCollections(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()
	projID, colls, ids := seedCollectionsProject(t, s, wsID)
	docs, err := s.ContentStore.GetCollection(ctx, projID, colls["docs"])
	require.NoError(t, err)
	docs.ConnectorConfig = map[string]string{platstore.CollectionConfigSourceOnly: "true"}
	require.NoError(t, s.ContentStore.UpdateCollection(ctx, docs))

	deck := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	var got []string
	for _, c := range deck.Cards {
		got = append(got, c.BlockID)
	}
	assert.ElementsMatch(t, []string{ids["Save"], ids["Cancel"], ids["Loose"]}, got)

	rec := deckCall(t, s, s.HandleGetReviewDeckCollections, wsID, projID, "", "u-reviewer", testReviewer, nil, url.Values{"locale": {"fr"}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	listed := decodeJSON[reviewDeckCollectionsResponse](t, rec)
	for _, c := range listed.Collections {
		assert.NotEqual(t, colls["docs"], c.CollectionID)
	}
	assert.Equal(t, 3, listed.All.Waiting)

	refs, total, err := s.ContentStore.ListPendingReview(ctx, platstore.PendingReviewQuery{
		ProjectID: projID, Stream: "main", ExcludeCollections: []string{colls["docs"]},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	for _, r := range refs {
		assert.NotEqual(t, ids["Read the guide"], r.BlockID)
	}

	skip, err := sourceOnlyItems(ctx, s.ContentStore, projID, "main")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"docs.md": true}, skip)
}

func TestReviewDeck_DealsCollectionsByPriority(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()
	projID, colls, ids := seedCollectionsProject(t, s, wsID)
	setPriority := func(name, p string) {
		coll, err := s.ContentStore.GetCollection(ctx, projID, colls[name])
		require.NoError(t, err)
		coll.ConnectorConfig = map[string]string{platstore.CollectionConfigPriority: p}
		require.NoError(t, s.ContentStore.UpdateCollection(ctx, coll))
	}
	setPriority("docs", "high")
	setPriority("web", "low")

	// One card a page, so every step crosses the cursor.
	var got []string
	q := url.Values{"locale": {"fr"}, "limit": {"1"}}
	for range 10 {
		deck := readDeck(t, s, wsID, projID, q)
		for _, c := range deck.Cards {
			got = append(got, c.BlockID)
		}
		if deck.NextCursor == "" {
			break
		}
		q.Set("cursor", deck.NextCursor)
	}
	require.Len(t, got, 4)
	assert.Equal(t, ids["Read the guide"], got[0], "the high collection comes first")
	assert.Equal(t, ids["Loose"], got[1], "no collection is normal")
	assert.ElementsMatch(t, []string{ids["Save"], ids["Cancel"]}, got[2:], "the low collection comes last")

	rec := deckCall(t, s, s.HandleGetReviewDeckCollections, wsID, projID, "", "u-reviewer", testReviewer, nil, url.Values{"locale": {"fr"}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	listed := decodeJSON[reviewDeckCollectionsResponse](t, rec)
	require.NotEmpty(t, listed.Collections)
	assert.Equal(t, colls["docs"], listed.Collections[0].CollectionID)
	assert.Equal(t, "high", listed.Collections[0].Priority)
}

func TestReviewDeck_DealsIdenticalTextOnce(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()
	projID, colls, _ := seedCollectionsProject(t, s, wsID)
	// The same source with the same translation in another collection, and
	// once with a different translation.
	require.NoError(t, s.ContentStore.StoreItem(ctx, projID, "main", &platstore.Item{
		Name: "more.json", Format: "txt", ItemType: "file", CollectionID: colls["docs"],
	}))
	require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, projID, "main", "more.json", []*model.Block{
		pendingFrBlock("m1", "Save", "Enregistrer"),
		pendingFrBlock("m2", "Save", "Sauvegarder"),
	}))

	deck := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	var same []deckCard
	targets := map[string]int{}
	for _, c := range deck.Cards {
		if c.SourceText == "Save" {
			targets[c.TargetText]++
			if c.TargetText == "Enregistrer" {
				same = append(same, c)
			}
		}
	}
	assert.Equal(t, map[string]int{"Enregistrer": 1, "Sauvegarder": 1}, targets, "one card per translation")
	require.Len(t, same, 1)
	require.Len(t, same[0].Copies, 1)
	cp := same[0].Copies[0]
	assert.NotEqual(t, same[0].BlockID, cp.BlockID)
	assert.NotEmpty(t, cp.TargetRevision)
	assert.NotEmpty(t, cp.SourceRevision)
	assert.Equal(t, 5, deck.Counts.Waiting, "the copy counts once")

	// A collection's own deck reads only its collection, and its card still
	// lists the copy in the other one.
	docs := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}, "collection": {colls["docs"]}})
	for _, c := range docs.Cards {
		if c.SourceText == "Save" && c.TargetText == "Enregistrer" {
			require.Len(t, c.Copies, 1)
			assert.Equal(t, colls["web"], c.Copies[0].CollectionID)
		}
	}
}
