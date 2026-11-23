// Package termstest helps tests put a concept writer in the race it must win
// or lose correctly.
package termstest

import (
	"context"
	"sync"
	"testing"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	fw "github.com/neokapi/neokapi/terms"
)

// Interleaved returns st with edit run once per concept, right after a writer
// first reads that concept through GetConceptAt: a person's edit landing
// between a writer's read and its write. edit writes through st itself, at the
// revision it reads.
func Interleaved(st sqlterms.Store, edit func(ctx context.Context, st sqlterms.Store, id string)) sqlterms.Store {
	return &interleaved{Store: st, edit: edit, done: map[string]bool{}}
}

type interleaved struct {
	sqlterms.Store
	edit func(ctx context.Context, st sqlterms.Store, id string)
	mu   sync.Mutex
	done map[string]bool
}

func (s *interleaved) GetConceptAt(ctx context.Context, id string) (fw.Concept, string, bool, error) {
	c, rev, ok, err := s.Store.GetConceptAt(ctx, id)
	if err != nil || !ok {
		return c, rev, ok, err
	}
	s.mu.Lock()
	first := !s.done[id]
	s.done[id] = true
	s.mu.Unlock()
	if first {
		s.edit(ctx, s.Store, id)
	}
	return c, rev, ok, err
}

// GetConcept lands the edit once, right after the first read of the
// concept, as GetConceptAt does: a writer that reads without the revision
// meets the same race.
func (s *interleaved) GetConcept(ctx context.Context, id string) (fw.Concept, bool, error) {
	c, ok, err := s.Store.GetConcept(ctx, id)
	if err != nil || !ok {
		return c, ok, err
	}
	s.mu.Lock()
	first := !s.done[id]
	s.done[id] = true
	s.mu.Unlock()
	if first {
		s.edit(ctx, s.Store, id)
	}
	return c, ok, err
}

// ConceptRevisions lands the edit on each listed concept once, right after
// its revision is read: a writer that reads revisions and then content
// (sqlterms.Rewrite) then holds a revision the edit has moved on from.
func (s *interleaved) ConceptRevisions(ctx context.Context, ids []string) (map[string]string, error) {
	revs, err := s.Store.ConceptRevisions(ctx, ids)
	if err != nil {
		return revs, err
	}
	for id := range revs {
		s.mu.Lock()
		first := !s.done[id]
		s.done[id] = true
		s.mu.Unlock()
		if first {
			s.edit(ctx, s.Store, id)
		}
	}
	return revs, nil
}

// SetDefinition is an edit for Interleaved: it sets the concept's definition,
// as a person saving the concept would.
func SetDefinition(t testing.TB, definition string) func(ctx context.Context, st sqlterms.Store, id string) {
	return func(ctx context.Context, st sqlterms.Store, id string) {
		c, rev, ok, err := st.GetConceptAt(ctx, id)
		if err != nil || !ok {
			t.Errorf("termstest: concept %s to edit is not there (%v)", id, err)
			return
		}
		c.Definition = definition
		if _, err := st.UpdateConcept(ctx, c, "", rev); err != nil {
			t.Errorf("termstest: the interleaved edit of %s failed: %v", id, err)
		}
	}
}
