package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleDef(id, name string) *flow.FlowDefinition {
	return &flow.FlowDefinition{
		ID:          id,
		Name:        name,
		Description: "a project flow",
		Nodes: []flow.FlowNode{
			{ID: "reader", Type: flow.NodeReader, Name: "auto", Position: flow.NodePosition{X: 0, Y: 0}},
			{ID: "translate", Type: flow.NodeTool, Name: "translate", Position: flow.NodePosition{X: 250, Y: 0}},
			{ID: "writer", Type: flow.NodeWriter, Name: "auto", Position: flow.NodePosition{X: 500, Y: 0}},
		},
		Edges: []flow.FlowEdge{
			{ID: "e1", Source: "reader", Target: "translate"},
			{ID: "e2", Source: "translate", Target: "writer"},
		},
	}
}

func TestFlowDefStore_CRUD(t *testing.T) {
	s := newTestStore(t)
	store := NewFlowDefStore(s.SQLDB())
	proj := createTestProject(t, s)
	ctx := t.Context()

	// Empty list.
	defs, err := store.List(ctx, proj.ID)
	require.NoError(t, err)
	assert.Empty(t, defs)

	// Upsert (create).
	def := sampleDef("flow-1", "Translate")
	require.NoError(t, store.Upsert(ctx, proj.ID, def))
	assert.Equal(t, "project", def.Source) // Upsert forces source.

	// Get.
	got, err := store.Get(ctx, proj.ID, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, "Translate", got.Name)
	assert.Equal(t, "project", got.Source)
	assert.Len(t, got.Nodes, 3)
	assert.Len(t, got.Edges, 2)
	assert.NotEmpty(t, got.CreatedAt)
	assert.NotEmpty(t, got.ModifiedAt)

	// List has one.
	defs, err = store.List(ctx, proj.ID)
	require.NoError(t, err)
	require.Len(t, defs, 1)
	assert.Equal(t, "flow-1", defs[0].ID)

	// Upsert (update) — same id, new name.
	def2 := sampleDef("flow-1", "Translate v2")
	require.NoError(t, store.Upsert(ctx, proj.ID, def2))
	got, err = store.Get(ctx, proj.ID, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, "Translate v2", got.Name)

	// Still only one row.
	defs, err = store.List(ctx, proj.ID)
	require.NoError(t, err)
	assert.Len(t, defs, 1)

	// Delete.
	require.NoError(t, store.Delete(ctx, proj.ID, "flow-1"))
	_, err = store.Get(ctx, proj.ID, "flow-1")
	require.ErrorIs(t, err, ErrFlowDefNotFound)

	// Delete missing → not found.
	assert.ErrorIs(t, store.Delete(ctx, proj.ID, "nope"), ErrFlowDefNotFound)
}

func TestFlowDefStore_ProjectScoped(t *testing.T) {
	s := newTestStore(t)
	store := NewFlowDefStore(s.SQLDB())
	projA := createTestProject(t, s)
	projB := createTestProject(t, s)
	ctx := t.Context()

	require.NoError(t, store.Upsert(ctx, projA.ID, sampleDef("a", "Flow A")))
	require.NoError(t, store.Upsert(ctx, projB.ID, sampleDef("b", "Flow B")))

	defsA, err := store.List(ctx, projA.ID)
	require.NoError(t, err)
	require.Len(t, defsA, 1)
	assert.Equal(t, "a", defsA[0].ID)

	// projB's flow is not visible from projA.
	_, err = store.Get(ctx, projA.ID, "b")
	assert.ErrorIs(t, err, ErrFlowDefNotFound)
}

// Saves based on the same revision race; exactly one is written and the rest
// are refused, because the check and the write are one statement.
func TestFlowDefStore_SaveBasedOnTheSameRevision(t *testing.T) {
	s := newTestStore(t)
	store := NewFlowDefStore(s.SQLDB())
	proj := createTestProject(t, s)
	ctx := t.Context()
	require.NoError(t, store.Create(ctx, proj.ID, sampleDef("flow-1", "Translate")))

	const writers = 8
	errs := make(chan error, writers)
	for i := range writers {
		go func() {
			def := sampleDef("flow-1", fmt.Sprintf("edit %d", i))
			errs <- store.Save(ctx, proj.ID, def, 1)
		}()
	}
	var won, refused int
	for range writers {
		switch err := <-errs; {
		case err == nil:
			won++
		case errors.Is(err, ErrFlowDefChanged):
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	assert.Equal(t, 1, won)
	assert.Equal(t, writers-1, refused)
	got, err := store.Get(ctx, proj.ID, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, int64(2), got.Revision)

	require.ErrorIs(t, store.Create(ctx, proj.ID, sampleDef("flow-1", "again")), ErrFlowDefExists)
	require.ErrorIs(t, store.Save(ctx, proj.ID, sampleDef("gone", "x"), 1), ErrFlowDefChanged)
}
