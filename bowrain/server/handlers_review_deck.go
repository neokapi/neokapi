package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
//
// A translation's group is computed once and kept (store.ReviewDeckFacts)
// until a row it was read from changes. A request classifies what is new in
// bounded batches, then reads, orders and counts the deck in SQL, and reads
// blocks whole only for the page it returns.

const (
	// deckClassifyBatch is how many translations one classification pass
	// reads whole.
	deckClassifyBatch = 200
	// deckClassifyBudget bounds the translations one request classifies. What
	// is left is counted as unclassified and classified by the next request.
	deckClassifyBudget = 2000
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
}

type deckCounts struct {
	Fast   int `json:"fast"`
	Closer int `json:"closer"`
	Desk   int `json:"desk"`
	Total  int `json:"total"`
	// Unclassified counts the translations waiting for review that this
	// request did not reach; the next request classifies them.
	Unclassified int `json:"unclassified,omitempty"`
}

type reviewDeckResponse struct {
	Locale     string                `json:"locale"`
	Cards      []deckCard            `json:"cards"`
	Counts     deckCounts            `json:"counts"`
	NextCursor string                `json:"next_cursor"`
	Policy     reviewdeck.DeckPolicy `json:"policy"`
}

// encodeDeckCursor is the opaque form of a deck position.
func encodeDeckCursor(k store.ReviewDeckKey) string {
	raw, _ := json.Marshal([]any{k.GroupRank, k.ItemName, k.Position, k.BlockID})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeDeckCursor(s string) (store.ReviewDeckKey, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return store.ReviewDeckKey{}, err
	}
	var parts []any
	if err := json.Unmarshal(raw, &parts); err != nil || len(parts) != 4 {
		return store.ReviewDeckKey{}, errors.New("malformed cursor")
	}
	rank, ok1 := parts[0].(float64)
	item, ok2 := parts[1].(string)
	pos, ok3 := parts[2].(float64)
	bid, ok4 := parts[3].(string)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return store.ReviewDeckKey{}, errors.New("malformed cursor")
	}
	return store.ReviewDeckKey{GroupRank: int(rank), ItemName: item, Position: int(pos), BlockID: bid}, nil
}

// deckPolicy is the policy the server's deck applies.
func (s *Server) deckPolicy() reviewdeck.DeckPolicy {
	return s.ReviewDeckPolicy.WithDefaults()
}

// deckPolicyKey names a policy in the facts computed under it.
func deckPolicyKey(p reviewdeck.DeckPolicy) string {
	return fmt.Sprintf("long=%d;fast=%d", p.LongSourceChars, p.FastMinScore)
}

func deckRank(g reviewdeck.DeckGroup) int { return slices.Index(reviewdeck.DeckGroups, g) }

// HandleGetReviewDeck answers the review deck for one language.
//
// GET /:ws/:id/review-deck/:ref?locale=nb&limit=50&cursor=&group=&collection=
func (s *Server) HandleGetReviewDeck(c echo.Context) error {
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
	limit, _ := pageParams(c, 50, 200)
	rank := -1
	if g := strings.TrimSpace(c.QueryParam("group")); g != "" {
		if rank = deckRank(reviewdeck.DeckGroup(g)); rank < 0 {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "group is fast, closer or desk"})
		}
	}
	var after *store.ReviewDeckKey
	if raw := strings.TrimSpace(c.QueryParam("cursor")); raw != "" {
		key, err := decodeDeckCursor(raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cursor is not one this endpoint returned"})
		}
		after = &key
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
	if c.QueryParams().Has("collection") {
		scope := strings.TrimSpace(c.QueryParam("collection"))
		q.CollectionID = &scope
	}

	if err := s.classifyDeck(ctx, c, decks, proj, q, policy); err != nil {
		return serverErr(c, err)
	}
	counts, err := decks.CountReviewDeck(ctx, q)
	if err != nil {
		return serverErr(c, err)
	}
	rows, err := decks.ListReviewDeck(ctx, q, rank, after, limit+1)
	if err != nil {
		return serverErr(c, err)
	}

	out := reviewDeckResponse{Locale: loc, Cards: []deckCard{}, Policy: policy}
	out.Counts.Fast = counts.ByRank[deckRank(reviewdeck.DeckFast)]
	out.Counts.Closer = counts.ByRank[deckRank(reviewdeck.DeckCloser)]
	out.Counts.Desk = counts.ByRank[deckRank(reviewdeck.DeckDesk)]
	out.Counts.Total = out.Counts.Fast + out.Counts.Closer + out.Counts.Desk
	out.Counts.Unclassified = counts.Unclassified
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		out.NextCursor = encodeDeckCursor(store.ReviewDeckKey{
			GroupRank: last.Facts.GroupRank, ItemName: last.ItemName, Position: last.Position, BlockID: last.BlockID,
		})
	}
	cards, err := s.deckPage(ctx, c, proj, stream, loc, rows)
	if err != nil {
		return serverErr(c, err)
	}
	out.Cards = cards
	return c.JSON(http.StatusOK, out)
}

// classifyDeck computes the facts of the deck's translations that have none,
// or whose facts no longer hold, deckClassifyBatch at a time and at most
// deckClassifyBudget in one request. Only each batch's blocks are read whole.
func (s *Server) classifyDeck(ctx context.Context, c echo.Context, decks store.ReviewDeckStore, proj *store.Project, q store.ReviewDeckQuery, policy reviewdeck.DeckPolicy) error {
	targetLoc := model.LocaleID(q.Locale)
	var (
		gate   *termGate
		scored map[string]scoredBlock
	)
	for done := 0; done < deckClassifyBudget; {
		entries, err := decks.UnclassifiedReview(ctx, q, min(deckClassifyBatch, deckClassifyBudget-done))
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return nil
		}
		if gate == nil {
			wsID, _ := c.Get("workspace_id").(string)
			gate = s.resolveTermGate(ctx, proj, q.Stream, wsID)
			scored = latestVoiceScores(ctx, s.VoiceStore, proj.ID, q.Stream)[string(locale.Normalize(targetLoc))]
		}
		ids := make([]string, 0, len(entries))
		for _, e := range entries {
			if !slices.Contains(ids, e.BlockID) {
				ids = append(ids, e.BlockID)
			}
		}
		byID, err := s.deckBlocks(ctx, proj.ID, q.Stream, ids)
		if err != nil {
			return err
		}
		preReviews, err := s.ContentStore.PreReviews(ctx, proj.ID, q.Stream, ids)
		if err != nil {
			slog.WarnContext(ctx, "review deck: read pre-reviews failed", "project", proj.ID, "error", err)
		}
		facts := make([]store.ReviewDeckFacts, 0, len(entries))
		for _, e := range entries {
			sb := byID[e.BlockID]
			if sb == nil {
				continue
			}
			facts = append(facts, deckFactsFor(ctx, sb, e, targetLoc, gate, scored, preReviews, policy, q.Policy))
		}
		if err := decks.RecordReviewDeckFacts(ctx, proj.ID, q.Stream, facts); err != nil {
			return err
		}
		done += len(entries)
		if len(entries) < deckClassifyBatch {
			return nil
		}
	}
	return nil
}

// deckBlocks reads the named blocks whole, keyed by id.
func (s *Server) deckBlocks(ctx context.Context, pid, stream string, ids []string) (map[string]*venue.StoredBlock, error) {
	out := make(map[string]*venue.StoredBlock, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	stored, err := s.ContentStore.GetBlocks(ctx, store.BlockQuery{ProjectID: pid, Stream: stream, IDs: ids})
	if err != nil {
		return nil, err
	}
	for _, sb := range stored {
		if sb.Block != nil {
			out[sb.Block.ID] = sb
		}
	}
	return out, nil
}

// deckFactsFor computes one translation's facts and places it in its group:
// the rule-based checks approve-passing applies, the terminology and voice
// bars, the pre-review score, and what the ledger says about the source and
// an earlier rejection.
func deckFactsFor(
	ctx context.Context,
	sb *venue.StoredBlock,
	e store.ReviewDeckEntry,
	loc model.LocaleID,
	gate *termGate,
	scored map[string]scoredBlock,
	preReviews []store.PreReview,
	policy reviewdeck.DeckPolicy,
	policyKey string,
) store.ReviewDeckFacts {
	b := sb.Block
	facts := reviewdeck.DeckFacts{SourceChars: utf8.RuneCountInString(model.RunsText(b.SourceRuns()))}
	out := store.ReviewDeckFacts{BlockID: e.BlockID, Locale: e.Locale, Basis: e.Basis, Policy: policyKey}

	if issues, err := runChecksOnBlock(ctx, b, pointChecks{TargetLocale: loc}); err == nil {
		for _, issue := range issues {
			facts.Findings++
			if issue.Severity == "error" {
				facts.FailingFindings++
			}
		}
	}
	out.TermCompliance = gate.compliance(ctx, b, loc)
	if out.TermCompliance == store.TermComplianceViolation {
		facts.Findings++
		facts.FailingFindings++
	}
	switch gate.voiceCompliance(ctx, b.ID, loc, scored) {
	case store.VoiceCompliancePassing, store.VoiceComplianceBelowBar:
		vs := scored[b.ID]
		score, bar := vs.score, vs.bar
		out.VoiceScore, out.VoiceBar = &score, &bar
		if score < bar {
			facts.Findings++
		}
	case store.VoiceComplianceUnchecked:
		bar := gate.profileFor(ctx, loc).ComplianceBar()
		out.VoiceBar = &bar
	}
	if pr := freshPreReview(preReviews, sb, loc); pr != nil {
		score := pr.Score
		facts.Score = &score
	}
	switch e.DecisionState {
	case venue.ReviewStateApproved:
		facts.Stale = e.DecisionBasis != "" && sb.SourceRevision != "" && e.DecisionBasis != sb.SourceRevision
	case venue.ReviewStateRejected:
		facts.RejectedRedraft = e.DecisionRevision != "" && e.DecisionRevision != model.TargetRevision(b, loc)
	}
	group, reason := reviewdeck.ClassifyDeckCard(facts, policy)
	out.Group, out.GroupRank, out.Reason = string(group), deckRank(group), reason
	out.Findings, out.Failing = facts.Findings, facts.FailingFindings
	return out
}

// deckPage builds the cards of one page: the only blocks a deck request reads
// whole besides the ones it classifies.
func (s *Server) deckPage(ctx context.Context, c echo.Context, proj *store.Project, stream, loc string, rows []store.ReviewDeckRow) ([]deckCard, error) {
	if len(rows) == 0 {
		return []deckCard{}, nil
	}
	targetLoc := model.LocaleID(loc)
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.BlockID)
	}
	byID, err := s.deckBlocks(ctx, proj.ID, stream, ids)
	if err != nil {
		return nil, err
	}
	preReviews, err := s.ContentStore.PreReviews(ctx, proj.ID, stream, ids)
	if err != nil {
		slog.WarnContext(ctx, "review deck: read pre-reviews failed", "project", proj.ID, "error", err)
	}
	var wrote map[store.TargetRef]string
	actor, _ := c.Get("user_id").(string)
	if authors, ok := s.ContentStore.(store.TargetAuthorStore); ok && actor != "" {
		if wrote, err = authors.LastTargetAuthors(ctx, proj.ID, stream, ids, []string{loc}); err != nil {
			slog.WarnContext(ctx, "review deck: read target authors failed", "project", proj.ID, "error", err)
		}
	}
	tb := s.deckTerms(ctx, c.Param("ws"))

	cards := make([]deckCard, 0, len(rows))
	for _, r := range rows {
		sb := byID[r.BlockID]
		if sb == nil {
			continue
		}
		b := sb.Block
		target, _ := b.TargetEdition(targetLoc)
		card := deckCard{
			BlockID:              b.ID,
			ItemName:             r.ItemName,
			Key:                  b.Name,
			CollectionID:         r.CollectionID,
			Locale:               r.Locale,
			SourceRuns:           nonNilRuns(b.SourceRuns()),
			TargetRuns:           nonNilRuns(target.Runs),
			SourceText:           model.RenderRunsWithData(b.SourceRuns()),
			TargetText:           model.RenderRunsWithData(target.Runs),
			TargetEditText:       model.RunsEditText(target.Runs),
			SourceRevision:       check.SourceRevision(b, proj.DefaultSourceLanguage),
			TargetRevision:       store.TargetRevision(sb, targetLoc),
			TargetStatus:         string(target.Status),
			FindingsCount:        r.Facts.Findings,
			FailingFindingsCount: r.Facts.Failing,
			TermCompliance:       r.Facts.TermCompliance,
			VoiceScore:           r.Facts.VoiceScore,
			VoiceBar:             r.Facts.VoiceBar,
			Notes:                []deckNote{},
			OwnWork:              actor != "" && wrote[store.TargetRef{BlockID: r.BlockID, Locale: r.Locale}] == actor,
			Group:                reviewdeck.DeckGroup(r.Facts.Group),
			Reason:               r.Facts.Reason,
		}
		card.Notes = s.deckNotes(ctx, b, targetLoc, r.Facts, proj, tb)
		if pr := freshPreReview(preReviews, sb, targetLoc); pr != nil {
			card.PreReview = preReviewViewOf(pr)
			if len(pr.Reasons) > 0 && card.Group != reviewdeck.DeckFast {
				card.Notes = append(card.Notes, deckNote{Kind: "pre_review", Text: pr.Reasons[0]})
			}
		}
		cards = append(cards, card)
	}
	return cards, nil
}

func nonNilRuns(runs []model.Run) []model.Run {
	if runs == nil {
		return []model.Run{}
	}
	return runs
}

// deckNotes are the lines a page card carries: the check findings, the
// terminology and voice verdicts its facts hold, and the terms its source
// uses with the renderings the terms store holds for the language.
func (s *Server) deckNotes(ctx context.Context, b *model.Block, loc model.LocaleID, facts store.ReviewDeckFacts, proj *store.Project, tb terms.Terminology) []deckNote {
	notes := []deckNote{}
	if facts.Findings > 0 {
		if issues, err := runChecksOnBlock(ctx, b, pointChecks{TargetLocale: loc}); err == nil {
			for _, issue := range issues {
				if len(notes) == deckNoteLimit {
					break
				}
				notes = append(notes, deckNote{Kind: "check", Text: issue.Message})
			}
		}
	}
	if facts.TermCompliance == store.TermComplianceViolation {
		notes = append(notes, deckNote{Kind: "term", Text: "uses a term the terms store rejects, or misses one it requires"})
	}
	if facts.VoiceScore != nil && facts.VoiceBar != nil && *facts.VoiceScore < *facts.VoiceBar {
		notes = append(notes, deckNote{Kind: "voice", Text: fmt.Sprintf("voice score %d, under the bar of %d", *facts.VoiceScore, *facts.VoiceBar)})
	}
	if tb == nil {
		return notes
	}
	matches, err := tb.LookupAll(ctx, b.SourceText(), terms.LookupOptions{
		SourceLocale: proj.DefaultSourceLanguage, TargetLocale: loc, ProjectID: proj.ID,
	})
	if err != nil {
		return notes
	}
	for _, m := range matches {
		var renderings []string
		for _, t := range m.Concept.UsableTerms(loc) {
			renderings = append(renderings, fmt.Sprintf("%q", t.Text))
		}
		text := fmt.Sprintf("term %q", m.Term.Text)
		if len(renderings) > 0 {
			text += ": " + strings.Join(renderings, ", ")
		}
		notes = append(notes, deckNote{Kind: "term", Text: text})
	}
	return notes
}

// deckTerms is the workspace's terms store when it holds terms, else nil.
func (s *Server) deckTerms(ctx context.Context, ws string) terms.Terminology {
	if s.wsStores == nil || ws == "" {
		return nil
	}
	tb, err := s.wsStores.getTerms(ctx, ws)
	if err != nil || tb == nil {
		return nil
	}
	if n, err := tb.Count(ctx); err != nil || n == 0 {
		return nil
	}
	return tb
}
