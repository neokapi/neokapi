package server

import (
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
)

// The deck by page: the translations waiting in one language, grouped by what
// a reader meets them inside. A component's strings are one page; an item's
// other blocks are another, which a reviewer walks section by section through
// the container route. Each page names its first waiting translation, which
// opens its container.

const reviewPagesLimit = 200

// deckPage is one page of the deck.
type deckPage struct {
	// Kind is component or document.
	Kind string `json:"kind"`
	// Title names the page: the component, or the item's file name.
	Title        string `json:"title"`
	ItemName     string `json:"item_name"`
	CollectionID string `json:"collection_id"`
	Component    string `json:"component,omitempty"`
	Waiting      int    `json:"waiting"`
	// Findings counts the page's waiting translations with a failing
	// finding.
	Findings     int    `json:"findings,omitempty"`
	FirstBlockID string `json:"first_block_id"`
}

type reviewDeckPagesResponse struct {
	Locale string     `json:"locale"`
	Pages  []deckPage `json:"pages"`
	// Waiting is the sum of the pages' waiting counts.
	Waiting int `json:"waiting"`
	// More reports pages past the ones returned.
	More bool `json:"more"`
}

// HandleGetReviewDeckPages answers GET /:ws/:id/review-deck/:ref/pages
// ?locale=nb[&collection=c][&sample=true] with the deck's pages. With sample,
// it answers the pages a reviewer reads under the involvement context (see
// interfaceSample).
func (s *Server) HandleGetReviewDeckPages(c echo.Context) error {
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
	scope, err := deckScopeOf(ctx, s.ContentStore, pid, stream)
	if err != nil {
		return serverErr(c, err)
	}
	q := store.ReviewDeckQuery{ProjectID: pid, Stream: stream, Locale: loc, Policy: deckPolicyKey(s.deckPolicy())}
	scope.apply(&q)
	if c.QueryParams().Has("collection") {
		coll := strings.TrimSpace(c.QueryParam("collection"))
		q.CollectionID = &coll
	}
	sample := c.QueryParam("sample") == "true"
	limit := reviewPagesLimit + 1
	if sample {
		limit = reviewPagesCountLimit
	}
	pages, err := decks.ListReviewDeckPages(ctx, q, limit)
	if err != nil {
		return serverErr(c, err)
	}
	if sample {
		pages = interfaceSample(pages)
	}
	out := reviewDeckPagesResponse{Locale: loc, Pages: []deckPage{}}
	if len(pages) > reviewPagesLimit {
		pages, out.More = pages[:reviewPagesLimit], true
	}
	for _, p := range pages {
		out.Pages = append(out.Pages, deckPageOf(p))
		out.Waiting += p.Waiting
	}
	return c.JSON(http.StatusOK, out)
}

func deckPageOf(p store.ReviewDeckPage) deckPage {
	out := deckPage{
		Kind: "document", Title: path.Base(p.ItemName), ItemName: p.ItemName, CollectionID: p.CollectionID,
		Waiting: p.Waiting, Findings: p.Findings, FirstBlockID: p.FirstBlockID,
	}
	if p.Component != "" {
		out.Kind, out.Title, out.Component = "component", p.Component, p.Component
	}
	return out
}
