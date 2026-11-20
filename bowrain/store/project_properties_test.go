package store_test

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
)

// Separate review workers can update the same project. Each write owns only its keys.
func TestEditProjectPreservesConcurrentChanges(t *testing.T) {
	for name, open := range map[string]func(*testing.T) platstore.ContentStore{
		"postgres": func(t *testing.T) platstore.ContentStore {
			s, err := bstore.NewPostgresStoreFromDB(pgtest.NewTestDB(t))
			require.NoError(t, err)
			return s
		},
		"sqlite": func(t *testing.T) platstore.ContentStore {
			s, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "project.db"))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Close()) })
			return s
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := open(t)
			ctx := t.Context()
			p := &platstore.Project{Name: "Settings", WorkspaceID: "ws", DefaultSourceLanguage: "en"}
			require.NoError(t, s.CreateProject(ctx, p))
			writeProperty := func(id, key, value string) error {
				_, _, err := s.EditProject(ctx, id, func(p *platstore.Project) (bool, error) {
					p.Properties[key] = value
					return true, nil
				})
				return err
			}
			require.NoError(t, writeProperty(p.ID, "mode", "manual"))

			var g errgroup.Group
			start := make(chan struct{})
			want := map[string]string{"mode": "manual"}
			for i := range 8 {
				key := "setting" + strconv.Itoa(i)
				want[key] = "set"
				g.Go(func() error {
					<-start
					return writeProperty(p.ID, key, "set")
				})
			}
			g.Go(func() error {
				<-start
				_, _, err := s.EditProject(ctx, p.ID, func(p *platstore.Project) (bool, error) {
					p.Name = "Renamed"
					return true, nil
				})
				return err
			})
			close(start)
			require.NoError(t, g.Wait())
			got, err := s.GetProject(ctx, p.ID)
			require.NoError(t, err)
			assert.Equal(t, want, got.Properties)
			assert.Equal(t, "Renamed", got.Name)
			assert.Equal(t, p.WorkspaceID, got.WorkspaceID)

			require.NoError(t, s.StoreBlocks(ctx, p.ID, "main", []*model.Block{model.NewBlock("source", "Hello")}))
			_, changed, err := s.EditProject(ctx, p.ID, func(p *platstore.Project) (bool, error) {
				p.DefaultSourceLanguage = "fr"
				p.Name = "Invalid"
				return true, nil
			})
			require.ErrorIs(t, err, platstore.ErrSourceLanguageFixed)
			require.False(t, changed)
			got, err = s.GetProject(ctx, p.ID)
			require.NoError(t, err)
			assert.Equal(t, "Renamed", got.Name)

			err = writeProperty("absent", "mode", "auto")
			assert.ErrorIs(t, err, sql.ErrNoRows)
		})
	}
}
