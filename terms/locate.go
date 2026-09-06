// This file answers one question for the whole engine: which declared terms
// does this passage use, and where exactly.
//
// A term can be declared in two places. A voice profile lists the words a brand
// forbids and a competitor's names it must not print; a tool's `term_rules:`
// lists the wording a piece of content is held to; and the bound terms store
// holds the concepts a project has actually decided, which is what `kapi apply`
// writes to. Both are the same kind of statement about the same words, so
// answering them separately is how two gates come to disagree about whether a
// word is in use.
//
// This package answers them together. It matches with check.TermMatcher through
// profile.MatchTermRules, so a word is a hit for the whole gate or for none of
// it; it resolves the store's matches to what the concept says to say instead;
// and it hands back occurrences already anchored to the block's runs, so no
// consumer re-derives a position from a byte offset.
//
// What a consumer does with an occurrence is its own business: the voice
// vocabulary gate raises a finding, term-check reports a violation, dnt-check
// asserts the target preserved it, and term-locate writes it onto the block as
// an annotation. Locating is the part they share.
package terms

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
)

// Source says where a declared term came from.
type OccurrenceSource string

const (
	// SourceRule is a rule the caller held: a voice profile's vocabulary, a
	// tool's `term_rules:`, a recipe's list.
	SourceRule OccurrenceSource = "rule"
	// SourceStore is a concept in the bound terms store.
	SourceStore OccurrenceSource = "store"
)

// An Occurrence is one use of one declared term in a piece of content, anchored
// to the runs it sits in.
type Occurrence struct {
	// Anchor is where the occurrence sits in the block's runs. It is the
	// position every consumer records, so a finding, an annotation and an
	// exported mark all point at the same characters.
	Anchor model.Anchor
	// Text is the surface form as the content spells it, which is not always
	// how the rule spells it: matching is case-insensitive, so a rule for
	// "kapi" hits "Kapi" and the occurrence carries what the content said.
	Text string
	// Term is the declared term that matched.
	Term string
	// Replacement is what to say instead, empty when the declaration names
	// nothing. A store match resolves it through the concept's preferred term
	// in the same language before falling back to the term's note.
	Replacement string
	Note        string
	// ConceptID ties the occurrence to the concept in the terms store and the
	// graph. Empty for a rule that names none, which a standalone profile with
	// no backing store is entitled to.
	ConceptID string
	// Severity and Kind come from the rule that matched and are empty for a
	// store match, which carries Status and Competitor instead. Nothing here is
	// a judgement about whether the occurrence is a problem: a term being used
	// is a fact, and which uses are violations is the consuming gate's policy.
	Severity profile.Severity
	Category profile.Dimension
	Kind     profile.VocabKind
	// Status is the concept's standing for a store match: preferred, approved,
	// forbidden, deprecated.
	Status model.TermStatus
	// Competitor marks a store term recorded as a rival's name.
	Competitor bool
	// Concept is the store concept the occurrence denotes, for a consumer that
	// needs more of it than the id — its sibling terms in another language,
	// say. Nil for a rule hit, which denotes no concept of its own.
	Concept *Concept
	// Score and MatchType record how the store matched: 1.0 and exact for a
	// literal hit, lower for a fuzzy one. Zero for a rule hit, which matches
	// through the shared matcher rather than the store's scoring.
	Score     float64
	MatchType model.MatchStrategy
	// DoNotTranslate marks a term that must survive into every target exactly as
	// it stands.
	DoNotTranslate bool
	Source         OccurrenceSource
	// Start and End are byte offsets into the searched text, for a consumer that
	// reports over flat text rather than over runs.
	Start, End int
}

// A LocateRequest is one pass over one piece of content.
type LocateRequest struct {
	// Text is the content searched, and Runs are the runs it flattens from.
	// Offsets reported by the matcher index into Text; the anchor on each
	// occurrence is resolved against Runs. Runs may be nil for a caller
	// matching plain text, and the anchors are then zero.
	Text string
	Runs []model.Run
	// RuleSets are the declared terms the caller holds, whatever their origin.
	// A caller with a voice profile passes profile.VocabularyRuleSets(p); a
	// caller with a tool's `term_rules:` passes its own set; a caller with both
	// passes both, and one pass covers them.
	RuleSets []profile.TermRuleSet
	// Store is the bound terms store, consulted when non-nil. Every source in
	// it is consulted rather than the voice vocabulary alone: a term decision
	// lands in the terminology source, so a vocabulary-only filter would
	// enforce a source nothing writes to and ignore the one everything does.
	Store Terminology
	// Locale is the language the store is asked in. The lookup matches a locale
	// exactly, so the region is tried and then the language beneath it.
	Locale model.LocaleID
	// Domains, MinScore and Scope are passed through to the store lookup. They
	// narrow WHICH declarations are consulted, which is a different question
	// from which uses of them matter — that stays with the caller.
	Domains  []string
	MinScore float64
	Scope    *graph.Scope
}

// Find returns every occurrence of every declared term in the request's content,
// rule hits first and store matches after, each in the order it was declared.
func Locate(ctx context.Context, req LocateRequest) ([]Occurrence, error) {
	if strings.TrimSpace(req.Text) == "" {
		return nil, nil
	}
	out := ruleOccurrences(req)
	if req.Store == nil {
		return out, nil
	}
	stored, err := storeOccurrences(ctx, req)
	if err != nil {
		return nil, err
	}
	return append(out, stored...), nil
}

// ruleOccurrences matches the caller's declared rules through the one matcher.
func ruleOccurrences(req LocateRequest) []Occurrence {
	hits := profile.MatchTermRules(req.RuleSets, req.Text)
	if len(hits) == 0 {
		return nil
	}
	dnt := doNotTranslateTerms(req.RuleSets)
	out := make([]Occurrence, 0, len(hits))
	for _, h := range hits {
		out = append(out, Occurrence{
			Anchor:         anchorFor(req.Runs, h.Start, h.End),
			Text:           req.Text[h.Start:h.End],
			Term:           h.Term,
			Replacement:    h.Replacement,
			Note:           h.Note,
			ConceptID:      h.ConceptID,
			Severity:       h.Severity,
			Category:       h.Category,
			Kind:           h.Kind,
			DoNotTranslate: dnt[strings.ToLower(h.Term)],
			Source:         SourceRule,
			Start:          h.Start,
			End:            h.End,
		})
	}
	return out
}

// doNotTranslateTerms indexes which declared terms carry the flag, so an
// occurrence can answer for it without the consumer re-reading the rules.
func doNotTranslateTerms(sets []profile.TermRuleSet) map[string]bool {
	out := map[string]bool{}
	for _, set := range sets {
		for _, rule := range set.Rules {
			if rule.DoNotTranslate {
				out[strings.ToLower(rule.Term)] = true
			}
		}
	}
	return out
}

// storeOccurrences looks the content up in the bound terms store.
//
// Matches are deduped across the candidate languages: a term recorded in both
// en-GB and en is one decision about one word, and reporting it twice would
// penalize the same characters twice.
func storeOccurrences(ctx context.Context, req LocateRequest) ([]Occurrence, error) {
	type hit struct {
		text string
		pos  int
	}
	seen := map[hit]bool{}
	var out []Occurrence
	for _, loc := range LookupLocales(req.Locale) {
		found, err := req.Store.LookupAll(ctx, req.Text, LookupOptions{
			SourceLocale: loc,
			Domains:      req.Domains,
			MinScore:     req.MinScore,
			Scope:        req.Scope,
		})
		if err != nil {
			return nil, err
		}
		for _, m := range found {
			key := hit{text: strings.ToLower(m.Term.Text), pos: m.Position.Start}
			if seen[key] {
				continue
			}
			seen[key] = true
			replacement, err := PreferredTerm(ctx, req.Store, m, req.Locale)
			if err != nil {
				return nil, err
			}
			concept := m.Concept
			out = append(out, Occurrence{
				Anchor:         anchorFor(req.Runs, m.Position.Start, m.Position.End),
				Text:           req.Text[m.Position.Start:m.Position.End],
				Term:           m.Term.Text,
				Replacement:    replacement,
				Note:           m.Term.Note,
				ConceptID:      m.Concept.ID,
				Category:       profile.DimensionVocabulary,
				Competitor:     m.Term.CompetitorTerm,
				Concept:        &concept,
				Score:          m.Score,
				MatchType:      m.MatchType,
				Status:         m.Term.Status,
				DoNotTranslate: m.Concept.DoNotTranslate,
				Source:         SourceStore,
				Start:          m.Position.Start,
				End:            m.Position.End,
			})
		}
	}
	return out, nil
}

// anchorFor resolves a byte span onto the runs it sits in. A caller matching
// plain text passes no runs, and the occurrence carries no position.
func anchorFor(runs []model.Run, start, end int) model.Anchor {
	if len(runs) == 0 {
		return model.Anchor{}
	}
	return model.RangeAnchorForBytes(runs, start, end)
}

// LookupLocales is the candidate list a terms lookup is tried against: the
// locale as written, then the language beneath it.
//
// A lookup matches the locale exactly, so a project writing en-GB against a
// vocabulary recorded as `en` would be held to nothing at all, and the gate
// would report PASS because it asked the wrong question. An empty locale yields
// no candidate: there is no language to look terms up in, and the empty string
// is a locale like any other in the store.
func LookupLocales(loc model.LocaleID) []model.LocaleID {
	loc = model.NormalizeLocale(loc)
	if loc == "" {
		return nil
	}
	out := []model.LocaleID{loc}
	if i := strings.IndexAny(string(loc), "-_"); i > 0 {
		out = append(out, loc[:i])
	}
	return out
}

// PreferredTerm answers "what should this say instead" for a matched term: the
// preferred term its concept carries in the same language, or the replacement
// the term's own note names when the concept carries no preferred sibling.
//
// The match is consulted first and the store second, because a lookup can return
// either shape: the in-memory store hands back the whole concept, while the
// SQLite one selects a row per term and fills in a concept STUB carrying the id
// and definition but none of its other terms. Reading only the match therefore
// leaves every occurrence from a real project without the alternative — the one
// part of a vocabulary finding that says what to do.
//
// The note is the last resort rather than the first, because a concept that
// carries both words is the better record and the one `kapi apply` writes. It
// still has to be read: a term decided before that, or imported from a
// vocabulary that files each word separately, has the replacement in the note
// and nowhere else, and a finding that names no alternative is the beat of the
// correction loop that lands without its fix.
func PreferredTerm(ctx context.Context, store Terminology, m TermMatch, loc model.LocaleID) (string, error) {
	if pref := m.Concept.PreferredTerm(loc); pref != nil && pref.Text != "" {
		return pref.Text, nil
	}
	if m.Concept.ID == "" || len(m.Concept.Terms) > 0 {
		// Nothing to look up, or the concept was whole and named none.
		return ReplacementFromNote(m.Term.Note), nil
	}
	full, found, err := store.GetConcept(ctx, m.Concept.ID)
	if err != nil {
		return "", fmt.Errorf("read concept %s: %w", m.Concept.ID, err)
	}
	if !found {
		return ReplacementFromNote(m.Term.Note), nil
	}
	if pref := full.PreferredTerm(loc); pref != nil && pref.Text != "" {
		return pref.Text, nil
	}
	return ReplacementFromNote(m.Term.Note), nil
}

// Hit presents the occurrence as a vocabulary hit, so a consumer can pass it
// to profile.HitsToFindings and get the message, suggestion and concept
// metadata every vocabulary surface shares rather than wording its own.
func (o Occurrence) Hit() profile.VocabHit {
	return profile.VocabHit{
		Kind:        o.Kind,
		Category:    o.Category,
		Severity:    o.Severity,
		Term:        o.Term,
		Replacement: o.Replacement,
		Note:        o.Note,
		ConceptID:   o.ConceptID,
		Start:       o.Start,
		End:         o.End,
	}
}
