package change

import (
	"context"
	"slices"

	"github.com/neokapi/neokapi/core/model"
)

// An edit made outside kapi (a person's editor saving a file, a git checkout)
// records nothing when it happens. kapi learns of it on the next read: the
// revision an edition holds ends no chain of recorded changes, because the
// last recorded change to it left another revision. A read hands every block
// it shows to the service's Observer, which compares each edition with its
// record of the last change to it and records the transition it finds as
// observed (design 5.7). The record is what keeps "who last wrote this" and a
// translation's basis true after such an edit: the edition's last change is
// one whose author nobody knows and which made the text from no recorded
// source.

// Observer learns what a read finds. A host implements it over its record of
// applied changes (kapi's block history). Without one a read records nothing.
type Observer interface {
	// Observe starts following one read of doc. It returns nil when it has
	// no record to compare the read with.
	Observe(ctx context.Context, doc DocInfo) Observation
}

// Observation is one read as an Observer follows it.
type Observation interface {
	// Saw is called for each block the read shows, with the editions the read
	// covers in it: every edition the block holds and every edition the read
	// asked for, held or not. The block is read-only.
	Saw(b *model.Block, editions []model.EditionKey)
	// Done is called once, when the read has shown its last block. A read
	// that fails calls it too, after the blocks it showed.
	Done(ctx context.Context)
}

// WithObserver sets what a read shows its blocks to.
func WithObserver(o Observer) Option { return func(s *Service) { s.observer = o } }

// unobservedKey marks the context of a read the caller follows itself.
type unobservedKey struct{}

// Unobserved returns ctx for a read the Observer does not follow: a read of a
// document its reader has just written, which the reader is about to record
// itself (a flow reading back what it committed).
func Unobserved(ctx context.Context) context.Context {
	return context.WithValue(ctx, unobservedKey{}, true)
}

// observing reports whether a read with ctx is shown to the observer.
func observing(ctx context.Context) bool {
	v, _ := ctx.Value(unobservedKey{}).(bool)
	return !v && !Previewing(ctx)
}

// observe starts following a read of doc, or returns nil.
func (s *Service) observe(ctx context.Context, doc DocInfo) Observation {
	if s.observer == nil || !observing(ctx) {
		return nil
	}
	return s.observer.Observe(ctx, doc)
}

// coveredEditions lists the editions a read covers in b: the ones it holds and
// the ones the read asked for.
func coveredEditions(b *model.Block, asked []model.EditionKey) []model.EditionKey {
	out := b.Editions()
	for _, k := range asked {
		if k = b.EditionKeyOf(k); !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}
