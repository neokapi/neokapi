package server

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	"github.com/neokapi/neokapi/bowrain/voicewrite"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/terms"
)

// The kinds of card the context deck holds.
const (
	deckKindVoiceRule = "voice_rule"
	deckKindConcept   = "concept"
	deckKindTermRule  = "term_rule"
	deckKindNote      = "note"
)

// deckKindOrder is the order the deck deals its kinds in: the voice first,
// then the terms, then the rules and notes recorded in the ledger. A
// term-translation card is dealt with the candidate terms or after its
// concept (contextDeckRank, followConcepts), and is counted by kind only
// when the deck holds one.
var deckKindOrder = []string{deckKindVoiceRule, deckKindConcept, deckKindTermRule, deckKindNote}

// The actions a context card offers.
const (
	deckActionKeep   = "keep"
	deckActionReject = "reject"
	deckActionLater  = "later"
)

// contextDeckCard is one context item waiting for a person to review it at
// its current revision.
type contextDeckCard struct {
	Item     contextop.ItemRef `json:"item"`
	Kind     string            `json:"kind"`
	Revision string            `json:"revision"`
	// Status is the ledger status of a term rule or a note: suggested,
	// established or contested.
	Status string `json:"status,omitempty"`
	// Term is a term rule: the form to avoid, its other forms and what to
	// write instead.
	Term *contextDeckTermRule `json:"term,omitempty"`
	// Terms are a concept's terms, the project's source language first.
	Terms []contextDeckConceptTerm `json:"terms,omitempty"`
	// Pattern is a voice rule.
	Pattern *contextDeckPattern `json:"pattern,omitempty"`
	// Text is a note's prose or a concept's definition.
	Text string `json:"text,omitempty"`
	// Language is the language the item governs, when it names one.
	Language string `json:"language,omitempty"`
	// Flags are fails, advisory, competitor and do_not_translate.
	Flags  []string          `json:"flags,omitempty"`
	Where  contextDeckWhere  `json:"where"`
	Author contextDeckAuthor `json:"author"`
	At     time.Time         `json:"at,omitzero"`
	// Review is an earlier attestation of an older revision: the item
	// changed since it was reviewed.
	Review *contextop.Review `json:"review,omitempty"`
	// OwnWork says the reviewer wrote the item by hand.
	OwnWork bool `json:"own_work"`
	// Basis is what a keep would record.
	Basis   contextop.ReviewBasis `json:"basis,omitempty"`
	Actions []string              `json:"actions"`
	// Origin is the bulk operation that brought the item in or last changed
	// it: an import of a context file, or a whole voice-profile write.
	Origin *contextDeckOrigin `json:"origin,omitempty"`
	// Reviewed says an attestation covers the item at its current revision.
	// Only a listing of an origin's items deals reviewed ones.
	Reviewed bool `json:"reviewed,omitempty"`
	// Proposed says the card is a candidate term: a concept whose
	// source-language term is proposed. Keep makes it a term of the project
	// (its proposed term becomes preferred); reject deletes it and records
	// that it is not a term, so no later scan proposes it again.
	Proposed bool `json:"proposed,omitempty"`
	// Reach is how many blocks of the project use a candidate, as the scan
	// that filed it counted them.
	Reach *contextDeckReach `json:"reach,omitempty"`
	// Translation is the target term a term-translation card proposes.
	Translation *contextDeckTranslation `json:"translation,omitempty"`
}

// contextDeckReach is the blocks that use a candidate term, in all and per
// collection, most first.
type contextDeckReach struct {
	Blocks      int                          `json:"blocks"`
	Collections []contextDeckCollectionReach `json:"collections"`
}

type contextDeckCollectionReach struct {
	Name   string `json:"name"`
	Blocks int    `json:"blocks"`
}

// reachBlocks is a card's reach, zero when it carries none.
func (card contextDeckCard) reachBlocks() int {
	if card.Reach == nil {
		return 0
	}
	return card.Reach.Blocks
}

// contextDeckRank is where a card's group sits in the deck: candidate terms
// and proposed translations of terms first, because a term decided once
// governs every block that uses it, then the kinds in deckKindOrder.
func contextDeckRank(card contextDeckCard) int {
	if card.Proposed || card.Kind == deckKindTermTranslation {
		return -1
	}
	return slices.Index(deckKindOrder, card.Kind)
}

// The kinds of bulk operation an origin names.
const (
	originKindImport    = "import"
	originKindVoiceEdit = "voice_edit"
)

// contextDeckOrigin is the bulk operation that brought an item in: who ran
// it, from where, when, and how many items it carried. The deck deals the
// items, never the operation, because keeping the operation would vouch for
// every item it carried unseen.
type contextDeckOrigin struct {
	// ID is the operation's id, which the origin listing takes.
	ID    string `json:"id"`
	Short string `json:"short"`
	// Kind is import or voice_edit.
	Kind string `json:"kind"`
	// Source is what an import read: terms, memory, voice or decisions.
	Source string `json:"source,omitempty"`
	// Path is the file an import read.
	Path string `json:"path,omitempty"`
	// Profile is the voice profile a voice edit wrote.
	Profile string `json:"profile,omitempty"`
	// Count is how many items an import carried, or how many fields a voice
	// edit changed. Zero when the operation did not record it.
	Count  int               `json:"count,omitempty"`
	Author contextDeckAuthor `json:"author"`
	At     time.Time         `json:"at"`
	// Note is what the person said about the operation.
	Note string `json:"note,omitempty"`
}

type contextDeckTermRule struct {
	Term        string   `json:"term"`
	Replacement string   `json:"replacement,omitempty"`
	Forms       []string `json:"forms,omitempty"`
}

type contextDeckConceptTerm struct {
	Text   string `json:"text"`
	Locale string `json:"locale"`
	Status string `json:"status,omitempty"`
}

type contextDeckPattern struct {
	// List is prohibited or required.
	List        string `json:"list"`
	Regex       string `json:"regex"`
	Description string `json:"description,omitempty"`
	// Rate is the ceiling, as "count per words", when the rule is one.
	Rate string `json:"rate,omitempty"`
	// Profile is the voice profile's name.
	Profile string `json:"profile,omitempty"`
	// Words are the words and phrases the regex matches, when it is a short
	// list of them (profile.PatternWords). A reader can judge those without
	// reading the regex.
	Words []string `json:"words,omitempty"`
}

// contextDeckWhere is where an item applies.
type contextDeckWhere struct {
	// Level is project or workspace.
	Level       string            `json:"level"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	ValidFrom   *time.Time        `json:"valid_from,omitempty"`
	ValidTo     *time.Time        `json:"valid_to,omitempty"`
}

// contextDeckAuthor is who wrote an item, as its store recorded it.
type contextDeckAuthor struct {
	// Kind is person, agent or tool.
	Kind    string `json:"kind"`
	Name    string `json:"name,omitempty"`
	Session string `json:"session,omitempty"`
	// Host is the agent host a person recorded the item from, read from the
	// operation's note ("recorded as a person in a claude-code shell").
	Host string `json:"host,omitempty"`
	// OnBehalfOf names the member an agent or a tool ran for, when the store
	// records it: the member who issued the token a tool pushed with.
	OnBehalfOf string `json:"on_behalf_of,omitempty"`
	// AgentAuthored says an agent or a tool wrote the item, or a person in an
	// agent host's shell.
	AgentAuthored bool `json:"agent_authored"`
	// userID is the member the store names, when it names one by id.
	userID string
}

// contextDeckCounts counts the cards by kind and by who wrote them.
// Proposed counts the candidate terms among the concepts.
type contextDeckCounts struct {
	Total    int            `json:"total"`
	ByKind   map[string]int `json:"by_kind"`
	Proposed int            `json:"proposed"`
	Agent    int            `json:"agent"`
	Person   int            `json:"person"`
}

func countDeck(cards []contextDeckCard) contextDeckCounts {
	out := contextDeckCounts{ByKind: map[string]int{}}
	for _, k := range deckKindOrder {
		out.ByKind[k] = 0
	}
	for _, c := range cards {
		out.Total++
		out.ByKind[c.Kind]++
		if c.Proposed {
			out.Proposed++
		}
		if c.Author.AgentAuthored {
			out.Agent++
		} else {
			out.Person++
		}
	}
	return out
}

// deckReviewer is who reads the deck: their id and name, what they may do in
// the project, and whether they are the workspace's only reviewer.
type deckReviewer struct {
	userID string
	name   string
	perms  platauth.Permission
	reach  platauth.CoordinateReach
	solo   bool
	// languages are the target languages the reviewer may review; empty is
	// every language.
	languages []string
}

// deckSource is what the deck of one project is dealt from.
type deckSource struct {
	wsID    string
	wsSlug  string
	project *platstore.Project
	key     workspace.ProjectKey
	records []contextop.Record
}

// overrideNote is the note a person's operation carries when it was recorded
// in an agent host's shell (host.ResolvedActor.NoteWith).
// NoteWith appends it in brackets after a note of the person's own, or
// writes it alone.
var overrideNote = regexp.MustCompile(`\s*\(?recorded as a person in an? (\S+) shell\)?`)

// opAuthor reads who wrote a ledger operation.
func opAuthor(r contextop.Record) contextDeckAuthor {
	kind := r.Actor.Kind
	if kind == "" {
		kind = contextop.ActorPerson
	}
	a := contextDeckAuthor{Kind: string(kind), Name: r.Actor.Name, Session: r.Actor.Session, OnBehalfOf: r.Actor.OnBehalfOf}
	if m := overrideNote.FindStringSubmatch(r.Note); m != nil {
		a.Host = m[1]
	}
	a.AgentAuthored = kind != contextop.ActorPerson || a.Host != ""
	return a
}

// recordAuthor is opAuthor with a person's name resolved: an operation a
// person recorded under a member's user id shows the member's display name,
// and keeps the id so the deck can tell the reviewer's own work.
func (s *Server) recordAuthor(ctx context.Context, r contextop.Record, names map[string]string) contextDeckAuthor {
	a := opAuthor(r)
	if a.Kind != string(contextop.ActorPerson) || a.Name == "" {
		return a
	}
	if name, ok := s.memberName(ctx, a.Name, names); ok {
		a.userID, a.Name = a.Name, name
	}
	return a
}

// memberName is the shown name (personName) of the member userID names, and whether
// one is known. names caches the answers of one deal, an unknown id as "".
func (s *Server) memberName(ctx context.Context, userID string, names map[string]string) (string, bool) {
	if name, ok := names[userID]; ok {
		return name, name != ""
	}
	name := s.personName(ctx, userID)
	names[userID] = name
	return name, name != ""
}

// memberAuthor is an item a store records under a member's user id, under
// "agent/<name>" for one an agent wrote, or under "tool/<name>" for one a
// tool wrote. An id that names no member shows no name rather than the id.
func (s *Server) memberAuthor(ctx context.Context, userID string, names map[string]string) contextDeckAuthor {
	if name, ok := strings.CutPrefix(userID, platauth.AgentIdentityPrefix); ok {
		return contextDeckAuthor{Kind: string(contextop.ActorAgent), Name: name, AgentAuthored: true}
	}
	if name, ok := platauth.ToolOfIdentity(userID); ok {
		return contextDeckAuthor{Kind: string(contextop.ActorTool), Name: name, AgentAuthored: true}
	}
	if userID == "" {
		return contextDeckAuthor{Kind: string(contextop.ActorPerson)}
	}
	name, _ := s.memberName(ctx, userID, names)
	return contextDeckAuthor{Kind: string(contextop.ActorPerson), Name: name, userID: userID}
}

// judge sets what a keep would record and which actions the card offers,
// from who wrote the item and who reads it. An agent's work is reviewed by
// the member it ran for (agent_authored); another person's by a peer. An item
// the reviewer wrote by hand, or one no record names an author for, needs
// another person, unless the reviewer is the workspace's only one
// (solo_owner).
func (card *contextDeckCard) judge(rv deckReviewer, need platauth.Permission, reaches, bound bool) {
	switch {
	case card.Author.AgentAuthored:
		card.Basis = contextop.BasisAgentAuthored
	case card.Author.userID != "" && card.Author.userID == rv.userID,
		card.Author.userID == "" && card.Author.Name != "" && card.Author.Name == rv.name:
		card.OwnWork = true
		if rv.solo {
			card.Basis = contextop.BasisSoloOwner
		}
	case card.Author.userID == "" && card.Author.Name == "":
		// No record says who wrote the item, so no reviewer can be shown to
		// be someone other than its author. Only a workspace's sole member
		// may keep it.
		if rv.solo {
			card.Basis = contextop.BasisSoloOwner
		}
	default:
		card.Basis = contextop.BasisPeer
	}
	card.Actions = []string{}
	if bound && rv.perms.Has(need) && reaches {
		if card.Basis != "" && card.Status != string(contextop.StatusContested) {
			card.Actions = append(card.Actions, deckActionKeep)
		}
		if card.Kind != deckKindVoiceRule {
			card.Actions = append(card.Actions, deckActionReject)
		}
	}
	card.Actions = append(card.Actions, deckActionLater)
}

// pendingContext deals the project's context items no attestation covers at
// their current revision, in deck order.
func (s *Server) pendingContext(ctx context.Context, src deckSource, rv deckReviewer) ([]contextDeckCard, error) {
	cards, err := s.contextItems(ctx, src, rv)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(cards, func(c contextDeckCard) bool { return c.Reviewed }), nil
}

// contextItems deals every context item of the project, reviewed or not, in
// deck order, each with the bulk operation that brought it in. A bulk
// operation (contextop.Record.Bulk) is never a card of its own: the concepts
// and voice rules it carried are, one by one.
func (s *Server) contextItems(ctx context.Context, src deckSource, rv deckReviewer) ([]contextDeckCard, error) {
	var cards []contextDeckCard
	bound := src.key != ""
	names := map[string]string{}
	var bulk []contextop.Record

	for _, r := range src.records {
		if !r.Kind.Bears() || !r.Status.Answers() {
			continue
		}
		if r.Project != src.key && r.Scope.Level != contextop.LevelWorkspace {
			continue
		}
		if r.Bulk() {
			bulk = append(bulk, r)
			continue
		}
		card := contextDeckCard{
			Item:     contextop.ItemRef{Kind: contextop.ItemOp, ID: r.ID},
			Revision: r.Revision,
			Status:   string(r.Status),
			Author:   s.recordAuthor(ctx, r, names),
			At:       r.At,
			Where: contextDeckWhere{
				Level: string(cmp.Or(r.Scope.Level, contextop.LevelProject)), Coordinates: r.Scope.Coordinates,
				ValidFrom: r.Scope.ValidFrom, ValidTo: r.Scope.ValidTo,
			},
		}
		switch r.Subject.Kind {
		case contextop.SubjectTerm:
			rule, ok := r.Subject.Rule()
			if !ok {
				continue
			}
			card.Kind = deckKindTermRule
			card.Term = &contextDeckTermRule{Term: rule.Term, Replacement: rule.Replacement, Forms: rule.Forms}
			card.Text = r.Subject.Text
			card.Flags = termRuleFlags(rule)
			if rule.DoNotTranslate {
				card.Language = string(src.project.DefaultSourceLanguage)
			}
		case contextop.SubjectNote:
			card.Kind = deckKindNote
			card.Text = r.Subject.Text
		default:
			continue
		}
		card.judge(rv, governingPermission(r.Subject.Kind), rv.reach.Unconstrained() || rv.reach.Reaches(r.Scope.Coordinates), bound)
		cards = append(cards, card)
	}

	origins := s.deckOrigins(ctx, bulk, names)

	concepts, translations, err := s.conceptCards(ctx, src, names)
	if err != nil {
		return nil, err
	}
	for i := range concepts {
		concepts[i].judge(rv, platauth.PermManageTerms, rv.reach.Unconstrained(), bound)
		concepts[i].Origin = conceptOrigin(origins, concepts[i].At)
	}
	cards = append(cards, concepts...)
	for _, tr := range translations {
		if !rv.mayReviewLanguage(tr.Language) {
			continue
		}
		tr.judge(rv, platauth.PermManageTerms, rv.reach.Unconstrained(), bound)
		cards = append(cards, tr)
	}

	voice, err := s.voiceRuleCards(ctx, src, bulk, origins, names)
	if err != nil {
		return nil, err
	}
	for i := range voice {
		voice[i].judge(rv, platauth.PermManageVoice, rv.reach.Unconstrained(), bound)
		if voice[i].Origin == nil {
			voice[i].Origin = voiceOrigin(origins, voice[i])
		}
	}
	cards = append(cards, voice...)

	reviews := contextop.Reviews(src.records)
	for i := range cards {
		review := reviews[cards[i].Item.Key()]
		if review.Current(cards[i].Revision) {
			cards[i].Reviewed = true
			continue
		}
		cards[i].Review = review
	}
	slices.SortStableFunc(cards, func(a, b contextDeckCard) int {
		return cmp.Or(
			cmp.Compare(contextDeckRank(a), contextDeckRank(b)),
			cmp.Compare(b.reachBlocks(), a.reachBlocks()),
			a.At.Compare(b.At),
			strings.Compare(a.Item.ID, b.Item.ID),
		)
	})
	return followConcepts(cards), nil
}

// bulkRecords are the bulk operations of the project's ledger.
func bulkRecords(src deckSource) []contextop.Record {
	var out []contextop.Record
	for _, r := range src.records {
		if r.Kind.Bears() && r.Status.Answers() && r.Bulk() &&
			(r.Project == src.key || r.Scope.Level == contextop.LevelWorkspace) {
			out = append(out, r)
		}
	}
	return out
}

// deckOrigins reads the bulk operations of the project's ledger as origins,
// oldest first.
func (s *Server) deckOrigins(ctx context.Context, bulk []contextop.Record, names map[string]string) []*contextDeckOrigin {
	out := make([]*contextDeckOrigin, 0, len(bulk))
	for _, r := range bulk {
		o := &contextDeckOrigin{
			ID: r.ID, Short: contextop.ShortID(r.ID), At: r.At,
			Author: s.recordAuthor(ctx, r, names),
			Note:   originNote(r.Note),
		}
		if src, ok := r.ImportOf(); ok {
			o.Kind, o.Source, o.Path, o.Count = originKindImport, src.Source, src.Path, src.Count
		} else {
			o.Kind, o.Profile = originKindVoiceEdit, voiceWriteProfile(r)
			if r.Voice != nil {
				o.Count = len(r.Voice.Changes) + r.Voice.Omitted
			}
		}
		out = append(out, o)
	}
	slices.SortStableFunc(out, func(a, b *contextDeckOrigin) int { return a.At.Compare(b.At) })
	return out
}

// originNote is what the person said about an operation, without the agent
// host marker the author already carries.
func originNote(note string) string {
	return strings.Trim(overrideNote.ReplaceAllString(note, ""), " ;")
}

// voiceWriteProfile is the voice profile a whole-profile write names: its
// VoiceEdit's, or the word after "voice profile " in the note of a write
// recorded without one.
func voiceWriteProfile(r contextop.Record) string {
	if r.Voice != nil {
		return r.Voice.Profile
	}
	rest, _ := strings.CutPrefix(r.Subject.Text, "voice profile ")
	name, _, _ := strings.Cut(rest, " ")
	return strings.TrimSuffix(name, ":")
}

// latestOrigin is the latest origin match accepts that was recorded at or
// before at, or the latest it accepts at all when none was or at is unknown.
// A store stamps an item when the item reached it, which is after the
// operation that carried it was recorded on the machine that ran it.
func latestOrigin(origins []*contextDeckOrigin, at time.Time, match func(*contextDeckOrigin) bool) *contextDeckOrigin {
	var before, last *contextDeckOrigin
	for _, o := range origins {
		if !match(o) {
			continue
		}
		last = o
		if !at.IsZero() && !o.At.After(at) {
			before = o
		}
	}
	if before != nil {
		return before
	}
	return last
}

// conceptOrigin is the terms import that brought a concept in.
func conceptOrigin(origins []*contextDeckOrigin, at time.Time) *contextDeckOrigin {
	return latestOrigin(origins, at, func(o *contextDeckOrigin) bool {
		return o.Kind == originKindImport && o.Source == contextop.ImportTerms
	})
}

// voiceOrigin is the voice import or whole-profile write that last changed
// the profile a voice rule belongs to.
func voiceOrigin(origins []*contextDeckOrigin, card contextDeckCard) *contextDeckOrigin {
	if card.Pattern == nil {
		return nil
	}
	return latestOrigin(origins, card.At, func(o *contextDeckOrigin) bool {
		switch o.Kind {
		case originKindVoiceEdit:
			return sameProfile(o.Profile, card.Item.Profile, card.Pattern.Profile)
		case originKindImport:
			return o.Source == contextop.ImportVoice && strings.Contains(voicewrite.ProfileSlug(o.Path), voicewrite.ProfileSlug(card.Pattern.Profile))
		}
		return false
	})
}

// sameProfile reports whether the profile a ledger operation names is the
// one the server holds as id and name. A checkout names a profile by its own
// id, which is the profile's name spelled as a slug.
func sameProfile(named, id, name string) bool {
	return voicewrite.SameProfile(named, id, name)
}

// termRuleFlags are the badges a term rule shows.
func termRuleFlags(rule coreprofile.TermRule) []string {
	var flags []string
	if rule.Advisory {
		flags = append(flags, "advisory")
	} else {
		flags = append(flags, "fails")
	}
	if rule.Competitor {
		flags = append(flags, "competitor")
	}
	if rule.DoNotTranslate {
		flags = append(flags, "do_not_translate")
	}
	return flags
}

// conceptCards are the cards of the concepts the workspace's terms store
// holds for the project: those of the whole workspace and the project's own.
// A concept's revision is its latest revision in the knowledge store, or the
// time it was last written when it has none. The second list is the cards
// of the target terms proposed for those concepts (translationCards).
func (s *Server) conceptCards(ctx context.Context, src deckSource, names map[string]string) ([]contextDeckCard, []contextDeckCard, error) {
	if s.wsStores == nil || src.wsSlug == "" {
		return nil, nil, nil
	}
	tb, err := s.wsStores.getTerms(ctx, src.wsSlug)
	if err != nil || tb == nil {
		return nil, nil, nil //nolint:nilerr // a workspace with no terms store has no concept cards
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("read the terms store: %w", err)
	}
	latest := map[string]*knowledge.ConceptRevision{}
	if s.KnowledgeStore != nil && src.wsID != "" {
		if latest, err = s.KnowledgeStore.LatestRevisions(ctx, src.wsID); err != nil {
			return nil, nil, err
		}
	}
	source := src.project.DefaultSourceLanguage
	out := make([]contextDeckCard, 0, len(concepts))
	var translations []contextDeckCard
	for _, cp := range concepts {
		project := termcandidate.ProjectOf(cp)
		if project != "" && project != src.project.ID {
			continue
		}
		card := contextDeckCard{
			Item:     contextop.ItemRef{Kind: contextop.ItemConcept, ID: cp.ID},
			Kind:     deckKindConcept,
			Text:     cp.Definition,
			Language: string(source),
			Where:    contextDeckWhere{Level: string(contextop.LevelWorkspace), Coordinates: cp.Coordinates()},
			At:       cp.CreatedAt,
		}
		if project != "" {
			card.Where.Level = string(contextop.LevelProject)
		}
		card.Proposed = termcandidate.IsCandidate(cp, source)
		if reach, ok := termcandidate.ReachOf(cp); ok {
			card.Reach = &contextDeckReach{Blocks: reach.Blocks, Collections: []contextDeckCollectionReach{}}
			for _, cc := range reach.Collections {
				card.Reach.Collections = append(card.Reach.Collections, contextDeckCollectionReach{Name: cc.Name, Blocks: cc.Blocks})
			}
		}
		card.Terms = conceptTerms(cp, source)
		if cp.Advisory {
			card.Flags = append(card.Flags, "advisory")
		} else {
			card.Flags = append(card.Flags, "fails")
		}
		if cp.DoNotTranslate {
			card.Flags = append(card.Flags, "do_not_translate")
		}
		if slices.ContainsFunc(cp.Terms, func(t terms.Term) bool { return t.CompetitorTerm }) {
			card.Flags = append(card.Flags, "competitor")
		}
		if rev := latest[cp.ID]; rev != nil {
			card.Revision = "r" + strconv.FormatInt(rev.Rev, 10)
			card.Author = s.memberAuthor(ctx, rev.Actor, names)
			card.At = rev.CreatedAt
		} else {
			card.Revision = "u" + strconv.FormatInt(cp.UpdatedAt.UnixNano(), 10)
			card.Author = contextDeckAuthor{Kind: string(contextop.ActorPerson)}
			// A candidate the scan filed names the scan as its author, so
			// the person who keeps it reviews a tool's work.
			if by := cp.Properties[termcandidate.PropProposedBy]; by != "" {
				card.Author = s.memberAuthor(ctx, by, names)
			}
		}
		out = append(out, card)
		translations = append(translations, s.translationCards(ctx, cp, card, source, names)...)
	}
	return out, translations, nil
}

// conceptTerms lists a concept's terms with the source language first.
func conceptTerms(cp terms.Concept, source model.LocaleID) []contextDeckConceptTerm {
	ts := slices.Clone(cp.Terms)
	slices.SortStableFunc(ts, func(a, b terms.Term) int {
		as, bs := sameLanguage(a.Locale, source), sameLanguage(b.Locale, source)
		switch {
		case as && !bs:
			return -1
		case bs && !as:
			return 1
		}
		return strings.Compare(string(a.Locale), string(b.Locale))
	})
	out := make([]contextDeckConceptTerm, len(ts))
	for i, t := range ts {
		out[i] = contextDeckConceptTerm{Text: t.Text, Locale: string(t.Locale), Status: string(t.Status)}
	}
	return out
}

// sameLanguage reports whether two locales name one language, ignoring the
// region: en-GB is English.
func sameLanguage(a, b model.LocaleID) bool {
	primary := func(l model.LocaleID) string {
		lang, _, _ := strings.Cut(strings.ReplaceAll(string(l), "_", "-"), "-")
		return strings.ToLower(lang)
	}
	return primary(a) != "" && primary(a) == primary(b)
}

// voiceRuleCards are the cards of the pattern rules of every voice profile
// bound in the project at its source language: the one the project resolves
// to, and each one a collection binds. A recipe that binds a voice per
// collection binds none on the project itself, so reading the project's
// binding alone deals no voice rule at all. A pattern rule has no id of its
// own. Its id is a digest of its list and its regular expression, and its
// revision a digest of the whole rule. Editing what a rule says is a new
// revision of it; editing what it matches is a new rule. The author is
// whoever made the latest write the ledger records that added the rule as it
// reads now; a rule no recorded write added names no author.
func (s *Server) voiceRuleCards(ctx context.Context, src deckSource, bulk []contextop.Record, origins []*contextDeckOrigin, names map[string]string) ([]contextDeckCard, error) {
	var out []contextDeckCard
	for _, profile := range s.boundVoiceProfiles(ctx, src) {
		steps := voiceEditSteps(bulk, profile)
		for _, card := range profileRuleCards(src, profile) {
			if step := steps.introduced(card.Pattern.List, card.Revision); step != nil {
				// The ledger is the only record of who made each voice edit,
				// an agent host included.
				card.Author = s.recordAuthor(ctx, step.rec, names)
				card.At = step.rec.At
				card.Origin = originByID(origins, step.rec.ID)
			}
			out = append(out, card)
		}
	}
	return out, nil
}

// voiceEditStep is one whole-profile write the ledger records, with the
// pattern rules the profile held before and after it, keyed by list and
// revision.
type voiceEditStep struct {
	rec    contextop.Record
	before map[string]bool
	after  map[string]bool
}

// voiceEditHistory is a profile's ledger writes, oldest first.
type voiceEditHistory []voiceEditStep

// voiceEditSteps reads the ledger's writes of profile. A write's after-state
// is the next write's before-state, and the last write's is the profile as
// stored.
func voiceEditSteps(bulk []contextop.Record, profile *coreprofile.VoiceProfile) voiceEditHistory {
	var steps voiceEditHistory
	for _, r := range bulk {
		if r.Voice == nil || !sameProfile(r.Voice.Profile, profile.ID, profile.Name) {
			continue
		}
		step := voiceEditStep{rec: r, before: map[string]bool{}}
		if r.Voice.Previous != "" {
			prev, err := coreprofile.LoadProfileYAML(strings.NewReader(r.Voice.Previous))
			if err != nil {
				continue
			}
			step.before = patternKeys(prev)
		}
		steps = append(steps, step)
	}
	slices.SortStableFunc(steps, func(a, b voiceEditStep) int { return a.rec.At.Compare(b.rec.At) })
	for i := range steps {
		if i+1 < len(steps) {
			steps[i].after = steps[i+1].before
		} else {
			steps[i].after = patternKeys(profile)
		}
	}
	return steps
}

// introduced is the latest write that added the rule at revision rev, or nil
// when no write the ledger records did.
func (h voiceEditHistory) introduced(list, rev string) *voiceEditStep {
	key := list + "\x00" + rev
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].after[key] && !h[i].before[key] {
			return &h[i]
		}
	}
	return nil
}

// patternKeys are a profile's pattern rules keyed by list and revision.
func patternKeys(p *coreprofile.VoiceProfile) map[string]bool {
	out := map[string]bool{}
	for list, patterns := range map[string][]coreprofile.Pattern{
		"prohibited": p.Style.ProhibitedPatterns,
		"required":   p.Style.RequiredPatterns,
	} {
		for _, pat := range patterns {
			_, rev := patternIdentity(list, pat)
			out[list+"\x00"+rev] = true
		}
	}
	return out
}

// originByID is the origin of the operation id names, or nil.
func originByID(origins []*contextDeckOrigin, id string) *contextDeckOrigin {
	for _, o := range origins {
		if o.ID == id {
			return o
		}
	}
	return nil
}

// boundVoiceProfiles are the voice profiles bound in the project, resolved at
// its source language: the project's own first, then each collection's, each
// profile once.
func (s *Server) boundVoiceProfiles(ctx context.Context, src deckSource) []*coreprofile.VoiceProfile {
	if s.VoiceStore == nil || s.ContentStore == nil {
		return nil
	}
	var wd voicescope.WorkspaceDefault
	if s.AuthStore != nil {
		wd = &mcpWorkspaceDefaultAdapter{auth: s.AuthStore}
	}
	scope := voicescope.Scope{
		WorkspaceID: src.project.WorkspaceID, ProjectID: src.project.ID, Stream: reviewInboxStream,
		Locale: src.project.DefaultSourceLanguage,
	}
	scopes := []voicescope.Scope{scope}
	if cols, err := s.ContentStore.ListCollections(ctx, src.project.ID, reviewInboxStream); err == nil {
		for _, col := range cols {
			if col.ConnectorConfig[coreprofile.PropertyProfileID] == "" {
				continue
			}
			at := scope
			at.CollectionID = col.ID
			scopes = append(scopes, at)
		}
	}
	seen := map[string]bool{}
	var out []*coreprofile.VoiceProfile
	for _, sc := range scopes {
		profile, err := voicescope.Resolve(ctx, s.ContentStore, wd, s.VoiceStore, sc)
		if err != nil || profile == nil || seen[profile.ID] {
			continue
		}
		seen[profile.ID] = true
		out = append(out, profile)
	}
	return out
}

// profileRuleCards are the cards of one voice profile's pattern rules.
func profileRuleCards(src deckSource, profile *coreprofile.VoiceProfile) []contextDeckCard {
	var out []contextDeckCard
	add := func(list string, patterns []coreprofile.Pattern) {
		for _, p := range patterns {
			id, rev := patternIdentity(list, p)
			card := contextDeckCard{
				Item:     contextop.ItemRef{Kind: contextop.ItemVoiceRule, ID: id, Profile: profile.ID},
				Kind:     deckKindVoiceRule,
				Revision: rev,
				Language: string(src.project.DefaultSourceLanguage),
				Where:    contextDeckWhere{Level: string(contextop.LevelProject)},
				Pattern: &contextDeckPattern{
					List: list, Regex: p.Regex, Description: p.Description, Profile: profile.Name,
					Words: coreprofile.PatternWords(p.Regex),
				},
			}
			if p.Scope != "" {
				card.Where.Coordinates = map[string]string{"text": p.Scope}
			}
			if p.Rate != nil {
				card.Pattern.Rate = fmt.Sprintf("%d/%d", p.Rate.Max, p.Rate.Per)
				card.Flags = append(card.Flags, "rate")
			}
			if p.Failing() {
				card.Flags = append(card.Flags, "fails")
			} else {
				card.Flags = append(card.Flags, "advisory")
			}
			// The author is the ledger's to say (voiceRuleCards); a rule no
			// recorded write added names none.
			card.Author = contextDeckAuthor{Kind: string(contextop.ActorPerson)}
			card.At = profile.UpdatedAt
			out = append(out, card)
		}
	}
	add("prohibited", profile.Style.ProhibitedPatterns)
	add("required", profile.Style.RequiredPatterns)
	return out
}

// patternIdentity is a pattern rule's id and revision.
func patternIdentity(list string, p coreprofile.Pattern) (id, rev string) {
	idSum := sha256.Sum256([]byte(list + "\x00" + p.Regex))
	body, _ := json.Marshal(p)
	revSum := sha256.Sum256(append([]byte(list+"\x00"), body...))
	return hex.EncodeToString(idSum[:8]), hex.EncodeToString(revSum[:8])
}

// findCard is the card naming item, and whether the deck holds one.
func findCard(cards []contextDeckCard, item contextop.ItemRef) (contextDeckCard, bool) {
	for _, c := range cards {
		if c.Item.Key() == item.Key() {
			return c, true
		}
	}
	return contextDeckCard{}, false
}
