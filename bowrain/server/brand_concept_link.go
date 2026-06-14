package server

import (
	"context"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// defaultBrandConceptLocale is the source locale a brand-vocabulary concept's
// terms get when the workspace has no project to infer one from. English is the
// platform's authoring default; the locale only scopes the concept's term text,
// so a fallback never blocks the forbidden→preferred link from being recorded.
const defaultBrandConceptLocale = model.LocaleID("en")

// linkRuleToConcept threads a promoted brand-vocabulary rule into the workspace
// knowledge graph (AD-021, "one node type: the concept"). The forbidden term a
// team keeps correcting away becomes a concept-backed forbidden term; its
// replacement becomes a preferred-term concept; and a USE_INSTEAD edge connects
// them — so a flat, correction-derived rule gains the concept story the rest of
// the platform reasons over (concept blast radius, the navigator, the brand-vocab
// finding's concept_id pivot).
//
// It is idempotent: re-promoting the same rule reuses the existing brand-vocab
// concepts (matched case-insensitively on term text and status) and never
// duplicates the USE_INSTEAD edge. It returns the forbidden concept's ID — which
// the caller stamps onto the promoted rule via SuggestedRule.ConceptID so the
// flat TermRule denotes its concept — and the knowledge events the caller should
// publish (an EventConceptCreated per newly minted concept and an
// EventConceptRelationAdded when a new edge is added). The returned slice is
// empty when the graph already held everything, so publishing it is a no-op.
//
// wsSlug keys the workspace termbase (getTB); wsID scopes the project lookup that
// resolves the source locale and stamps the emitted events.
func (s *Server) linkRuleToConcept(ctx context.Context, wsSlug, wsID string, rule corebrand.SuggestedRule) (string, []knowledge.MergeEvent, error) {
	term := strings.TrimSpace(rule.Term)
	if term == "" {
		return "", nil, nil
	}
	tb, err := s.wsStores.getTB(wsSlug)
	if err != nil {
		return "", nil, err
	}

	locale := s.brandConceptLocale(ctx, wsID)

	var events []knowledge.MergeEvent

	// The forbidden term becomes (or reuses) a brand-vocabulary concept. Writing a
	// forbidden term directly bypasses the change-set governance the HTTP concept
	// handlers enforce, which is correct here: the promotion this links from is
	// itself the reviewed (or autonomy-thresholded) decision, so the loop is the
	// governance.
	forbiddenID, created, err := upsertBrandVocabConcept(ctx, tb, term, locale, model.TermForbidden)
	if err != nil {
		return "", nil, err
	}
	if created {
		events = append(events, conceptEvent(knowledge.EventConceptCreated, wsID, forbiddenID, ""))
	}

	// Its replacement becomes (or reuses) a preferred-term concept, joined to the
	// forbidden one by USE_INSTEAD. A rule with no replacement is a pure ban — no
	// preferred concept, no relation.
	if replacement := strings.TrimSpace(rule.Replacement); replacement != "" {
		replacementID, rCreated, err := upsertBrandVocabConcept(ctx, tb, replacement, locale, model.TermPreferred)
		if err != nil {
			return forbiddenID, events, err
		}
		if rCreated {
			events = append(events, conceptEvent(knowledge.EventConceptCreated, wsID, replacementID, ""))
		}
		added, err := ensureUseInstead(ctx, tb, forbiddenID, replacementID)
		if err != nil {
			return forbiddenID, events, err
		}
		if added {
			events = append(events, conceptEvent(knowledge.EventConceptRelationAdded, wsID, forbiddenID, ""))
		}
	}

	return forbiddenID, events, nil
}

// brandConceptLocale resolves the source locale to tag a brand concept's terms
// with: the first matching workspace project's default source language, falling
// back to English when no project (or no content store) is available.
func (s *Server) brandConceptLocale(ctx context.Context, wsID string) model.LocaleID {
	if s.ContentStore != nil {
		if loc := firstWorkspaceSourceLocale(ctx, s.ContentStore, wsID); loc != "" {
			return loc
		}
	}
	return defaultBrandConceptLocale
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

// upsertBrandVocabConcept finds an existing brand-vocabulary concept carrying a
// term with the given text (case-insensitive) and status, or creates a single-
// term concept when none exists. It reports the concept ID and whether it was
// freshly created.
func upsertBrandVocabConcept(ctx context.Context, tb termbase.TBStore, text string, locale model.LocaleID, status model.TermStatus) (string, bool, error) {
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return "", false, err
	}
	if existing := findBrandVocabConcept(concepts, text, status); existing != "" {
		return existing, false, nil
	}
	now := time.Now().UTC()
	c := termbase.Concept{
		ID:     id.New(),
		Source: termbase.TermSourceBrandVocabulary,
		Terms: []termbase.Term{{
			Text:   text,
			Locale: locale,
			Status: status,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := tb.AddConcept(ctx, c); err != nil {
		return "", false, err
	}
	return c.ID, true, nil
}

// findBrandVocabConcept returns the ID of a brand-vocabulary concept that holds a
// term matching text (case-insensitive) at the given status, or "" if none does.
func findBrandVocabConcept(concepts []termbase.Concept, text string, status model.TermStatus) string {
	for _, c := range concepts {
		if conceptSource(c) != termbase.TermSourceBrandVocabulary {
			continue
		}
		for _, t := range c.Terms {
			if t.Status == status && strings.EqualFold(strings.TrimSpace(t.Text), strings.TrimSpace(text)) {
				return c.ID
			}
		}
	}
	return ""
}

// ensureUseInstead adds a USE_INSTEAD relation from the forbidden concept to its
// replacement, unless an equivalent edge already exists. It reports whether a new
// relation was added.
func ensureUseInstead(ctx context.Context, tb termbase.TBStore, sourceID, targetID string) (bool, error) {
	rels, err := tb.RelationsOf(ctx, sourceID, nil)
	if err != nil {
		return false, err
	}
	for _, r := range rels {
		if r.RelationType == graph.LabelUseInstead && r.SourceID == sourceID && r.TargetID == targetID {
			return false, nil
		}
	}
	rel := termbase.ConceptRelation{
		ID:           id.New(),
		SourceID:     sourceID,
		TargetID:     targetID,
		RelationType: graph.LabelUseInstead,
		CreatedAt:    time.Now().UTC(),
	}
	if err := tb.AddRelation(ctx, rel); err != nil {
		return false, err
	}
	return true, nil
}
