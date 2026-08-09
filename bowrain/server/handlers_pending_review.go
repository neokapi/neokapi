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

	refs, total, err := s.ContentStore.ListPendingReview(ctx, pid, streamParam(c), locales, limit, offset)
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
			BlockID:  r.BlockID,
			ItemName: r.ItemName,
			Locale:   r.Locale,
			Block:    byID[r.BlockID],
		})
	}
	return c.JSON(http.StatusOK, pendingReviewResponse{Entries: entries, Total: total, Limit: limit, Offset: offset})
}
