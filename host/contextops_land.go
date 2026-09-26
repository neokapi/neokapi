package host

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projector"
)

// land writes an established rule where the subsystems that read context
// already look.
//
// A project-scoped rule goes through the same appliers `kapi apply` uses: into
// the project's terms store or the project's content memory. A term rule with
// several forms to avoid lands each form, and a form that differs from the
// form to use only in case stays out of the store, which folds case. Every reader downstream, from a check to the
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
		w, err := s.writer(ctx, r)
		if err != nil {
			return "", err
		}
		if err := contextop.Widen(ctx, w.Rules(), r); err != nil {
			return "", err
		}
		return "the whole workspace", nil
	}
	landed := ""
	for _, entry := range s.assetEntries(r) {
		res := s.app.applyAssetEntry(ctx, s.cmd, entry)
		if res.Status == "error" {
			return "", fmt.Errorf("keep %s: %s", contextop.ShortID(r.ID), res.Detail)
		}
		if res.Detail != "" && res.Status == "applied" {
			landed = res.Detail
		}
	}
	if landed == "" {
		landed = landedTerms
		if r.Subject.Kind == contextop.SubjectMemory {
			landed = landedMemory
		}
	}
	return landed, nil
}

// settled returns an operation as the log now folds it, after settling what
// recording it changed: a correction that contests an established rule takes
// the rule out of force, and setting that correction aside puts it back.
func (s *contextOpsSession) settled(ctx context.Context, written contextop.Record, before []contextop.Record) (ContextOperation, error) {
	moved, err := s.reconcile(ctx, before)
	if err != nil {
		return ContextOperation{}, err
	}
	out := ContextOperation{Record: written, Landed: strings.Join(moved, "; ")}
	if now, gerr := s.ledger.Get(ctx, written.ID); gerr == nil {
		out.Status, out.ContestedBy, out.Established = now.Status, now.ContestedBy, now.Established
	}
	return out, nil
}

// reconcile brings the stores in line with the rules whose standing moved
// since before was read. An established rule a correction now contests is
// taken out of the store it was written to, so it advises instead of failing
// a check; a contested rule that is established again is written back.
func (s *contextOpsSession) reconcile(ctx context.Context, before []contextop.Record) ([]string, error) {
	after, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Subjects: true})
	if err != nil {
		return nil, err
	}
	was := make(map[string]contextop.Record, len(before))
	for _, r := range before {
		was[r.ID] = r
	}
	var moved []string
	for _, r := range after {
		prior, held := was[r.ID]
		if !held || !r.Established || !prior.Established {
			continue
		}
		binding := r.Status == contextop.StatusEstablished
		wasBinding := prior.Status == contextop.StatusEstablished
		switch {
		case wasBinding && !binding:
			r.Status = contextop.StatusEstablished
			where, rerr := s.retract(ctx, r)
			if rerr != nil {
				return nil, rerr
			}
			if where != "" {
				moved = append(moved, fmt.Sprintf("#%s is contested and taken out of %s", contextop.ShortID(r.ID), where))
			}
		case !wasBinding && binding:
			where, lerr := s.land(ctx, r)
			if lerr != nil {
				return nil, lerr
			}
			moved = append(moved, fmt.Sprintf("#%s is established again in %s", contextop.ShortID(r.ID), where))
		}
	}
	return moved, nil
}

// writer is the project's projector, stamping what it writes with the
// operation that caused it.
func (s *contextOpsSession) writer(ctx context.Context, r contextop.Record) (*projector.Projector, error) {
	w, err := s.app.Projector(ctx, s.root)
	if err != nil {
		return nil, err
	}
	return w.With(projector.Origin{By: string(r.Kind), Cause: r.ID}), nil
}

// retract takes a rule back out of wherever keeping put it. A suggestion
// nobody kept put nothing anywhere, so retracting it is a no-op: the rule
// stops advising the moment the log says so.
func (s *contextOpsSession) retract(ctx context.Context, r contextop.Record) (string, error) {
	if r.Status != contextop.StatusEstablished {
		return "", nil
	}
	w, err := s.writer(ctx, r)
	if err != nil {
		return "", err
	}
	if err := contextop.Narrow(ctx, w.Rules(), r.Project, r.ID); err != nil {
		return "", err
	}
	if r.Scope.Level == contextop.LevelWorkspace {
		return "the whole workspace", nil
	}
	return s.retractFromProject(ctx, r)
}

// retractFromProject removes a rule from the store a keep wrote it to.
func (s *contextOpsSession) retractFromProject(ctx context.Context, r contextop.Record) (string, error) {
	switch {
	case r.Subject.Kind == contextop.SubjectTerm && r.Subject.Term != nil:
		where := ""
		for _, form := range storedForms(*r.Subject.Term) {
			rule := *r.Subject.Term
			rule.Term = form
			out, err := s.retractTerm(ctx, rule)
			if err != nil {
				return "", err
			}
			if out != "" {
				where = out
			}
		}
		return where, nil
	case r.Subject.Kind == contextop.SubjectMemory && r.Subject.Memory != nil:
		return s.retractMemoryPair(ctx, *r.Subject.Memory)
	}
	return "", nil
}

// assetEntries renders an established rule as the change-set entries `kapi
// apply` takes, so keeping and applying reach the store by one path: one entry
// per form a term rule avoids, or the term alone as the form the project uses
// when the rule avoids nothing.
func (s *contextOpsSession) assetEntries(r contextop.Record) []changeEntry {
	switch {
	case r.Subject.Kind == contextop.SubjectTerm && r.Subject.Term != nil:
		rule := *r.Subject.Term
		if rule.Replacement == "" {
			return []changeEntry{{
				Kind:   kindTerm,
				Op:     "upsert",
				Term:   rule.Term,
				Locale: s.sourceLocale(),
				Status: string(model.TermPreferred),
			}}
		}
		forms := storedForms(rule)
		out := make([]changeEntry, 0, len(forms))
		for _, form := range forms {
			out = append(out, changeEntry{
				Kind:        kindTerm,
				Op:          "upsert",
				Term:        form,
				Replacement: rule.Replacement,
				Locale:      s.sourceLocale(),
				Status:      string(model.TermForbidden),
				Advisory:    rule.Advisory,
				Competitor:  rule.Competitor,
			})
		}
		return out
	case r.Subject.Kind == contextop.SubjectMemory && r.Subject.Memory != nil:
		pair := *r.Subject.Memory
		return []changeEntry{{
			Kind:         kindMemory,
			Op:           "add",
			Source:       pair.Source,
			Target:       pair.Target,
			SourceLocale: pair.SourceLocale,
			TargetLocale: pair.TargetLocale,
		}}
	}
	return nil
}

// storedForms lists the forms of a term rule the terms store can hold. The
// store folds case, so a form that differs from the form to use only in case
// would land on the preferred term itself; such a form stays in the rule and
// out of the store.
func storedForms(rule coreprofile.TermRule) []string {
	var out []string
	for _, form := range append([]string{rule.Term}, rule.Forms...) {
		if form == "" || (rule.Replacement != "" && strings.EqualFold(form, rule.Replacement)) {
			continue
		}
		out = append(out, form)
	}
	return out
}

// sourceLocale is the language an established term is recorded in: the project's
// own, which is the language its source content is written in.
func (s *contextOpsSession) sourceLocale() string {
	if loc := s.app.SourceLocale(); loc != "" {
		return loc
	}
	return "en"
}

// retractTerm removes a term from the project's terms store.
//
// The term goes and the concept stands, because a term kept into a concept
// that already existed must leave that concept's other terms where they are. A
// concept the term was alone in goes with it.
func (s *contextOpsSession) retractTerm(ctx context.Context, rule coreprofile.TermRule) (string, error) {
	w, err := s.app.Projector(ctx, s.root)
	if err != nil {
		return "", err
	}
	store := w.With(projector.Origin{By: "retract"}).Terms()
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

// retractMemoryPair removes a pair from the project's content memory.
func (s *contextOpsSession) retractMemoryPair(ctx context.Context, pair contextop.MemoryPair) (string, error) {
	w, err := s.app.Projector(ctx, s.root)
	if err != nil {
		return "", err
	}
	store := w.With(projector.Origin{By: "retract"}).Memory()
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
