package terms

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/storage"
	fw "github.com/neokapi/neokapi/terms"
)

// atRevisions is a fw.Terminology over st for a framework routine that reads
// every concept and writes back the ones it changed (fw.PromoteRule,
// fw.DemoteRule, fw.UpsertDecision callers). Concepts records the revision of
// each concept it returns; AddConcept writes a concept it read at that
// revision, and creates one it did not read. A concept that changed since the
// read refuses the write.
type atRevisions struct {
	Store
	revs map[string]string
}

// Concepts reads the revisions first and the concepts after, so a write
// landing between the two reads leaves a revision older than the content
// read: a later write naming it is refused, and nothing passes over content
// the routine did not see.
func (s *atRevisions) Concepts(ctx context.Context) ([]fw.Concept, error) {
	all, err := s.Store.Concepts(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(all))
	for i, c := range all {
		ids[i] = c.ID
	}
	revs, err := s.ConceptRevisions(ctx, ids)
	if err != nil {
		return nil, err
	}
	concepts, err := s.Store.Concepts(ctx)
	if err != nil {
		return nil, err
	}
	s.revs = revs
	return concepts, nil
}

func (s *atRevisions) GetConcept(ctx context.Context, id string) (fw.Concept, bool, error) {
	c, rev, ok, err := s.GetConceptAt(ctx, id)
	if err == nil && ok {
		s.revs[id] = rev
	}
	return c, ok, err
}

func (s *atRevisions) AddConcept(ctx context.Context, c fw.Concept) error {
	return s.AddConceptWithStream(ctx, c, "")
}

func (s *atRevisions) AddConceptWithStream(ctx context.Context, c fw.Concept, stream string) error {
	var rev string
	var err error
	if base, ok := s.revs[c.ID]; ok {
		rev, err = s.UpdateConcept(ctx, c, stream, base)
	} else {
		rev, err = s.CreateConcept(ctx, c, stream)
	}
	if err != nil {
		return err
	}
	s.revs[c.ID] = rev
	return nil
}

// DeleteConcept deletes a concept the routine read, at the revision it read.
// One that changed since refuses the delete, and Rewrite runs the routine
// again over what the concept holds then.
func (s *atRevisions) DeleteConcept(ctx context.Context, id string) error {
	base, ok := s.revs[id]
	if !ok {
		return fmt.Errorf("delete concept %s: the routine did not read it", id)
	}
	if err := s.DeleteConceptAt(ctx, id, base); err != nil {
		return err
	}
	delete(s.revs, id)
	return nil
}

// Rewrite runs routine, a framework routine that reads concepts and writes
// back the ones it changes, over st so each write names the revision the
// routine read. When another write lands first the routine runs again over
// a fresh read, so it derives its writes from what the concepts hold then:
// it never resends a write it lost, and an edit saved in between stays.
func Rewrite(ctx context.Context, st Store, routine func(fw.Terminology) error) error {
	for range rederiveAttempts {
		err := routine(&atRevisions{Store: st, revs: map[string]string{}})
		switch {
		case err == nil:
			return nil
		case errors.Is(err, storage.ErrStale), errors.Is(err, ErrConceptExists), errors.Is(err, ErrConceptNotFound):
			continue
		default:
			return err
		}
	}
	return fmt.Errorf("rewrite concepts: %w", ErrKeptChanging)
}
