package terms

import (
	"context"
	"sync"

	"github.com/neokapi/neokapi/bowrain/storage"
	fw "github.com/neokapi/neokapi/terms"
)

// WithRevisions gives a store without revisions of its own, such as an
// in-memory one, the write rules of PostgresStore: AddConcept creates and
// never replaces, and a concept changes only through UpdateConcept at the
// revision the writer read. Its checks and writes run under one lock, so they
// refuse exactly what the PostgreSQL statements refuse.
func WithRevisions(inner fw.Store) Store {
	return &revisioned{Store: inner, revs: map[string]string{}}
}

type revisioned struct {
	fw.Store
	mu   sync.Mutex
	revs map[string]string
}

// revLocked returns the revision of a stored concept, issuing one for a
// concept written to the inner store directly.
func (r *revisioned) revLocked(id string) string {
	rev, ok := r.revs[id]
	if !ok {
		rev = storage.NewRevision()
		r.revs[id] = rev
	}
	return rev
}

func (r *revisioned) GetConceptAt(ctx context.Context, id string) (fw.Concept, string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok, err := r.Store.GetConcept(ctx, id)
	if err != nil || !ok {
		return fw.Concept{}, "", false, err
	}
	return c, r.revLocked(id), true, nil
}

func (r *revisioned) ConceptRevisions(ctx context.Context, ids []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		_, ok, err := r.Store.GetConcept(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out[id] = r.revLocked(id)
		}
	}
	return out, nil
}

func (r *revisioned) createLocked(ctx context.Context, concept fw.Concept, stream string) (string, error) {
	_, ok, err := r.Store.GetConcept(ctx, concept.ID)
	if err != nil {
		return "", err
	}
	if ok {
		return "", ErrConceptExists
	}
	if err := r.Store.AddConceptWithStream(ctx, concept, stream); err != nil {
		return "", err
	}
	rev := storage.NewRevision()
	r.revs[concept.ID] = rev
	return rev, nil
}

func (r *revisioned) CreateConcept(ctx context.Context, concept fw.Concept, stream string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.createLocked(ctx, concept, stream)
}

func (r *revisioned) UpdateConcept(ctx context.Context, concept fw.Concept, stream, base string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok, err := r.Store.GetConcept(ctx, concept.ID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrConceptNotFound
	}
	if rev := r.revLocked(concept.ID); rev != base {
		return "", &StaleConceptError{Current: current, Revision: rev}
	}
	if err := r.Store.AddConceptWithStream(ctx, concept, stream); err != nil {
		return "", err
	}
	rev := storage.NewRevision()
	r.revs[concept.ID] = rev
	return rev, nil
}

func (r *revisioned) AddConcept(ctx context.Context, concept fw.Concept) error {
	return r.AddConceptWithStream(ctx, concept, "")
}

func (r *revisioned) AddConceptWithStream(ctx context.Context, concept fw.Concept, stream string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.createLocked(ctx, concept, stream)
	return err
}

func (r *revisioned) DeleteConcept(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.Store.DeleteConcept(ctx, id); err != nil {
		return err
	}
	delete(r.revs, id)
	return nil
}
