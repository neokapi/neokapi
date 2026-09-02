package server

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
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
	// TermCompliance is this target's terminology verdict, empty when no
	// terminology governance was active for the locale. It is one of the two
	// bars beyond the rule-based checks that approve-passing applies, so a queue
	// that bucketed on checks alone called blocks passing that the server refuses.
	TermCompliance store.TermCompliance `json:"term_compliance,omitempty"`
	// VoiceScore is the latest persisted voice score for this block and
	// locale, and VoiceBar the compliance bar of the profile that produced it
	// (VoiceProfile.ComplianceBar). Both are absent together when the block has
	// never been scored — the server then applies no voice bar to it either.
	VoiceScore *int `json:"voice_score,omitempty"`
	VoiceBar   *int `json:"voice_bar,omitempty"`
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
	blockByID := map[string]*model.Block{}
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
			if sb.Block != nil {
				blockByID[sb.Block.ID] = sb.Block
			}
		}
	}

	// The two bars beyond the rule-based checks that approve-passing applies,
	// resolved ONCE for the page: the terminology gate (one terms snapshot + a
	// voice profile per locale, then offline per block) and the project's
	// persisted voice scores (one read, latest per block+locale, each paired with
	// the bar of the profile that produced it). Both are the same helpers
	// shipstate.go uses, so the queue's verdict is the predicate the bulk pass
	// will actually apply.
	wsID, _ := c.Get("workspace_id").(string)
	gate := s.resolveTermGate(ctx, proj, streamParam(c), wsID)
	scores := latestVoiceScores(ctx, s.VoiceStore, pid, streamParam(c))

	entries := make([]pendingReviewEntry, 0, len(refs))
	for _, r := range refs {
		entry := pendingReviewEntry{
			BlockID:      r.BlockID,
			ItemName:     r.ItemName,
			Locale:       r.Locale,
			Block:        byID[r.BlockID],
			CollectionID: r.CollectionID,
		}
		loc := model.LocaleID(r.Locale)
		if block := blockByID[r.BlockID]; block != nil {
			// Unchecked and compliant are different answers: with no governance
			// active there is nothing the target could have violated, and
			// claiming compliance would be claiming evidence.
			if gate.active(ctx, loc) {
				entry.TermCompliance = store.TermComplianceCompliant
				if !gate.compliant(ctx, block, loc) {
					entry.TermCompliance = store.TermComplianceViolation
				}
			}
			if vs, ok := scores[string(locale.Normalize(loc))][block.ID]; ok {
				score, bar := vs.score, vs.bar
				entry.VoiceScore, entry.VoiceBar = &score, &bar
			}
		}
		entries = append(entries, entry)
	}
	return c.JSON(http.StatusOK, pendingReviewResponse{Entries: entries, Total: total, Limit: limit, Offset: offset})
}
