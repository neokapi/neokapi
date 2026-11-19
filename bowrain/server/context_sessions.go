package server

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	fwproject "github.com/neokapi/neokapi/core/project"
)

// A context session deals a person up to contextSessionSize cards from the
// project's one context queue and holds them for that person while it runs,
// so two people working at once never decide the same card. It runs until its
// owner ends it, or until contextSessionIdle passes with no decision.
const (
	contextSessionSize = 10
	contextSessionIdle = 30 * time.Minute
	// contextSessionReachBudget bounds the block counts a session start runs
	// for cards that carry no reach: one aggregate query each.
	contextSessionReachBudget = 30
	// contextSinceListed bounds the terms each "since" list names.
	contextSinceListed = 10
	deckCodeHeld       = "held"
)

// sessionNow is the clock sessions read; tests move it.
var sessionNow = time.Now

func (s *Server) registerContextSessionRoutes(g *echo.Group) {
	g.POST("/:id/context/sessions", s.refuseTool(s.HandleStartContextSession))
	g.POST("/:id/context/sessions/:sid/end", s.refuseTool(s.HandleEndContextSession))
}

// contextSessionCard is a context card as a session deals it: the deck's
// card, the channels its term appears in, and for a concept the target terms
// proposed for it in the reviewer's languages.
type contextSessionCard struct {
	contextDeckCard
	Channels  []string                 `json:"channels,omitempty"`
	Proposals []contextDeckTranslation `json:"proposals,omitempty"`
}

// contextSessionInfo is a session's id and times.
type contextSessionInfo struct {
	ID        string     `json:"id"`
	StartedAt time.Time  `json:"started_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

func sessionInfo(sess platstore.ContextSession) contextSessionInfo {
	out := contextSessionInfo{ID: sess.ID, StartedAt: sess.StartedAt, ExpiresAt: sess.ExpiresAt}
	if !sess.EndedAt.IsZero() {
		at := sess.EndedAt
		out.EndedAt = &at
	}
	return out
}

// contextSinceTerm is one term in the "since the last session" header.
type contextSinceTerm struct {
	Item   contextop.ItemRef `json:"item"`
	Term   string            `json:"term"`
	Blocks int               `json:"blocks"`
	// Before is the blocks the term reached when the last session started.
	Before int `json:"before,omitempty"`
}

// contextSessionSince is what changed in the project's context queue since
// the team's last session ended: candidates added, terms whose reach grew,
// and cards other people decided in their sessions.
type contextSessionSince struct {
	At time.Time `json:"at"`
	// By is who ran the last session.
	By string `json:"by,omitempty"`
	// AddedTotal and GrewTotal count every term; Added and Grew name the
	// first of them, widest reach first.
	AddedTotal int                `json:"added_total"`
	Added      []contextSinceTerm `json:"added"`
	GrewTotal  int                `json:"grew_total"`
	Grew       []contextSinceTerm `json:"grew"`
	// DecidedByOthers counts the decisions other people made in their
	// sessions since then.
	DecidedByOthers int `json:"decided_by_others"`
}

// contextSessionResponse is a session as it starts.
type contextSessionResponse struct {
	Session contextSessionInfo   `json:"session"`
	Bound   bool                 `json:"bound"`
	Cards   []contextSessionCard `json:"cards"`
	// Since is absent on a project's first session.
	Since *contextSessionSince `json:"since,omitempty"`
	// Waiting counts the cards the caller could decide that the session did
	// not deal, and HeldByOthers the cards other people's sessions hold.
	Waiting      int `json:"waiting"`
	HeldByOthers int `json:"held_by_others"`
}

// contextSessionSummary is what a session's decisions did. A figure the
// server cannot count from what it holds is left out.
type contextSessionSummary struct {
	Decided  int `json:"decided"`
	Kept     int `json:"kept"`
	Changed  int `json:"changed"`
	Rejected int `json:"rejected"`
	// Blocks is how many blocks use the terms kept or changed, as their
	// reach was counted when the session dealt them.
	Blocks *int `json:"blocks,omitempty"`
	// FindingsCleared is the source findings the term rules turned down
	// raised at the project's last check.
	FindingsCleared *int `json:"findings_cleared,omitempty"`
	// Waiting counts the cards still waiting for the caller, and Next names
	// the first of them.
	Waiting int    `json:"waiting"`
	Next    string `json:"next,omitempty"`
}

// contextSessionEndResponse is a session as it ends.
type contextSessionEndResponse struct {
	Session contextSessionInfo    `json:"session"`
	Summary contextSessionSummary `json:"summary"`
}

// contextSessionStore is the content store's session store.
func (s *Server) contextSessionStore() (platstore.ContextSessionStore, bool) {
	if s.ContentStore == nil {
		return nil, false
	}
	return platstore.ContextSessions(s.ContentStore)
}

// heldByOthers is the item keys live sessions of people other than userID
// hold, and the sessions of userID's own that hold a card.
func heldByOthers(holds []platstore.ContextSessionHold, userID string) (others map[string]bool, own []string) {
	others = map[string]bool{}
	for _, h := range holds {
		if h.UserID == userID {
			if !slices.Contains(own, h.SessionID) {
				own = append(own, h.SessionID)
			}
			continue
		}
		others[h.Item] = true
	}
	return others, own
}

// withoutHeld drops the cards another person's live session holds.
func (s *Server) withoutHeld(ctx context.Context, projectID, userID string, cards []contextDeckCard) ([]contextDeckCard, error) {
	css, ok := s.contextSessionStore()
	if !ok {
		return cards, nil
	}
	holds, err := css.ContextSessionHolds(ctx, projectID, sessionNow())
	if err != nil {
		return nil, err
	}
	others, _ := heldByOthers(holds, userID)
	if len(others) == 0 {
		return cards, nil
	}
	return slices.DeleteFunc(cards, func(c contextDeckCard) bool { return others[c.Item.Key()] }), nil
}

// actionable reports whether the reviewer can decide the card: keep or
// reject it.
func actionable(c contextDeckCard) bool {
	return slices.Contains(c.Actions, deckActionKeep) || slices.Contains(c.Actions, deckActionReject)
}

// HandleStartContextSession answers POST /:ws/:id/context/sessions: it deals
// the caller up to ten pending cards they can decide, widest reach first,
// skipping the cards other people's sessions hold, and holds them for the
// caller. A session the caller had running ends first. The answer carries
// what changed since the team's last session.
func (s *Server) HandleStartContextSession(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	css, ok := s.contextSessionStore()
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "context sessions are not available on this server"})
	}
	src, rv, err := s.deckRequest(c)
	if errors.Is(err, errNoProject) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	if err != nil {
		return serverErr(c, err)
	}
	ctx := c.Request().Context()
	now := sessionNow().UTC()
	projectID := src.project.ID

	holds, err := css.ContextSessionHolds(ctx, projectID, now)
	if err != nil {
		return serverErr(c, err)
	}
	others, own := heldByOthers(holds, rv.userID)
	for _, sid := range own {
		if err := css.EndContextSession(ctx, sid, now); err != nil {
			return serverErr(c, err)
		}
	}

	all, err := s.contextItems(ctx, src, rv)
	if err != nil {
		return serverErr(c, err)
	}
	reach := map[string]int{}
	var open []contextDeckCard
	for _, card := range all {
		if card.Kind == deckKindConcept && card.Reach != nil {
			reach[card.Item.ID] = card.Reach.Blocks
		}
		if !card.Reviewed && actionable(card) && !others[card.Item.Key()] {
			open = append(open, card)
		}
	}
	if src.key == "" {
		open = nil
	}
	s.countMissingReach(ctx, projectID, src.project.DefaultSourceLanguage, open)
	open = rankByReach(open)

	sess := platstore.ContextSession{
		ID: id.New(), ProjectID: projectID, UserID: rv.userID,
		Reach: reach, StartedAt: now, ExpiresAt: now.Add(contextSessionIdle),
	}
	keys := make([]string, len(open))
	for i, card := range open {
		keys[i] = card.Item.Key()
	}
	held, err := css.StartContextSession(ctx, sess, keys, contextSessionSize)
	if err != nil {
		return serverErr(c, err)
	}
	dealt := cardsByKeys(open, held)

	channels := s.collectionChannels(ctx, projectID)
	proposals := proposalsByConcept(all, rv)
	out := contextSessionResponse{
		Session:      sessionInfo(sess),
		Bound:        src.key != "",
		Cards:        make([]contextSessionCard, 0, len(dealt)),
		Waiting:      len(open) - len(dealt),
		HeldByOthers: len(others),
	}
	stored := make([]platstore.ContextSessionCard, 0, len(dealt))
	for _, card := range dealt {
		sc := contextSessionCard{contextDeckCard: card, Channels: cardChannels(card, channels)}
		if card.Kind == deckKindConcept {
			sc.Proposals = proposals[card.Item.ID]
		}
		out.Cards = append(out.Cards, sc)
		stored = append(stored, platstore.ContextSessionCard{
			Item: card.Item.Key(), Kind: card.Kind, Term: cardLabel(card), Concept: cardConcept(card), Blocks: card.reachBlocks(),
		})
	}
	if err := css.SetContextSessionCards(ctx, sess.ID, stored); err != nil {
		return serverErr(c, err)
	}

	since, err := s.sessionSince(ctx, css, sess, rv.userID, all)
	if err != nil {
		return serverErr(c, err)
	}
	out.Since = since
	return c.JSON(http.StatusOK, out)
}

// countMissingReach counts, for the cards that carry no reach, the source
// blocks that hold one of the card's words, one aggregate query per card, up
// to contextSessionReachBudget of them in deck order. A candidate term
// carries the reach its scan counted and is not counted again.
func (s *Server) countMissingReach(ctx context.Context, projectID string, source model.LocaleID, cards []contextDeckCard) {
	budget := contextSessionReachBudget
	for i := range cards {
		if budget == 0 {
			return
		}
		if cards[i].Reach != nil {
			continue
		}
		words := reachWords(cards[i], source)
		if len(words) == 0 {
			continue
		}
		budget--
		counts, err := s.ContentStore.CountBlocks(ctx, platstore.BlockQuery{
			ProjectID: projectID, Stream: reviewInboxStream, TextAny: words,
		})
		if err != nil {
			continue
		}
		cards[i].Reach = &contextDeckReach{Blocks: counts.Total, Collections: []contextDeckCollectionReach{}}
	}
}

// reachWords are the words whose blocks a card governs: a concept's terms in
// the source language that are in use, a term rule's term and its forms, a
// voice rule's words when its pattern is a short list of them.
func reachWords(card contextDeckCard, source model.LocaleID) []string {
	var out []string
	switch {
	case card.Kind == deckKindConcept:
		for _, t := range card.Terms {
			if sameLanguage(model.LocaleID(t.Locale), source) && t.Status != string(model.TermForbidden) && t.Status != string(model.TermDeprecated) {
				out = append(out, t.Text)
			}
		}
	case card.Term != nil:
		out = append(out, card.Term.Term)
		out = append(out, card.Term.Forms...)
	case card.Pattern != nil:
		out = append(out, card.Pattern.Words...)
	}
	return slices.DeleteFunc(out, func(w string) bool { return strings.TrimSpace(w) == "" })
}

// rankByReach orders cards by reach, widest first, keeping each proposed
// translation right after its concept card. A concept and the translations
// that follow it rank as one group, by the concept's reach; cards of equal
// reach keep the deck's order.
func rankByReach(cards []contextDeckCard) []contextDeckCard {
	type group struct {
		cards []contextDeckCard
		reach int
	}
	var groups []group
	for _, card := range cards {
		if cid, _, ok := parseTermProposalItem(card.Item); ok && len(groups) > 0 {
			last := &groups[len(groups)-1]
			if last.cards[0].Item.Kind == contextop.ItemConcept && last.cards[0].Item.ID == cid {
				last.cards = append(last.cards, card)
				continue
			}
		}
		groups = append(groups, group{cards: []contextDeckCard{card}, reach: card.reachBlocks()})
	}
	slices.SortStableFunc(groups, func(a, b group) int { return cmp.Compare(b.reach, a.reach) })
	out := make([]contextDeckCard, 0, len(cards))
	for _, g := range groups {
		out = append(out, g.cards...)
	}
	return out
}

// cardsByKeys are the cards keys names, in the cards' order.
func cardsByKeys(cards []contextDeckCard, keys []string) []contextDeckCard {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	var out []contextDeckCard
	for _, card := range cards {
		if want[card.Item.Key()] {
			out = append(out, card)
		}
	}
	return out
}

// collectionChannels maps each collection of the project to the channel its
// point declares.
func (s *Server) collectionChannels(ctx context.Context, projectID string) map[string]string {
	out := map[string]string{}
	cols, err := s.ContentStore.ListCollections(ctx, projectID, reviewInboxStream)
	if err != nil {
		return out
	}
	for _, col := range cols {
		if ch := col.Context[fwproject.ChannelAxis]; ch != "" {
			out[col.Name] = ch
		}
	}
	return out
}

// cardChannels are the channels a card's term appears in: those of the
// collections its reach names, widest first, or the channel the card is
// scoped to.
func cardChannels(card contextDeckCard, channels map[string]string) []string {
	var out []string
	if card.Reach != nil {
		for _, cc := range card.Reach.Collections {
			if ch := channels[cc.Name]; ch != "" && cc.Blocks > 0 && !slices.Contains(out, ch) {
				out = append(out, ch)
			}
		}
	}
	if len(out) == 0 {
		if ch := card.Where.Coordinates[fwproject.ChannelAxis]; ch != "" {
			out = append(out, ch)
		}
	}
	return out
}

// proposalsByConcept are the target terms proposed for each concept, in the
// languages the reviewer may review.
func proposalsByConcept(cards []contextDeckCard, rv deckReviewer) map[string][]contextDeckTranslation {
	out := map[string][]contextDeckTranslation{}
	for _, card := range cards {
		cid, _, ok := parseTermProposalItem(card.Item)
		if !ok || card.Translation == nil || card.Reviewed || !rv.mayReviewLanguage(card.Translation.Locale) {
			continue
		}
		out[cid] = append(out[cid], *card.Translation)
	}
	return out
}

// cardLabel is the words that name a card to a person: its term, or what its
// rule or note says.
func cardLabel(card contextDeckCard) string {
	switch {
	case card.Translation != nil:
		return card.Translation.Term
	case len(card.Terms) > 0:
		return card.Terms[0].Text
	case card.Term != nil:
		return card.Term.Term
	case card.Pattern != nil:
		if card.Pattern.Description != "" {
			return card.Pattern.Description
		}
		if len(card.Pattern.Words) > 0 {
			return strings.Join(card.Pattern.Words, ", ")
		}
		return card.Pattern.Regex
	}
	label := []rune(strings.TrimSpace(card.Text))
	if len(label) > 60 {
		return string(label[:59]) + "…"
	}
	return string(label)
}

// cardConcept is the concept a concept card or a term-translation card
// belongs to.
func cardConcept(card contextDeckCard) string {
	if card.Item.Kind == contextop.ItemConcept {
		return card.Item.ID
	}
	if cid, _, ok := parseTermProposalItem(card.Item); ok {
		return cid
	}
	return ""
}

// sessionSince is what changed since the project's last ended session: nil
// when the project has none.
func (s *Server) sessionSince(ctx context.Context, css platstore.ContextSessionStore, sess platstore.ContextSession, userID string, all []contextDeckCard) (*contextSessionSince, error) {
	last, ok, err := css.LastEndedContextSession(ctx, sess.ProjectID, sess.StartedAt)
	if err != nil || !ok {
		return nil, err
	}
	out := &contextSessionSince{At: last.EndedAt, Added: []contextSinceTerm{}, Grew: []contextSinceTerm{}}
	if last.UserID != userID {
		out.By = s.displayName(ctx, last.UserID)
	}
	for _, card := range all {
		if card.Kind != deckKindConcept {
			continue
		}
		term := contextSinceTerm{Item: card.Item, Term: cardLabel(card), Blocks: card.reachBlocks()}
		if card.Proposed && !card.Reviewed && card.At.After(last.EndedAt) {
			out.Added = append(out.Added, term)
			continue
		}
		if before, had := last.Reach[card.Item.ID]; had && card.Reach != nil && card.Reach.Blocks > before {
			term.Before = before
			out.Grew = append(out.Grew, term)
		}
	}
	slices.SortStableFunc(out.Added, func(a, b contextSinceTerm) int { return cmp.Compare(b.Blocks, a.Blocks) })
	slices.SortStableFunc(out.Grew, func(a, b contextSinceTerm) int {
		return cmp.Compare(b.Blocks-b.Before, a.Blocks-a.Before)
	})
	out.AddedTotal, out.GrewTotal = len(out.Added), len(out.Grew)
	out.Added = out.Added[:min(len(out.Added), contextSinceListed)]
	out.Grew = out.Grew[:min(len(out.Grew), contextSinceListed)]

	sessions, err := css.ContextSessionsSince(ctx, sess.ProjectID, last.EndedAt)
	if err != nil {
		return nil, err
	}
	for _, other := range sessions {
		if other.UserID == userID {
			continue
		}
		for _, d := range other.Decisions {
			if d.At.After(last.EndedAt) {
				out.DecidedByOthers++
			}
		}
	}
	return out, nil
}

// sessionHolding is the caller's live session that holds item, or "" when
// none does; held reports that another person's live session holds it.
func (s *Server) sessionHolding(ctx context.Context, css platstore.ContextSessionStore, projectID, userID, item string) (own string, held bool, err error) {
	holds, err := css.ContextSessionHolds(ctx, projectID, sessionNow())
	if err != nil {
		return "", false, err
	}
	for _, h := range holds {
		if h.Item != item {
			continue
		}
		if h.UserID == userID {
			return h.SessionID, false, nil
		}
		return "", true, nil
	}
	return "", false, nil
}

// sessionOutcome is what a session records for a decision that succeeded.
func sessionOutcome(req contextDeckDecideRequest) string {
	switch {
	case req.Action == deckActionReject:
		return platstore.SessionRejected
	case strings.TrimSpace(req.Text) != "":
		return platstore.SessionChanged
	default:
		return platstore.SessionKept
	}
}

// HandleEndContextSession answers POST /:ws/:id/context/sessions/:sid/end:
// it ends the caller's session, releases the cards it still held, and
// answers with what its decisions did. Ending a session that ended already,
// or expired, answers its summary again.
func (s *Server) HandleEndContextSession(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	css, ok := s.contextSessionStore()
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "context sessions are not available on this server"})
	}
	ctx := c.Request().Context()
	sess, ok, err := css.GetContextSession(ctx, c.Param("sid"))
	if err != nil {
		return serverErr(c, err)
	}
	userID := ledgerMemberOf(c).UserID
	if !ok || sess.ProjectID != projectParam(c) || sess.UserID != userID {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "no session of yours in this project has that id"})
	}
	if sess.EndedAt.IsZero() {
		if err := css.EndContextSession(ctx, sess.ID, sessionNow()); err != nil {
			return serverErr(c, err)
		}
		if sess, _, err = css.GetContextSession(ctx, sess.ID); err != nil {
			return serverErr(c, err)
		}
	}
	summary := summarizeSession(sess)
	if summary.Rejected > 0 {
		s.countFindingsCleared(ctx, sess, &summary)
	}
	src, rv, err := s.deckRequest(c)
	if err != nil && !errors.Is(err, errNoProject) {
		return serverErr(c, err)
	}
	if err == nil {
		cards, err := s.pendingContext(ctx, src, rv)
		if err != nil {
			return serverErr(c, err)
		}
		if cards, err = s.withoutHeld(ctx, src.project.ID, rv.userID, cards); err != nil {
			return serverErr(c, err)
		}
		cards = slices.DeleteFunc(cards, func(card contextDeckCard) bool { return !actionable(card) })
		summary.Waiting = len(cards)
		if len(cards) > 0 {
			summary.Next = cardLabel(rankByReach(cards)[0])
		}
	}
	return c.JSON(http.StatusOK, contextSessionEndResponse{Session: sessionInfo(sess), Summary: summary})
}

// summarizeSession counts a session's decisions, and the blocks that use the
// terms kept or changed in it, each concept once.
func summarizeSession(sess platstore.ContextSession) contextSessionSummary {
	var out contextSessionSummary
	cards := map[string]platstore.ContextSessionCard{}
	for _, card := range sess.Cards {
		cards[card.Item] = card
	}
	blocks, counted := 0, false
	seen := map[string]bool{}
	for _, d := range sess.Decisions {
		out.Decided++
		switch d.Outcome {
		case platstore.SessionKept:
			out.Kept++
		case platstore.SessionChanged:
			out.Changed++
		case platstore.SessionRejected:
			out.Rejected++
			continue
		default:
			continue
		}
		card, ok := cards[d.Item]
		if !ok || card.Blocks == 0 {
			continue
		}
		key := cmp.Or(card.Concept, card.Item)
		if seen[key] {
			continue
		}
		seen[key] = true
		blocks += card.Blocks
		counted = true
	}
	if counted {
		out.Blocks = &blocks
	}
	return out
}

// countFindingsCleared adds up the source findings the project's last check
// counted for the term rules the session turned down: each such finding goes
// with its rule. The figure is left out when the project has no check tally.
func (s *Server) countFindingsCleared(ctx context.Context, sess platstore.ContextSession, out *contextSessionSummary) {
	rejected := map[string]bool{}
	cards := map[string]platstore.ContextSessionCard{}
	for _, card := range sess.Cards {
		cards[card.Item] = card
	}
	for _, d := range sess.Decisions {
		if card, ok := cards[d.Item]; ok && d.Outcome == platstore.SessionRejected && card.Kind == deckKindTermRule && card.Term != "" {
			rejected[strings.ToLower(card.Term)] = true
		}
	}
	if len(rejected) == 0 {
		return
	}
	tallies, ok := platstore.ItemCheckTallies(s.ContentStore)
	if !ok {
		return
	}
	rows, err := tallies.ListItemCheckTallies(ctx, sess.ProjectID, reviewInboxStream)
	if err != nil || len(rows) == 0 {
		return
	}
	cleared := 0
	for _, row := range rows {
		for _, rule := range row.Rules {
			if strings.HasPrefix(rule.Rule, "terms.") && rejected[strings.ToLower(rule.Subject)] {
				cleared += rule.Count
			}
		}
	}
	out.FindingsCleared = &cleared
}
