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
// then the terms, then the rules and notes recorded in the ledger.
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
	// AgentAuthored says an agent or a tool wrote the item, or a person in an
	// agent host's shell.
	AgentAuthored bool `json:"agent_authored"`
	// userID is the member the store names, when it names one by id.
	userID string
}

// contextDeckCounts counts the cards by kind and by who wrote them.
type contextDeckCounts struct {
	Total  int            `json:"total"`
	ByKind map[string]int `json:"by_kind"`
	Agent  int            `json:"agent"`
	Person int            `json:"person"`
}

func countDeck(cards []contextDeckCard) contextDeckCounts {
	out := contextDeckCounts{ByKind: map[string]int{}}
	for _, k := range deckKindOrder {
		out.ByKind[k] = 0
	}
	for _, c := range cards {
		out.Total++
		out.ByKind[c.Kind]++
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
var overrideNote = regexp.MustCompile(`recorded as a person in an? (\S+) shell`)

// opAuthor reads who wrote a ledger operation.
func opAuthor(r contextop.Record) contextDeckAuthor {
	kind := r.Actor.Kind
	if kind == "" {
		kind = contextop.ActorPerson
	}
	a := contextDeckAuthor{Kind: string(kind), Name: r.Actor.Name, Session: r.Actor.Session}
	if m := overrideNote.FindStringSubmatch(r.Note); m != nil {
		a.Host = m[1]
	}
	a.AgentAuthored = kind != contextop.ActorPerson || a.Host != ""
	return a
}

// memberAuthor is an item a store records under a member's user id, or
// under "agent/<name>" for one an agent wrote.
func (s *Server) memberAuthor(ctx context.Context, userID string, names map[string]string) contextDeckAuthor {
	if name, ok := strings.CutPrefix(userID, "agent/"); ok {
		return contextDeckAuthor{Kind: string(contextop.ActorAgent), Name: name, AgentAuthored: true}
	}
	if userID == "" {
		return contextDeckAuthor{Kind: string(contextop.ActorPerson)}
	}
	name, ok := names[userID]
	if !ok {
		name = s.displayName(ctx, userID)
		names[userID] = name
	}
	return contextDeckAuthor{Kind: string(contextop.ActorPerson), Name: name, userID: userID}
}

// judge sets what a keep would record and which actions the card offers,
// from who wrote the item and who reads it. An agent's work is reviewed by
// the member it ran for (agent_authored); another person's by a peer. An item
// the reviewer wrote by hand needs another person, unless the reviewer is the
// workspace's only one (solo_owner).
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
	var cards []contextDeckCard
	bound := src.key != ""
	names := map[string]string{}

	for _, r := range src.records {
		if !r.Kind.Bears() || !r.Status.Answers() {
			continue
		}
		if r.Project != src.key && r.Scope.Level != contextop.LevelWorkspace {
			continue
		}
		card := contextDeckCard{
			Item:     contextop.ItemRef{Kind: contextop.ItemOp, ID: r.ID},
			Revision: r.Revision,
			Status:   string(r.Status),
			Author:   opAuthor(r),
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

	concepts, err := s.conceptCards(ctx, src, names)
	if err != nil {
		return nil, err
	}
	for i := range concepts {
		concepts[i].judge(rv, platauth.PermManageTerms, rv.reach.Unconstrained(), bound)
	}
	cards = append(cards, concepts...)

	voice, err := s.voiceRuleCards(ctx, src, names)
	if err != nil {
		return nil, err
	}
	for i := range voice {
		voice[i].judge(rv, platauth.PermManageVoice, rv.reach.Unconstrained(), bound)
	}
	cards = append(cards, voice...)

	reviews := contextop.Reviews(src.records)
	pending := cards[:0]
	for _, card := range cards {
		review := reviews[card.Item.Key()]
		if review.Current(card.Revision) {
			continue
		}
		card.Review = review
		pending = append(pending, card)
	}
	slices.SortStableFunc(pending, func(a, b contextDeckCard) int {
		return cmp.Or(
			cmp.Compare(slices.Index(deckKindOrder, a.Kind), slices.Index(deckKindOrder, b.Kind)),
			a.At.Compare(b.At),
			strings.Compare(a.Item.ID, b.Item.ID),
		)
	})
	return pending, nil
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
// time it was last written when it has none.
func (s *Server) conceptCards(ctx context.Context, src deckSource, names map[string]string) ([]contextDeckCard, error) {
	if s.wsStores == nil || src.wsSlug == "" {
		return nil, nil
	}
	tb, err := s.wsStores.getTerms(ctx, src.wsSlug)
	if err != nil || tb == nil {
		return nil, nil //nolint:nilerr // a workspace with no terms store has no concept cards
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the terms store: %w", err)
	}
	latest := map[string]*knowledge.ConceptRevision{}
	if s.KnowledgeStore != nil && src.wsID != "" {
		if latest, err = s.KnowledgeStore.LatestRevisions(ctx, src.wsID); err != nil {
			return nil, err
		}
	}
	source := src.project.DefaultSourceLanguage
	out := make([]contextDeckCard, 0, len(concepts))
	for _, cp := range concepts {
		if cp.ProjectID != "" && cp.ProjectID != src.project.ID {
			continue
		}
		card := contextDeckCard{
			Item:     contextop.ItemRef{Kind: contextop.ItemConcept, ID: cp.ID},
			Kind:     deckKindConcept,
			Text:     cp.Definition,
			Language: string(source),
			Where:    contextDeckWhere{Level: string(contextop.LevelWorkspace)},
			At:       cp.CreatedAt,
		}
		if cp.ProjectID != "" {
			card.Where.Level = string(contextop.LevelProject)
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
		}
		out = append(out, card)
	}
	return out, nil
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

// voiceRuleCards are the cards of the pattern rules of the voice profile
// bound to the project at its source language. A pattern rule has no id of
// its own, so its id is a digest of its list and its regular expression and
// its revision a digest of the whole rule: editing what a rule says is a new
// revision of it, and editing what it matches is a new rule. The author is
// whoever wrote the first profile version that holds the rule as it reads
// now.
func (s *Server) voiceRuleCards(ctx context.Context, src deckSource, names map[string]string) ([]contextDeckCard, error) {
	if s.VoiceStore == nil || s.ContentStore == nil {
		return nil, nil
	}
	var wd voicescope.WorkspaceDefault
	if s.AuthStore != nil {
		wd = &mcpWorkspaceDefaultAdapter{auth: s.AuthStore}
	}
	profile, err := voicescope.Resolve(ctx, s.ContentStore, wd, s.VoiceStore, voicescope.Scope{
		WorkspaceID: src.project.WorkspaceID, ProjectID: src.project.ID, Stream: reviewInboxStream,
		Locale: src.project.DefaultSourceLanguage,
	})
	if err != nil || profile == nil {
		return nil, nil //nolint:nilerr // a project with no voice bound has no voice rule cards
	}
	versions, _ := s.VoiceStore.ListProfileVersions(ctx, profile.ID)
	slices.SortFunc(versions, func(a, b *coreprofile.ProfileVersion) int { return cmp.Compare(a.Version, b.Version) })

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
			card.Author, card.At = s.patternAuthor(ctx, list, rev, profile, versions, names)
			out = append(out, card)
		}
	}
	add("prohibited", profile.Style.ProhibitedPatterns)
	add("required", profile.Style.RequiredPatterns)
	return out, nil
}

// patternIdentity is a pattern rule's id and revision.
func patternIdentity(list string, p coreprofile.Pattern) (id, rev string) {
	idSum := sha256.Sum256([]byte(list + "\x00" + p.Regex))
	body, _ := json.Marshal(p)
	revSum := sha256.Sum256(append([]byte(list+"\x00"), body...))
	return hex.EncodeToString(idSum[:8]), hex.EncodeToString(revSum[:8])
}

// patternAuthor is who wrote the first profile version holding the rule at
// revision rev, or the profile's author when no version holds it.
func (s *Server) patternAuthor(ctx context.Context, list, rev string, profile *coreprofile.VoiceProfile, versions []*coreprofile.ProfileVersion, names map[string]string) (contextDeckAuthor, time.Time) {
	for _, v := range versions {
		patterns := v.Snapshot.Style.ProhibitedPatterns
		if list == "required" {
			patterns = v.Snapshot.Style.RequiredPatterns
		}
		for _, p := range patterns {
			if _, r := patternIdentity(list, p); r == rev {
				return s.memberAuthor(ctx, v.CreatedBy, names), v.CreatedAt
			}
		}
	}
	return s.memberAuthor(ctx, profile.CreatedBy, names), profile.UpdatedAt
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
