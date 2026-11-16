package server

import (
	"cmp"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/host/reviewdeck"
)

// deckCollection is the review deck of one language narrowed to one
// collection, as GET /review-deck/:ref?collection= serves it.
type deckCollection struct {
	// CollectionID is what the deck's collection parameter takes. It is
	// empty for the blocks in no collection, and for the whole language.
	CollectionID string `json:"collection_id"`
	Name         string `json:"name"`
	// Waiting is what the deck holds, counted as reviewInboxLanguage counts
	// it.
	Waiting      int `json:"waiting"`
	Fast         int `json:"fast"`
	Closer       int `json:"closer"`
	Desk         int `json:"desk"`
	Unclassified int `json:"unclassified"`
	// Translations is every translation of the language in the collection,
	// and Decided the part no longer waiting: approved, or rejected and
	// waiting for a new draft. Decided of Translations is the progress.
	Translations int `json:"translations"`
	Decided      int `json:"decided"`
}

type reviewDeckCollectionsResponse struct {
	Locale string `json:"locale"`
	Stream string `json:"stream"`
	// All is the whole language: the deck without a collection parameter.
	All         deckCollection   `json:"all"`
	Collections []deckCollection `json:"collections"`
}

// HandleGetReviewDeckCollections answers the review deck of one language
// split by collection, so a reviewer can take it one collection at a time.
//
// Collections with something waiting come first, in the project's collection
// order (the default collection, then by name), and the blocks in no
// collection follow them. Collections with nothing left waiting come last. A
// collection with no translation in the language is left out. Like the deck,
// the request first classifies what is new, within the same budget, so the
// group counts fill in as the deck is used.
//
// GET /:ws/:id/review-deck/:ref/collections?locale=nb
func (s *Server) HandleGetReviewDeckCollections(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	decks, ok := store.ReviewDeckStores(s.ContentStore)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "the review deck needs the PostgreSQL content store"})
	}
	loc := strings.TrimSpace(c.QueryParam("locale"))
	if loc == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale is required"})
	}
	if err := s.requireLanguagePermission(c, platauth.PermReview, loc); err != nil {
		return err
	}

	ctx := c.Request().Context()
	pid := projectParam(c)
	stream := streamParam(c)
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	policy := s.deckPolicy()
	q := store.ReviewDeckQuery{ProjectID: pid, Stream: stream, Locale: loc, Policy: deckPolicyKey(policy)}
	if err := s.classifyDeck(ctx, c, decks, proj, q, policy); err != nil {
		return serverErr(c, err)
	}
	counts, err := decks.CountReviewDeckByCollection(ctx, q)
	if err != nil {
		return serverErr(c, err)
	}
	colls, err := s.ContentStore.ListCollections(ctx, pid, stream)
	if err != nil {
		return serverErr(c, err)
	}

	out := reviewDeckCollectionsResponse{Locale: loc, Stream: stream, Collections: []deckCollection{}}
	for _, n := range counts {
		addDeckCollection(&out.All, deckCollectionOf("", "", n))
	}
	var waiting, done []deckCollection
	add := func(id, name string) {
		n, ok := counts[id]
		if !ok || n.Translations == 0 {
			return
		}
		dc := deckCollectionOf(id, name, n)
		if dc.Waiting > 0 {
			waiting = append(waiting, dc)
		} else {
			done = append(done, dc)
		}
	}
	for _, coll := range colls {
		if coll.ID != "" {
			add(coll.ID, coll.Name)
		}
	}
	add("", "")
	out.Collections = append(append(out.Collections, waiting...), done...)
	return c.JSON(http.StatusOK, out)
}

func deckCollectionOf(id, name string, n store.ReviewDeckCollectionCounts) deckCollection {
	dc := deckCollection{
		CollectionID: id,
		Name:         cmp.Or(name, id),
		Fast:         n.ByRank[deckRank(reviewdeck.DeckFast)],
		Closer:       n.ByRank[deckRank(reviewdeck.DeckCloser)],
		Desk:         n.ByRank[deckRank(reviewdeck.DeckDesk)],
		Unclassified: n.Unclassified,
		Translations: n.Translations,
	}
	dc.Waiting = dc.Fast + dc.Closer + dc.Desk + dc.Unclassified
	dc.Decided = dc.Translations - dc.Waiting
	return dc
}

func addDeckCollection(sum *deckCollection, dc deckCollection) {
	sum.Waiting += dc.Waiting
	sum.Fast += dc.Fast
	sum.Closer += dc.Closer
	sum.Desk += dc.Desk
	sum.Unclassified += dc.Unclassified
	sum.Translations += dc.Translations
	sum.Decided += dc.Decided
}
