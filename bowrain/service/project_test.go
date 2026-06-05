package service

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *bstore.PostgresStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	s, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return s
}

func TestProjectServiceCRUD(t *testing.T) {
	s := newTestStore(t)
	svc := NewProjectService(s)
	ctx := t.Context()

	p := &store.Project{
		Name:                  "Test",
		DefaultSourceLanguage: model.LocaleEnglish,
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
	assert.Empty(t, projects)
}

func TestProjectServiceBlocks(t *testing.T) {
	s := newTestStore(t)
	svc := NewProjectService(s)
	ctx := t.Context()

	p := &store.Project{Name: "Test", DefaultSourceLanguage: model.LocaleEnglish}
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
	ctx := t.Context()

	p := &store.Project{Name: "Test", DefaultSourceLanguage: model.LocaleEnglish}
	require.NoError(t, svc.CreateProject(ctx, p))
	require.NoError(t, svc.StoreBlocks(ctx, p.ID, []*model.Block{model.NewBlock("b1", "Hello")}))

	v, err := svc.CreateVersion(ctx, p.ID, "v1", "First")
	require.NoError(t, err)
	assert.Equal(t, "v1", v.Label)

	versions, err := svc.ListVersions(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 1)
}
