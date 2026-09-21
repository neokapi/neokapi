package host

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/memory"
)

// land writes a confirmed rule where the subsystems that read context already
// look.
//
// A project-scoped rule goes through the same appliers `kapi apply` uses: into
// the committed source the recipe binds, and from there into the project's
// terms store, voice profile or content memory. Every reader downstream, from a
// check to `kapi context snapshot` to the governing fingerprint, sees it
// without being taught anything new, and `git diff` shows what changed.
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

// retractFromProject removes a rule from the committed source the recipe binds
// and from the store that source compiles into.
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

// retractTerm removes a term from the committed terms source and rebuilds the
// concept it sat in.
//
// The concept is deleted from the store and re-imported from the source rather
// than edited in place: a term added to a concept that already existed must
// leave that concept's other terms standing, and re-importing what the source
// still declares is the only way to be sure of that.
func (s *contextOpsSession) retractTerm(ctx context.Context, rule coreprofile.TermRule) (string, error) {
	srcPath, err := s.app.ensureTermsSourceBinding(s.recipe, s.root)
	if err != nil {
		return "", err
	}
	file, err := loadKTBFile(srcPath)
	if err != nil {
		return "", err
	}
	locale := model.NormalizeLocale(model.LocaleID(s.sourceLocale()))
	ci := indexOfTerm(file.Concepts, rule.Term, locale)
	if ci < 0 {
		return "", nil
	}
	conceptID := file.Concepts[ci].ID
	ti := termIndex(&file.Concepts[ci], rule.Term, locale)
	file.Concepts[ci].Terms = append(file.Concepts[ci].Terms[:ti], file.Concepts[ci].Terms[ti+1:]...)
	if len(file.Concepts[ci].Terms) == 0 {
		file.Concepts = append(file.Concepts[:ci], file.Concepts[ci+1:]...)
	}
	if _, err := writeKTB(srcPath, file); err != nil {
		return "", err
	}

	db, err := s.app.ProjectDB(ctx, s.root)
	if err != nil {
		return "", err
	}
	if store := db.Terms(); store != nil {
		// A concept the store never held is what a retraction of a rule someone
		// removed by hand meets, and the source is already correct.
		if err := store.DeleteConcept(ctx, conceptID); err != nil && !strings.Contains(err.Error(), "concept not found") {
			return "", fmt.Errorf("retract term %q: %w", rule.Term, err)
		}
	}
	if err := s.app.compileTermsSource(ctx, s.root, srcPath); err != nil {
		return "", err
	}
	return filepath.Base(srcPath), nil
}

// retractVoiceRule removes a rule from the committed voice profile and
// re-imports the profile, which replaces the stored copy wholesale.
func (s *contextOpsSession) retractVoiceRule(ctx context.Context, voice contextop.VoiceRule) (string, error) {
	profilePath, err := s.app.ensureVoiceProfileBinding(s.recipe, s.root)
	if err != nil {
		return "", err
	}
	prof, err := loadOrInitProfile(profilePath, s.root)
	if err != nil {
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
	if err := writeProfileYAML(profilePath, prof); err != nil {
		return "", err
	}
	if err := s.app.compileVoiceProfile(ctx, s.cmd, profilePath); err != nil {
		return "", err
	}
	return filepath.Base(profilePath), nil
}

// retractMemoryPair removes a pair from the committed memory bundle and from
// the project's content memory.
func (s *contextOpsSession) retractMemoryPair(ctx context.Context, pair contextop.MemoryPair) (string, error) {
	srcPath, err := s.app.ensureMemorySourceBinding(s.recipe, s.root)
	if err != nil {
		return "", err
	}
	entries, err := loadKMBEntries(srcPath)
	if err != nil {
		return "", err
	}
	srcLocale := model.NormalizeLocale(model.LocaleID(pair.SourceLocale))
	if srcLocale == "" {
		srcLocale = model.NormalizeLocale(model.LocaleID(s.sourceLocale()))
	}
	id := memoryEntryID(pair.Source, srcLocale, model.LocaleID(pair.TargetLocale))
	kept := make([]memory.Entry, 0, len(entries))
	removed := false
	for _, e := range entries {
		if e.ID == id {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	if !removed {
		return "", nil
	}
	if err := writeKMB(srcPath, kept); err != nil {
		return "", err
	}
	db, err := s.app.ProjectDB(ctx, s.root)
	if err != nil {
		return "", err
	}
	if store := db.Memory(); store != nil {
		if err := store.Delete(ctx, id); err != nil {
			return "", fmt.Errorf("retract content-memory pair: %w", err)
		}
	}
	if err := s.app.compileMemorySource(ctx, s.root, srcPath); err != nil {
		return "", err
	}
	return filepath.Base(srcPath), nil
}
