package terms

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/bowrain/storage"
	fw "github.com/neokapi/neokapi/terms"
)

// ConceptWriter is a terms store whose writes to a stored concept name the
// revision the writer read. A write that names a revision the concept has
// moved on from is refused, so no writer replaces a change it never saw.
type ConceptWriter interface {
	// GetConceptAt returns the concept with the revision a writer names to
	// change it.
	GetConceptAt(ctx context.Context, id string) (fw.Concept, string, bool, error)
	// CreateConcept stores a new concept. An existing one is ErrConceptExists.
	CreateConcept(ctx context.Context, concept fw.Concept, stream string) (string, error)
	// UpdateConcept replaces the concept when it is still at base. A concept
	// that moved is storage.ErrStale; one that is gone is ErrConceptNotFound.
	UpdateConcept(ctx context.Context, concept fw.Concept, stream, base string) (string, error)
	// ConceptRevisions returns the revision of each listed concept that
	// exists, for a view that lists concepts a person may change.
	ConceptRevisions(ctx context.Context, ids []string) (map[string]string, error)
	// DeleteConceptAt removes the concept when it is still at base. A
	// concept that moved is a *StaleConceptError; one that is gone is
	// ErrConceptNotFound.
	DeleteConceptAt(ctx context.Context, id, base string) error
	// DeleteConceptsAt removes each listed concept at its revision (id to
	// revision), skipping one that is gone. When one has moved, nothing is
	// deleted and the answer is a *StaleConceptError.
	DeleteConceptsAt(ctx context.Context, revs map[string]string) error
}

// Store is the workspace terms store the server writes through: the
// framework's store, with concept writes that name a revision.
type Store interface {
	fw.Store
	ConceptWriter
	ImportLedger
}

var _ Store = (*PostgresStore)(nil)

// Derive returns what an automated writer wants a concept to hold, given
// what it holds now (exists is false when there is no concept under the id).
// It returns write false when there is nothing to change. It is called again
// with the new state whenever another write lands first, so it must derive
// from current every time and never return a value it computed before.
type Derive func(current fw.Concept, exists bool) (next fw.Concept, write bool, err error)

// rederiveAttempts bounds Rederive. Each attempt re-reads the concept, so
// running out means other writers changed it this many times in a row.
const rederiveAttempts = 8

// ErrKeptChanging is Rederive's answer when the concept changed under every
// attempt. Nothing was written.
var ErrKeptChanging = errors.New("the concept kept changing while it was being rewritten")

// Rederive is how an automated writer changes one concept. It reads the
// concept with its revision, derives the new value from what it read, and
// writes it at that revision. When another write lands in between, it reads
// the concept again and derives again: it never resends the write it lost,
// so a person's edit made in between stays, with the writer's change derived
// on top of it. It returns the concept it wrote, or the stored one and false
// when derive found nothing to change.
func Rederive(ctx context.Context, w ConceptWriter, id, stream string, derive Derive) (fw.Concept, bool, error) {
	if id == "" {
		return fw.Concept{}, false, errors.New("rederive: concept ID is required")
	}
	for range rederiveAttempts {
		current, rev, exists, err := w.GetConceptAt(ctx, id)
		if err != nil {
			return fw.Concept{}, false, err
		}
		next, write, err := derive(cloneConcept(current), exists)
		if err != nil {
			return fw.Concept{}, false, err
		}
		if !write {
			return current, false, nil
		}
		next.ID = id
		if exists {
			_, err = w.UpdateConcept(ctx, next, stream, rev)
		} else {
			_, err = w.CreateConcept(ctx, next, stream)
		}
		switch {
		case err == nil:
			return next, true, nil
		case errors.Is(err, storage.ErrStale), errors.Is(err, ErrConceptExists), errors.Is(err, ErrConceptNotFound):
			continue
		default:
			return fw.Concept{}, false, err
		}
	}
	return fw.Concept{}, false, fmt.Errorf("concept %s: %w", id, ErrKeptChanging)
}

// cloneConcept copies c deeply enough that a Derive can change what it is
// handed without changing what the store returned.
func cloneConcept(c fw.Concept) fw.Concept {
	c.Properties = maps.Clone(c.Properties)
	c.Terms = slices.Clone(c.Terms)
	for i, t := range c.Terms {
		t.Forms = slices.Clone(t.Forms)
		if t.Validity != nil {
			v := *t.Validity
			v.Tags = maps.Clone(v.Tags)
			t.Validity = &v
		}
		c.Terms[i] = t
	}
	return c
}
