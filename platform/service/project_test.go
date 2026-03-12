package service

import (
	"context"
	"testing"

	bstore "github.com/gokapi/gokapi/bowrain/store"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/platform/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *bstore.SQLiteStore {
	t.Helper()
	s, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProjectServiceCRUD(t *testing.T) {
	s := newTestStore(t)
	svc := NewProjectService(s)
	ctx := context.Background()

	p := &store.Project{
		Name:         "Test",
		SourceLocale: model.LocaleEnglish,
	}
	require.NoError(t, svc.CreateProject(ctx, p))
	assert.NotEmpty(t, p.ID)

	got, err := svc.GetProject(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test", got.Name)

	projects, err := svc.ListProjects(ctx)
	require.NoError(t, err)
	assert.Len(t, projects, 1)

	p.Name = "Updated"
	require.NoError(t, svc.UpdateProject(ctx, p))

	require.NoError(t, svc.DeleteProject(ctx, p.ID))
	projects, _ = svc.ListProjects(ctx)
	assert.Len(t, projects, 0)
}

func TestProjectServiceBlocks(t *testing.T) {
	s := newTestStore(t)
	svc := NewProjectService(s)
	ctx := context.Background()

	p := &store.Project{Name: "Test", SourceLocale: model.LocaleEnglish}
	require.NoError(t, svc.CreateProject(ctx, p))

	b := model.NewBlock("b1", "Hello")
	require.NoError(t, svc.StoreBlocks(ctx, p.ID, []*model.Block{b}))

	got, err := svc.GetBlock(ctx, p.ID, "b1")
	require.NoError(t, err)
	assert.Equal(t, "Hello", got.SourceText())

	blocks, err := svc.GetBlocks(ctx, store.BlockQuery{ProjectID: p.ID})
	require.NoError(t, err)
	assert.Len(t, blocks, 1)
}

func TestProjectServiceVersioning(t *testing.T) {
	s := newTestStore(t)
	svc := NewProjectService(s)
	ctx := context.Background()

	p := &store.Project{Name: "Test", SourceLocale: model.LocaleEnglish}
	require.NoError(t, svc.CreateProject(ctx, p))
	require.NoError(t, svc.StoreBlocks(ctx, p.ID, []*model.Block{model.NewBlock("b1", "Hello")}))

	v, err := svc.CreateVersion(ctx, p.ID, "v1", "First")
	require.NoError(t, err)
	assert.Equal(t, "v1", v.Label)

	versions, err := svc.ListVersions(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 1)
}
