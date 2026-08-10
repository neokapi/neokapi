package server

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// pendingReviewEntry is one queue entry: the (block, locale) pair awaiting a
// decision, carried with its full block payload so the session renders
// without a follow-up fetch per item.
type pendingReviewEntry struct {
	BlockID  string             `json:"block_id"`
	ItemName string             `json:"item_name"`
	Locale   string             `json:"locale"`
	Block    *BlockInfoResponse `json:"block,omitempty"`
	// CollectionID is the collection of the entry's item, "" for an item in no
	// collection. It comes from the same join the collection filter tests, so a
	// queue narrowed to a collection and a queue grouped by collection cannot
	// disagree about where a row belongs.
	CollectionID string `json:"collection_id"`
}

type pendingReviewResponse struct {
	Entries []pendingReviewEntry `json:"entries"`
	Total   int                  `json:"total"`
	Limit   int                  `json:"limit"`
	Offset  int                  `json:"offset"`
}

// HandleListPendingReview pages the translation review queue. One indexed
// query answers what the review session once assembled client-side with a
// blocks fetch per item — minutes of "gathering" at dogfood scale, and only
// ever the first dashboard page of it.
//
// The optional `collection` query parameter narrows the queue to one
// collection's items, server-side. PRESENCE of the key is the filter, not its
// value: `?collection=` selects the items in no collection — the ungrouped
// bucket the dashboard rollups name — which is a scope of its own and not the
// absence of one. A reviewer arriving from a collection card used to get the
// project's whole queue and filter it in the browser over a bounded slice, so a
// collection larger than the slice showed fewer entries than its card counted.
func (s *Server) HandleListPendingReview(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	ctx := c.Request().Context()

	var locales []string
	if raw := strings.TrimSpace(c.QueryParam("locales")); raw != "" {
		for l := range strings.SplitSeq(raw, ",") {
			if l = strings.TrimSpace(l); l != "" {
				locales = append(locales, l)
			}
		}
	}
	limit, offset := pageParams(c, 200, 500)

	var collectionID *string
	if c.QueryParams().Has("collection") {
		scope := strings.TrimSpace(c.QueryParam("collection"))
		collectionID = &scope
	}

	refs, total, err := s.ContentStore.ListPendingReview(ctx, store.PendingReviewQuery{
		ProjectID:    pid,
		Stream:       streamParam(c),
		Locales:      locales,
		CollectionID: collectionID,
		Limit:        limit,
		Offset:       offset,
	})
	if err != nil {
		return serverErr(c, err)
	}

	// Hydrate the page's distinct blocks in one fetch.
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	targetLocales := make([]string, len(proj.TargetLanguages))
	for i, l := range proj.TargetLanguages {
		targetLocales[i] = string(l)
	}
	ids := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, r := range refs {
		if !seen[r.BlockID] {
			seen[r.BlockID] = true
			ids = append(ids, r.BlockID)
		}
	}
	byID := map[string]*BlockInfoResponse{}
	if len(ids) > 0 {
		stored, err := s.ContentStore.GetBlocks(ctx, store.BlockQuery{
			ProjectID: pid,
			Stream:    streamParam(c),
			IDs:       ids,
		})
		if err != nil {
			return serverErr(c, err)
		}
		for _, sb := range stored {
			bi := storedBlockToInfoResponse(sb, targetLocales)
			byID[bi.ID] = &bi
		}
	}

	entries := make([]pendingReviewEntry, 0, len(refs))
	for _, r := range refs {
		entries = append(entries, pendingReviewEntry{
			BlockID:      r.BlockID,
			ItemName:     r.ItemName,
			Locale:       r.Locale,
			Block:        byID[r.BlockID],
			CollectionID: r.CollectionID,
		})
	}
	return c.JSON(http.StatusOK, pendingReviewResponse{Entries: entries, Total: total, Limit: limit, Offset: offset})
}
