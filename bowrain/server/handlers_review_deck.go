package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host/reviewdeck"
	"github.com/neokapi/neokapi/terms"
)

// The review deck: the translations waiting for review in one language,
// grouped by the attention each needs (reviewdeck.ClassifyDeckCard) and
// ordered fast, closer, then desk. Each card carries what a reviewer decides
// on and the revisions a decision binds to, so a client sends a change set
// straight from the card.

const (
	// deckScanBatch is how many queue entries one hydration reads.
	deckScanBatch = 500
	// deckScanLimit bounds the entries one request classifies. A longer
	// queue is answered for its first deckScanLimit entries, and the counts
	// say they are partial.
	deckScanLimit = 10000
	// deckNoteLimit bounds the check notes one card carries.
	deckNoteLimit = 3
)

// deckNote is one line about a card: a check finding, a term the source
// uses, the voice score, or the pre-review's reason.
type deckNote struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// deckCard is one translation waiting for review.
type deckCard struct {
	BlockID      string `json:"block_id"`
	ItemName     string `json:"item_name"`
	Key          string `json:"key"`
	CollectionID string `json:"collection_id"`
	Locale       string `json:"locale"`

	SourceRuns []model.Run `json:"source_runs"`
	TargetRuns []model.Run `json:"target_runs"`
	SourceText string      `json:"source_text"`
	TargetText string      `json:"target_text"`
	// TargetEditText is the translation in the placeholder form a set_content
	// text takes, each code an <x id="…"/>.
	TargetEditText string `json:"target_edit_text"`
	// SourceRevision is the revision a change set's basis names, and
	// TargetRevision the revision its if_match names.
	SourceRevision string `json:"source_revision"`
	TargetRevision string `json:"target_revision"`
	TargetStatus   string `json:"target_status,omitempty"`

	PreReview            *preReviewView       `json:"pre_review,omitempty"`
	FindingsCount        int                  `json:"findings_count"`
	FailingFindingsCount int                  `json:"failing_findings_count"`
	TermCompliance       store.TermCompliance `json:"term_compliance,omitempty"`
	VoiceScore           *int                 `json:"voice_score,omitempty"`
	VoiceBar             *int                 `json:"voice_bar,omitempty"`
	Notes                []deckNote           `json:"notes"`
	// OwnWork is a translation the caller wrote.
	OwnWork bool `json:"own_work"`

	Group  reviewdeck.DeckGroup `json:"group"`
	Reason string               `json:"reason"`

	// block is the stored block the card was built from, for the notes the
	// page's cards are given.
	block *model.Block
}

type deckCounts struct {
	Fast    int  `json:"fast"`
	Closer  int  `json:"closer"`
	Desk    int  `json:"desk"`
	Total   int  `json:"total"`
	Partial bool `json:"partial,omitempty"`
}

type reviewDeckResponse struct {
	Locale     string                `json:"locale"`
	Cards      []deckCard            `json:"cards"`
	Counts     deckCounts            `json:"counts"`
	NextCursor string                `json:"next_cursor"`
	Policy     reviewdeck.DeckPolicy `json:"policy"`
}

// deckCursor is the card a page ended on.
type deckCursor struct {
	group   reviewdeck.DeckGroup
	blockID string
	locale  string
}

func (c deckCursor) encode() string {
	raw := string(c.group) + "\x00" + c.blockID + "\x00" + c.locale
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeDeckCursor(s string) (deckCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return deckCursor{}, err
	}
	parts := strings.Split(string(raw), "\x00")
	if len(parts) != 3 || !slices.Contains(reviewdeck.DeckGroups, reviewdeck.DeckGroup(parts[0])) {
		return deckCursor{}, fmt.Errorf("malformed cursor")
	}
	return deckCursor{group: reviewdeck.DeckGroup(parts[0]), blockID: parts[1], locale: parts[2]}, nil
}

// deckPolicy is the policy the server's deck applies.
func (s *Server) deckPolicy() reviewdeck.DeckPolicy {
	return s.ReviewDeckPolicy.WithDefaults()
}

// HandleGetReviewDeck answers the review deck for one language.
//
// GET /:ws/:id/review-deck/:ref?locale=nb&limit=50&cursor=&group=&collection=
func (s *Server) HandleGetReviewDeck(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	loc := strings.TrimSpace(c.QueryParam("locale"))
	if loc == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale is required"})
	}
	if err := s.requireLanguagePermission(c, platauth.PermReview, loc); err != nil {
		return err
	}
	limit, _ := pageParams(c, 50, 200)
	var only reviewdeck.DeckGroup
	if g := strings.TrimSpace(c.QueryParam("group")); g != "" {
		only = reviewdeck.DeckGroup(g)
		if !slices.Contains(reviewdeck.DeckGroups, only) {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "group is fast, closer or desk"})
		}
	}
	var cursor *deckCursor
	if raw := strings.TrimSpace(c.QueryParam("cursor")); raw != "" {
		cur, err := decodeDeckCursor(raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cursor is not one this endpoint returned"})
		}
		cursor = &cur
	}
	var collectionID *string
	if c.QueryParams().Has("collection") {
		scope := strings.TrimSpace(c.QueryParam("collection"))
		collectionID = &scope
	}

	ctx := c.Request().Context()
	pid := projectParam(c)
	stream := streamParam(c)
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	policy := s.deckPolicy()
	cards, partial, err := s.classifyDeck(ctx, c, proj, stream, loc, collectionID, policy)
	if err != nil {
		return serverErr(c, err)
	}

	out := reviewDeckResponse{Locale: loc, Cards: []deckCard{}, Policy: policy}
	out.Counts.Partial = partial
	ordered := make([]deckCard, 0, len(cards))
	for _, g := range reviewdeck.DeckGroups {
		for _, card := range cards {
			if card.Group == g {
				ordered = append(ordered, card)
			}
		}
	}
	for _, card := range ordered {
		switch card.Group {
		case reviewdeck.DeckFast:
			out.Counts.Fast++
		case reviewdeck.DeckCloser:
			out.Counts.Closer++
		case reviewdeck.DeckDesk:
			out.Counts.Desk++
		}
	}
	out.Counts.Total = len(ordered)

	if only != "" {
		ordered = slices.DeleteFunc(ordered, func(card deckCard) bool { return card.Group != only })
	}
	start := deckStart(ordered, cursor)
	end := min(start+limit, len(ordered))
	page := ordered[start:end]
	s.addTermNotes(ctx, c.Param("ws"), proj, loc, page)
	for i := range page {
		page[i].block = nil
	}
	out.Cards = page
	if end < len(ordered) && len(page) > 0 {
		last := page[len(page)-1]
		out.NextCursor = deckCursor{group: last.Group, blockID: last.BlockID, locale: last.Locale}.encode()
	}
	return c.JSON(http.StatusOK, out)
}

// deckStart is the index of the first card after cursor. When the cursor's
// card has left the queue, the page starts at the first card of its group.
func deckStart(cards []deckCard, cursor *deckCursor) int {
	if cursor == nil {
		return 0
	}
	for i, card := range cards {
		if card.BlockID == cursor.blockID && card.Locale == cursor.locale {
			return i + 1
		}
	}
	for i, card := range cards {
		if slices.Index(reviewdeck.DeckGroups, card.Group) >= slices.Index(reviewdeck.DeckGroups, cursor.group) {
			return i
		}
	}
	return len(cards)
}

// classifyDeck reads the pending-review queue for one language, batch by
// batch, and builds a card for each entry with its group. It reports whether
// the queue was longer than deckScanLimit.
func (s *Server) classifyDeck(ctx context.Context, c echo.Context, proj *store.Project, stream, loc string, collectionID *string, policy reviewdeck.DeckPolicy) ([]deckCard, bool, error) {
	pid := proj.ID
	targetLoc := model.LocaleID(loc)
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
	gate := s.resolveTermGate(ctx, proj, stream, wsID)
	scored := latestVoiceScores(ctx, s.VoiceStore, pid, stream)[string(locale.Normalize(targetLoc))]
	decisions := s.deckDecisions(ctx, pid, stream)
	authors, _ := s.ContentStore.(store.TargetAuthorStore)

	var cards []deckCard
	for offset := 0; offset < deckScanLimit; offset += deckScanBatch {
		refs, total, err := s.ContentStore.ListPendingReview(ctx, store.PendingReviewQuery{
			ProjectID: pid, Stream: stream, Locales: []string{loc}, CollectionID: collectionID,
			Limit: min(deckScanBatch, deckScanLimit-offset), Offset: offset,
		})
		if err != nil {
			return nil, false, err
		}
		if len(refs) == 0 {
			return cards, false, nil
		}
		ids := make([]string, 0, len(refs))
		for _, r := range refs {
			if !slices.Contains(ids, r.BlockID) {
				ids = append(ids, r.BlockID)
			}
		}
		stored, err := s.ContentStore.GetBlocks(ctx, store.BlockQuery{ProjectID: pid, Stream: stream, IDs: ids})
		if err != nil {
			return nil, false, err
		}
		byID := make(map[string]*venue.StoredBlock, len(stored))
		for _, sb := range stored {
			if sb.Block != nil {
				byID[sb.Block.ID] = sb
			}
		}
		preReviews, err := s.ContentStore.PreReviews(ctx, pid, stream, ids)
		if err != nil {
			slog.WarnContext(ctx, "review deck: read pre-reviews failed", "project", pid, "error", err)
		}
		var wrote map[store.TargetRef]string
		if authors != nil && actor != "" {
			if wrote, err = authors.LastTargetAuthors(ctx, pid, stream, ids, []string{loc}); err != nil {
				slog.WarnContext(ctx, "review deck: read target authors failed", "project", pid, "error", err)
			}
		}
		for _, r := range refs {
			sb := byID[r.BlockID]
			if sb == nil {
				continue
			}
			card := s.deckCardFor(ctx, sb, r, targetLoc, gate, scored, preReviews, decisions, policy, proj.DefaultSourceLanguage)
			card.OwnWork = actor != "" && wrote[store.TargetRef{BlockID: r.BlockID, Locale: r.Locale}] == actor
			cards = append(cards, card)
		}
		if offset+len(refs) >= total {
			return cards, false, nil
		}
	}
	return cards, true, nil
}

// deckDecisions reads the stream's decision ledger once, keyed by item, unit
// and variant. A store without a ledger has no decisions to read.
func (s *Server) deckDecisions(ctx context.Context, pid, stream string) map[string]venue.UnitDecision {
	ds, ok := s.ContentStore.(store.DecisionStore)
	if !ok {
		return nil
	}
	rows, err := ds.ListUnitDecisions(ctx, pid, stream)
	if err != nil {
		slog.WarnContext(ctx, "review deck: read decisions failed", "project", pid, "error", err)
		return nil
	}
	out := make(map[string]venue.UnitDecision, len(rows))
	for _, d := range rows {
		out[decisionKey(d.ItemName, d.Unit, d.Variant)] = d
	}
	return out
}

func decisionKey(item, unit, variant string) string {
	return item + "\x00" + unit + "\x00" + string(model.NormalizeLocale(model.LocaleID(variant)))
}

// deckCardFor builds one card and places it in its group.
func (s *Server) deckCardFor(
	ctx context.Context,
	sb *venue.StoredBlock,
	r store.PendingReviewRef,
	loc model.LocaleID,
	gate *termGate,
	scored map[string]scoredBlock,
	preReviews []store.PreReview,
	decisions map[string]venue.UnitDecision,
	policy reviewdeck.DeckPolicy,
	sourceLang model.LocaleID,
) deckCard {
	b := sb.Block
	target, _ := b.TargetEdition(loc)
	card := deckCard{
		BlockID:        b.ID,
		ItemName:       r.ItemName,
		Key:            b.Name,
		CollectionID:   r.CollectionID,
		Locale:         r.Locale,
		SourceRuns:     b.SourceRuns(),
		TargetRuns:     target.Runs,
		SourceText:     model.RenderRunsWithData(b.SourceRuns()),
		TargetText:     model.RenderRunsWithData(target.Runs),
		TargetEditText: model.RunsEditText(target.Runs),
		SourceRevision: check.SourceRevision(b, sourceLang),
		TargetRevision: store.TargetRevision(sb, loc),
		TargetStatus:   string(target.Status),
		Notes:          []deckNote{},
		block:          b,
	}
	if card.SourceRuns == nil {
		card.SourceRuns = []model.Run{}
	}
	if card.TargetRuns == nil {
		card.TargetRuns = []model.Run{}
	}
	facts := reviewdeck.DeckFacts{SourceChars: utf8.RuneCountInString(model.RunsText(b.SourceRuns()))}

	// The rule-based checks approve-passing applies.
	if issues, err := runChecksOnBlock(ctx, b, pointChecks{TargetLocale: loc}); err == nil {
		for _, issue := range issues {
			facts.Findings++
			if issue.Severity == "error" {
				facts.FailingFindings++
			}
			if len(card.Notes) < deckNoteLimit {
				card.Notes = append(card.Notes, deckNote{Kind: "check", Text: issue.Message})
			}
		}
	}
	// The terminology bar: a violation fails, as approve-passing reads it.
	card.TermCompliance = gate.compliance(ctx, b, loc)
	if card.TermCompliance == store.TermComplianceViolation {
		facts.Findings++
		facts.FailingFindings++
		card.Notes = append(card.Notes, deckNote{Kind: "term", Text: "uses a term the terms store rejects, or misses one it requires"})
	}
	// The voice bar: a score under the bar is a finding that does not fail.
	switch gate.voiceCompliance(ctx, b.ID, loc, scored) {
	case store.VoiceCompliancePassing, store.VoiceComplianceBelowBar:
		vs := scored[b.ID]
		score, bar := vs.score, vs.bar
		card.VoiceScore, card.VoiceBar = &score, &bar
		if score < bar {
			facts.Findings++
			card.Notes = append(card.Notes, deckNote{Kind: "voice", Text: fmt.Sprintf("voice score %d, under the bar of %d", score, bar)})
		}
	case store.VoiceComplianceUnchecked:
		bar := gate.profileFor(ctx, loc).ComplianceBar()
		card.VoiceBar = &bar
	}
	if pr := freshPreReview(preReviews, sb, loc); pr != nil {
		card.PreReview = preReviewViewOf(pr)
		score := pr.Score
		facts.Score = &score
		if len(pr.Reasons) > 0 && score < policy.FastMinScore {
			card.Notes = append(card.Notes, deckNote{Kind: "pre_review", Text: pr.Reasons[0]})
		}
	}
	if d, ok := decisions[decisionKey(sb.ItemName, sb.SourceID, r.Locale)]; ok && sb.SourceID != "" {
		switch d.ReviewState {
		case venue.ReviewStateApproved:
			facts.Stale = d.Basis != "" && sb.SourceRevision != "" && d.Basis != sb.SourceRevision
		case venue.ReviewStateRejected:
			facts.RejectedRedraft = d.Revision != "" && d.Revision != model.TargetRevision(b, loc)
		}
	}
	card.FindingsCount, card.FailingFindingsCount = facts.Findings, facts.FailingFindings
	card.Group, card.Reason = reviewdeck.ClassifyDeckCard(facts, policy)
	return card
}

// addTermNotes gives each card of a page the terms its source uses, with the
// renderings the terms store holds for the language: the same lookup the
// document surface's term marks come from. A workspace with no terms adds
// none.
func (s *Server) addTermNotes(ctx context.Context, ws string, proj *store.Project, loc string, page []deckCard) {
	if s.wsStores == nil || ws == "" || len(page) == 0 {
		return
	}
	tb, err := s.wsStores.getTerms(ctx, ws)
	if err != nil || tb == nil {
		return
	}
	if n, err := tb.Count(ctx); err != nil || n == 0 {
		return
	}
	for i := range page {
		b := page[i].block
		if b == nil {
			continue
		}
		matches, err := tb.LookupAll(ctx, b.SourceText(), terms.LookupOptions{
			SourceLocale: proj.DefaultSourceLanguage,
			TargetLocale: model.LocaleID(loc),
			ProjectID:    proj.ID,
		})
		if err != nil {
			continue
		}
		for _, m := range matches {
			var renderings []string
			for _, t := range m.Concept.UsableTerms(model.LocaleID(loc)) {
				renderings = append(renderings, fmt.Sprintf("%q", t.Text))
			}
			text := fmt.Sprintf("term %q", m.Term.Text)
			if len(renderings) > 0 {
				text += ": " + strings.Join(renderings, ", ")
			}
			page[i].Notes = append(page[i].Notes, deckNote{Kind: "term", Text: text})
		}
	}
}
