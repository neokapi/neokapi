package termscope

import (
	"context"
	"sync"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// searchPage is the page size a view reads a search in. A view filters a
// search before it pages it, so it reads every page of the underlying one.
const searchPage = 500

// View is the terms of one project: the store's concepts that apply in the
// project (AppliesIn), and the relations between them. Reads see nothing
// else. Writes go to the store unchanged, so a concept written through a view
// keeps the limit it carries. An empty project id returns tb itself.
//
// A view reads which concepts it hides once, on its first lookup, and keeps
// the answer. It is meant for one request or one job, not to be held.
func View(tb terms.Terminology, projectID string) terms.Terminology {
	if tb == nil || projectID == "" {
		return tb
	}
	return &view{Terminology: tb, project: projectID}
}

// StoreView is View for a terms.Store: the stream-aware reads are narrowed
// the same way. The view names the store's revision when the store names
// one, so a cache keyed by the revision must also key by the project.
func StoreView(tb terms.Store, projectID string) terms.Store {
	if tb == nil || projectID == "" {
		return tb
	}
	sv := &storeView{store: tb}
	sv.Terminology, sv.project = tb, projectID
	return sv
}

type view struct {
	terms.Terminology
	project string

	once   sync.Once
	hidden map[string]bool
	err    error
}

// hiddenIDs are the ids of the store's concepts that do not apply in the
// project.
func (v *view) hiddenIDs(ctx context.Context) (map[string]bool, error) {
	v.once.Do(func() {
		concepts, err := v.Terminology.Concepts(ctx)
		if err != nil {
			v.err = err
			return
		}
		v.hidden = map[string]bool{}
		for _, cp := range concepts {
			if !AppliesIn(cp, v.project) {
				v.hidden[cp.ID] = true
			}
		}
	})
	return v.hidden, v.err
}

func (v *view) GetConcept(ctx context.Context, id string) (terms.Concept, bool, error) {
	cp, ok, err := v.Terminology.GetConcept(ctx, id)
	if err != nil || !ok || AppliesIn(cp, v.project) {
		return cp, ok, err
	}
	return terms.Concept{}, false, nil
}

func (v *view) Concepts(ctx context.Context) ([]terms.Concept, error) {
	concepts, err := v.Terminology.Concepts(ctx)
	if err != nil {
		return nil, err
	}
	return Filter(concepts, v.project), nil
}

func (v *view) Count(ctx context.Context) (int, error) {
	concepts, err := v.Concepts(ctx)
	return len(concepts), err
}

func (v *view) Lookup(ctx context.Context, sourceText string, opts terms.LookupOptions) ([]terms.TermMatch, error) {
	return v.matches(ctx, v.Terminology.Lookup, sourceText, opts)
}

func (v *view) LookupAll(ctx context.Context, sourceText string, opts terms.LookupOptions) ([]terms.TermMatch, error) {
	return v.matches(ctx, v.Terminology.LookupAll, sourceText, opts)
}

// matches runs a lookup and drops the matches of hidden concepts. A match
// carries its concept without properties, so the hidden ids decide.
func (v *view) matches(ctx context.Context, lookup func(context.Context, string, terms.LookupOptions) ([]terms.TermMatch, error), sourceText string, opts terms.LookupOptions) ([]terms.TermMatch, error) {
	found, err := lookup(ctx, sourceText, opts)
	if err != nil || len(found) == 0 {
		return found, err
	}
	hidden, err := v.hiddenIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]terms.TermMatch, 0, len(found))
	for _, m := range found {
		if !hidden[m.Concept.ID] {
			out = append(out, m)
		}
	}
	return out, nil
}

func (v *view) Search(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]terms.Concept, int, error) {
	return v.search(func(off, lim int) ([]terms.Concept, int, error) {
		return v.Terminology.Search(ctx, query, sourceLocale, targetLocale, off, lim)
	}, offset, limit)
}

// search reads every page of an underlying search, keeps the concepts that
// apply in the project, and pages what is left, so the total and the pages
// agree with each other.
func (v *view) search(page func(offset, limit int) ([]terms.Concept, int, error), offset, limit int) ([]terms.Concept, int, error) {
	var kept []terms.Concept
	for off := 0; ; {
		got, total, err := page(off, searchPage)
		if err != nil {
			return nil, 0, err
		}
		kept = append(kept, Filter(got, v.project)...)
		off += len(got)
		if len(got) == 0 || off >= total {
			break
		}
	}
	total := len(kept)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []terms.Concept{}, total, nil
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return kept[offset:end], total, nil
}

func (v *view) RelationsOf(ctx context.Context, conceptID string, scope *graph.Scope) ([]terms.ConceptRelation, error) {
	rels, err := v.Terminology.RelationsOf(ctx, conceptID, scope)
	if err != nil {
		return nil, err
	}
	return v.relations(ctx, rels)
}

func (v *view) ListRelations(ctx context.Context, scope *graph.Scope) ([]terms.ConceptRelation, error) {
	rels, err := v.Terminology.ListRelations(ctx, scope)
	if err != nil {
		return nil, err
	}
	return v.relations(ctx, rels)
}

// relations drops the relations that touch a hidden concept.
func (v *view) relations(ctx context.Context, rels []terms.ConceptRelation) ([]terms.ConceptRelation, error) {
	if len(rels) == 0 {
		return rels, nil
	}
	hidden, err := v.hiddenIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]terms.ConceptRelation, 0, len(rels))
	for _, r := range rels {
		if !hidden[r.SourceID] && !hidden[r.TargetID] {
			out = append(out, r)
		}
	}
	return out, nil
}

type storeView struct {
	view
	store terms.Store
}

func (v *storeView) AddConceptWithStream(ctx context.Context, concept terms.Concept, stream string) error {
	return v.store.AddConceptWithStream(ctx, concept, stream)
}

func (v *storeView) AddRelationWithStream(ctx context.Context, rel terms.ConceptRelation, stream string) error {
	return v.store.AddRelationWithStream(ctx, rel, stream)
}

func (v *storeView) SearchForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]terms.Concept, int, error) {
	return v.search(func(off, lim int) ([]terms.Concept, int, error) {
		return v.store.SearchForStream(ctx, query, sourceLocale, targetLocale, stream, streamChain, off, lim)
	}, offset, limit)
}

func (v *storeView) RelationsForStream(ctx context.Context, conceptID string, stream string, streamChain []string, scope *graph.Scope) ([]terms.ConceptRelation, error) {
	rels, err := v.store.RelationsForStream(ctx, conceptID, stream, streamChain, scope)
	if err != nil {
		return nil, err
	}
	return v.relations(ctx, rels)
}

// Revision is the store's revision, empty when the store names none. The
// project is not part of it: the view reads the same rows at every revision.
func (v *storeView) Revision(ctx context.Context) (string, error) {
	if r, ok := v.store.(interface {
		Revision(ctx context.Context) (string, error)
	}); ok {
		return r.Revision(ctx)
	}
	return "", nil
}
