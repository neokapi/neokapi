package host

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
)

// land writes a confirmed rule where the subsystems that read context already
// look.
//
// A project-scoped rule goes through the same appliers `kapi apply` uses: into
// the project's terms store, the voice profile the recipe binds by name, or the
// project's content memory. Every reader downstream, from a check to the
// governing fingerprint, sees it without being taught anything new. The log is
// the history, and the store is where the rule lives.
//
// A rule widened to the workspace has no project to live in, so it goes into
// the workspace's rule store instead, and the project copy is taken back out:
// one rule, one home.
func (s *contextOpsSession) land(ctx context.Context, r contextop.Record) (string, error) {
	if r.Scope.Level == contextop.LevelWorkspace {
		if _, err := s.retractFromProject(ctx, r); err != nil {
			return "", err
		}
		if err := contextop.Widen(ctx, s.ws, r); err != nil {
			return "", err
		}
		return "the whole workspace", nil
	}
	entry, ok := s.assetEntry(r)
	if !ok {
		return "", nil
	}
	res := s.app.applyAssetEntry(ctx, s.cmd, entry)
	if res.Status == "error" {
		return "", fmt.Errorf("confirm %s: %s", r.ID, res.Detail)
	}
	return res.Detail, nil
}

// retract takes a rule back out of wherever confirming put it. A candidate
// nobody confirmed put nothing anywhere, so retracting it is a no-op: the rule
// stops advising the moment the log says so.
func (s *contextOpsSession) retract(ctx context.Context, r contextop.Record) (string, error) {
	if r.Status != contextop.StatusConfirmed {
		return "", nil
	}
	if err := contextop.Narrow(ctx, s.ws, r.Project, r.ID); err != nil {
		return "", err
	}
	if r.Scope.Level == contextop.LevelWorkspace {
		return "the whole workspace", nil
	}
	return s.retractFromProject(ctx, r)
}

// retractFromProject removes a rule from the store a confirmation wrote it to.
func (s *contextOpsSession) retractFromProject(ctx context.Context, r contextop.Record) (string, error) {
	switch r.Subject.Kind {
	case contextop.SubjectTerm:
		return s.retractTerm(ctx, *r.Subject.Term)
	case contextop.SubjectVoice:
		return s.retractVoiceRule(ctx, *r.Subject.Voice)
	case contextop.SubjectMemory:
		return s.retractMemoryPair(ctx, *r.Subject.Memory)
	}
	return "", nil
}

// assetEntry renders a confirmed rule as the change-set entry `kapi apply`
// takes, so confirming and applying reach the store by one path.
func (s *contextOpsSession) assetEntry(r contextop.Record) (changeEntry, bool) {
	switch r.Subject.Kind {
	case contextop.SubjectTerm:
		rule := *r.Subject.Term
		return changeEntry{
			Kind:        kindTerm,
			Op:          "upsert",
			Term:        rule.Term,
			Replacement: rule.Replacement,
			Locale:      s.sourceLocale(),
			Status:      string(model.TermForbidden),
		}, true
	case contextop.SubjectVoice:
		voice := *r.Subject.Voice
		return changeEntry{
			Kind:        kindVoice,
			Op:          "add-rule",
			List:        voice.List,
			Term:        voice.Rule.Term,
			Replacement: voice.Rule.Replacement,
			Severity:    voice.Rule.Severity,
		}, true
	case contextop.SubjectMemory:
		pair := *r.Subject.Memory
		return changeEntry{
			Kind:         kindMemory,
			Op:           "add",
			Source:       pair.Source,
			Target:       pair.Target,
			SourceLocale: pair.SourceLocale,
			TargetLocale: pair.TargetLocale,
		}, true
	}
	return changeEntry{}, false
}

// sourceLocale is the language a confirmed term is recorded in: the project's
// own, which is the language its source content is written in.
func (s *contextOpsSession) sourceLocale() string {
	if loc := s.app.SourceLocale(); loc != "" {
		return loc
	}
	return "en"
}

// retractTerm removes a term from the project's terms store.
//
// The term goes and the concept stands, because a term confirmed into a concept
// that already existed must leave that concept's other terms where they are. A
// concept the term was alone in goes with it.
func (s *contextOpsSession) retractTerm(ctx context.Context, rule coreprofile.TermRule) (string, error) {
	db, err := s.app.ProjectDB(ctx, s.root)
	if err != nil {
		return "", err
	}
	store := db.Terms()
	if store == nil {
		return "", nil
	}
	concepts, err := store.Concepts(ctx)
	if err != nil {
		return "", fmt.Errorf("read the project's terms: %w", err)
	}
	locale := model.NormalizeLocale(model.LocaleID(s.sourceLocale()))
	ci := indexOfTerm(concepts, rule.Term, locale)
	if ci < 0 {
		return "", nil
	}
	concept := concepts[ci]
	ti := termIndex(&concept, rule.Term, locale)
	concept.Terms = append(concept.Terms[:ti], concept.Terms[ti+1:]...)
	if len(concept.Terms) == 0 {
		// A concept the store never held is what a retraction of a rule someone
		// removed by hand meets.
		if err := store.DeleteConcept(ctx, concept.ID); err != nil && !strings.Contains(err.Error(), "concept not found") {
			return "", fmt.Errorf("retract term %q: %w", rule.Term, err)
		}
		return landedTerms, nil
	}
	if err := store.AddConcept(ctx, concept); err != nil {
		return "", fmt.Errorf("retract term %q: %w", rule.Term, err)
	}
	return landedTerms, nil
}

// retractVoiceRule removes a rule from the voice profile the recipe binds. A
// project that binds no profile holds no rule to take out.
func (s *contextOpsSession) retractVoiceRule(ctx context.Context, voice contextop.VoiceRule) (string, error) {
	db, err := s.app.ProjectDB(ctx, s.root)
	if err != nil {
		return "", err
	}
	store := db.Voice()
	if store == nil {
		return "", nil
	}
	prof, _, found, err := s.app.loadVoiceAtGovernance(ctx, s.root, store, s.defaultGovernance())
	if err != nil || !found || prof == nil {
		return "", err
	}
	list := voiceRuleList(prof, voice.List)
	kept := make([]coreprofile.TermRule, 0, len(*list))
	removed := false
	for _, rule := range *list {
		if strings.EqualFold(rule.Term, voice.Rule.Term) {
			removed = true
			continue
		}
		kept = append(kept, rule)
	}
	if !removed {
		return "", nil
	}
	*list = kept
	if err := store.UpdateProfile(ctx, prof); err != nil {
		return "", fmt.Errorf("retract voice rule %q: %w", voice.Rule.Term, err)
	}
	return landedVoice + prof.ID, nil
}

// defaultGovernance resolves what governs the project as a whole, which is the
// point a project-scoped rule was confirmed at.
func (s *contextOpsSession) defaultGovernance() *project.ResolvedGovernance {
	rc, err := s.proj.ResolveGovernanceFor(project.GovernancePoint{At: s.app.GovernanceInstant()})
	if err != nil {
		return nil
	}
	return rc
}

// retractMemoryPair removes a pair from the project's content memory.
func (s *contextOpsSession) retractMemoryPair(ctx context.Context, pair contextop.MemoryPair) (string, error) {
	db, err := s.app.ProjectDB(ctx, s.root)
	if err != nil {
		return "", err
	}
	store := db.Memory()
	if store == nil {
		return "", nil
	}
	srcLocale := model.NormalizeLocale(model.LocaleID(pair.SourceLocale))
	if srcLocale == "" {
		srcLocale = model.NormalizeLocale(model.LocaleID(s.sourceLocale()))
	}
	id := memoryEntryID(pair.Source, srcLocale, model.LocaleID(pair.TargetLocale))
	if _, held, err := store.GetEntry(ctx, id); err != nil {
		return "", fmt.Errorf("read the project's content memory: %w", err)
	} else if !held {
		return "", nil
	}
	if err := store.Delete(ctx, id); err != nil {
		return "", fmt.Errorf("retract content-memory pair: %w", err)
	}
	return landedMemory, nil
}
