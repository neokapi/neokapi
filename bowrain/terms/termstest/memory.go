package termstest

import (
	"context"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/terms"
)

// Memory is an in-memory terms store with the write rules of the PostgreSQL
// one (sqlterms.WithRevisions). Streams are ignored.
func Memory() sqlterms.Store {
	return sqlterms.WithRevisions(&streamless{fw.NewInMemoryStore()})
}

// streamless gives an in-memory store the stream methods of fw.Store, each
// reading or writing the one stream it has.
type streamless struct{ *fw.InMemoryStore }

func (s *streamless) AddConceptWithStream(ctx context.Context, concept fw.Concept, _ string) error {
	return s.AddConcept(ctx, concept)
}

func (s *streamless) SearchForStream(ctx context.Context, query string, source, target model.LocaleID, _ string, _ []string, offset, limit int) ([]fw.Concept, int, error) {
	return s.Search(ctx, query, source, target, offset, limit)
}

func (s *streamless) AddRelationWithStream(ctx context.Context, rel fw.ConceptRelation, _ string) error {
	return s.AddRelation(ctx, rel)
}

func (s *streamless) RelationsForStream(ctx context.Context, conceptID, _ string, _ []string, scope *graph.Scope) ([]fw.ConceptRelation, error) {
	return s.RelationsOf(ctx, conceptID, scope)
}
