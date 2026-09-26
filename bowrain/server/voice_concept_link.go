package server

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// defaultVoiceConceptLocale is the language a promoted term is written in when
// the workspace has no project to infer one from. English is the platform's
// authoring default; the locale only scopes the term's text, so a fallback never
// blocks a promotion.
const defaultVoiceConceptLocale = model.LocaleID("en")

// promoteRuleToTerms lands a correction-derived rule in the workspace terms
// store (terms.PromoteRule): the term a team kept correcting away becomes a
// forbidden term in the workspace's source language, joined to the concept whose
// preferred term is its replacement. Writing a forbidden term directly bypasses
// the change-set governance the HTTP concept handlers enforce, which is correct
// here: the promotion is itself the reviewed (or autonomy-thresholded)
// decision, so the loop is the governance.
//
// It is idempotent: promoting the same rule again changes nothing. It returns
// whether the store changed, the concept the term joined, and the knowledge
// event the caller publishes (concept created or updated; none when nothing
// changed).
//
// wsSlug keys the workspace terms (getTerms); wsID scopes the project lookup that
// resolves the source locale and stamps the emitted event.
func (s *Server) promoteRuleToTerms(ctx context.Context, wsSlug, wsID string, rule coreprofile.SuggestedRule) (bool, string, []knowledge.MergeEvent, error) {
	term := strings.TrimSpace(rule.Term)
	if term == "" {
		return false, "", nil, nil
	}
	if s.wsStores == nil {
		return false, "", nil, errors.New("terms store not configured")
	}
	tb, err := s.wsStores.getTerms(wsSlug)
	if err != nil {
		return false, "", nil, err
	}
	locale := s.voiceConceptLocale(ctx, wsID)
	before, err := tb.Concepts(ctx)
	if err != nil {
		return false, "", nil, err
	}
	existed := map[string]bool{}
	for _, c := range before {
		existed[c.ID] = true
	}
	rule.Term = term
	changed, err := terms.PromoteRule(ctx, tb, locale, rule)
	if err != nil || !changed {
		return false, "", nil, err
	}
	after, err := tb.Concepts(ctx)
	if err != nil {
		return true, "", nil, err
	}
	ci := terms.IndexOfTerm(after, term, locale)
	if ci < 0 {
		return true, "", nil, nil
	}
	conceptID := after[ci].ID
	evType := knowledge.EventConceptUpdated
	if !existed[conceptID] {
		evType = knowledge.EventConceptCreated
	}
	return true, conceptID, []knowledge.MergeEvent{conceptEvent(evType, wsID, conceptID, "")}, nil
}

// demoteRuleFromTerms removes a promoted term from the workspace terms store
// (terms.DemoteRule), reporting whether the store changed.
func (s *Server) demoteRuleFromTerms(ctx context.Context, wsSlug, wsID, term string) (bool, error) {
	if s.wsStores == nil {
		return false, errors.New("terms store not configured")
	}
	tb, err := s.wsStores.getTerms(wsSlug)
	if err != nil {
		return false, err
	}
	return terms.DemoteRule(ctx, tb, s.voiceConceptLocale(ctx, wsID), term)
}

// landWordRules writes word rules a voice file carries (a starter pack's terms)
// into the workspace terms store, in the workspace's source language: a rule
// naming a term becomes a forbidden term (a competitor's name when the rule says
// so, advisory when it says so) joined to the concept of its replacement, and a
// rule naming only a replacement becomes a preferred term. Rules the store
// already records are left as they are. It returns how many concepts changed.
// A server with no terms store has nowhere to hold them, and lands none.
func (s *Server) landWordRules(ctx context.Context, wsSlug, wsID string, rules []coreprofile.TermRule) (int, error) {
	if len(rules) == 0 || s.wsStores == nil {
		return 0, nil
	}
	tb, err := s.wsStores.getTerms(wsSlug)
	if errors.Is(err, errNoPgDB) {
		slog.WarnContext(ctx, "no terms store: a voice file's word rules were not landed", "workspace", wsSlug, "rules", len(rules))
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return 0, err
	}
	locale := s.voiceConceptLocale(ctx, wsID)
	written := 0
	for _, r := range rules {
		d := terms.Decision{
			Text:        strings.TrimSpace(r.Term),
			Locale:      locale,
			Status:      model.TermForbidden,
			Replacement: strings.TrimSpace(r.Replacement),
			Advisory:    r.Advisory,
			Competitor:  r.Competitor,
			Forms:       r.Forms,
		}
		if d.Text == "" {
			d.Text, d.Replacement, d.Status = d.Replacement, "", model.TermPreferred
		}
		if d.Text == "" {
			continue
		}
		var target int
		var changed bool
		concepts, target, changed = terms.UpsertDecision(concepts, d)
		if !changed {
			continue
		}
		if err := tb.AddConcept(ctx, concepts[target]); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// voiceConceptLocale resolves the source locale to tag a brand concept's terms
// with: the first matching workspace project's default source language, falling
// back to English when no project (or no content store) is available.
func (s *Server) voiceConceptLocale(ctx context.Context, wsID string) model.LocaleID {
	if s.ContentStore != nil {
		if loc := firstWorkspaceSourceLocale(ctx, s.ContentStore, wsID); loc != "" {
			return loc
		}
	}
	return defaultVoiceConceptLocale
}

// firstWorkspaceSourceLocale returns the default source language of the first
// project in the workspace that declares one, or "" when none does. wsID == ""
// matches any project (single-tenant / test setups).
func firstWorkspaceSourceLocale(ctx context.Context, ps store.ProjectStore, wsID string) model.LocaleID {
	projects, err := ps.ListProjects(ctx)
	if err != nil {
		return ""
	}
	for _, p := range projects {
		if p == nil {
			continue
		}
		if wsID != "" && p.WorkspaceID != wsID {
			continue
		}
		if p.DefaultSourceLanguage != "" {
			return p.DefaultSourceLanguage
		}
	}
	return ""
}
