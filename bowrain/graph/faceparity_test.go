package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/contextgraph"
	coregraph "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	fwproject "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host/facetest/fixture"
)

// The platform leg of the face parity contract.
//
// The local faces read a term's usage off the uses_term edges their own
// extraction wrote, and host/facetest records the number. The platform's
// concept page reads the same edges off the workspace graph the server writer
// produces, so the record can hold the server to the same count: the fixture's
// extracted blocks are stored as one pushed project, the workspace vocabulary
// is seeded with the fixture's concepts, the writer materializes, and the
// per-term counts read back through contextgraph.UsesByProject are compared
// against what the record says every local face reports.

// usesFacts is the part of a TermFacts row the graph answers for.
type usesFacts struct{ uses, blocks int }

func TestFaceParity_ServerGraphMatchesTheRecord(t *testing.T) {
	p := fixture.Write(t)
	f := newWriterFixture(t)
	ctx := t.Context()
	want := fixture.Golden(t)

	for _, c := range fixture.Concepts() {
		require.NoError(t, f.terms.AddConcept(ctx, c))
	}
	proj := f.pushFixture(t, ctx, p)
	require.Positive(t, f.materialize(t, ctx, proj))

	got := map[[2]string]usesFacts{}
	scope := contextgraph.Scope{Workspace: writerWorkspace, Project: proj.ID}
	for _, c := range fixture.Concepts() {
		rollup, err := contextgraph.UsesByProject(ctx, f.graph, scope, c.ID, coregraph.Scope{})
		require.NoError(t, err)
		for _, pu := range rollup {
			for _, tu := range pu.Terms {
				got[[2]string{c.ID, tu.Term}] = usesFacts{uses: tu.Occurrences, blocks: tu.Blocks}
			}
		}
	}

	require.NotEmpty(t, want.ContextSearch.Terms, "the record has terms to agree about")
	for _, term := range want.ContextSearch.Terms {
		key := [2]string{term.ConceptID, term.Term}
		assert.Equal(t, usesFacts{uses: term.Uses, blocks: term.UseBlocks}, got[key],
			"%s/%q: the server's edges count what the record says every local face counts", term.ConceptID, term.Term)
	}
}

// pushFixture stores the fixture's extracted blocks as one project on the
// server, collection by collection and document by document, the way a push
// lands them: the blocks the local faces counted are the blocks the server
// writer reads.
func (f *writerFixture) pushFixture(t *testing.T, ctx context.Context, p fixture.Project) *platstore.Project {
	t.Helper()
	recipe, err := fwproject.Load(p.Recipe)
	require.NoError(t, err)

	proj := &platstore.Project{
		Name:                  recipe.Name,
		DefaultSourceLanguage: "en",
		WorkspaceID:           writerWorkspace,
		DefaultStream:         writerStream,
		Properties:            map[string]string{},
	}
	require.NoError(t, f.content.CreateProject(ctx, proj))

	collections := map[string]bool{}
	byDocument := map[string][]*model.Block{}
	var order []string
	for _, eb := range fixture.Extract(t, p) {
		collID := proj.ID + "-" + eb.Collection
		if !collections[eb.Collection] {
			collections[eb.Collection] = true
			require.NoError(t, f.content.CreateCollection(ctx, &platstore.Collection{
				ID:        collID,
				ProjectID: proj.ID,
				Name:      eb.Collection,
				Kind:      platstore.CollectionUploaded,
				Stream:    writerStream,
				Context:   map[string]string{},
				Owner:     venue.ContextOwnerRecipe,
			})) //nolint:exhaustruct // the writer reads name, context and stream
		}
		if _, seen := byDocument[eb.Document]; !seen {
			order = append(order, eb.Document)
			require.NoError(t, f.content.StoreItem(ctx, proj.ID, writerStream, &platstore.Item{
				ProjectID: proj.ID, Name: eb.Document, Format: "file", ItemType: "file",
				CollectionID: collID,
			}))
		}
		byDocument[eb.Document] = append(byDocument[eb.Document], &model.Block{
			ID:           eb.Block.ID,
			Translatable: eb.Block.Translatable,
			Source:       eb.Block.Source,
		})
	}
	for _, doc := range order {
		require.NoError(t, f.content.StoreBlocksForItem(ctx, proj.ID, writerStream, doc, byDocument[doc]))
	}
	return proj
}
