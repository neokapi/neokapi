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
)

// Separate review workers can update the same project. Each write owns only its keys.
func TestMergeProjectProperties(t *testing.T) {
	for name, open := range map[string]func(*testing.T) platstore.ProjectStore{
		"postgres": func(t *testing.T) platstore.ProjectStore {
			s, err := bstore.NewPostgresStoreFromDB(pgtest.NewTestDB(t))
			require.NoError(t, err)
			return s
		},
		"sqlite": func(t *testing.T) platstore.ProjectStore {
			s, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "project.db"))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Close()) })
			return s
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := open(t)
			ctx := t.Context()
			p := &platstore.Project{Name: "Settings", WorkspaceID: "ws"}
			require.NoError(t, s.CreateProject(ctx, p))
			first, err := s.MergeProjectProperties(ctx, p.ID, map[string]string{"mode": "manual"})
			require.NoError(t, err)
			require.Equal(t, "manual", first.Properties["mode"])

			var g errgroup.Group
			start := make(chan struct{})
			want := map[string]string{"mode": "manual"}
			for i := range 8 {
				key := "setting" + strconv.Itoa(i)
				want[key] = "set"
				g.Go(func() error {
					<-start
					_, err := s.MergeProjectProperties(ctx, p.ID, map[string]string{key: "set"})
					return err
				})
			}
			close(start)
			require.NoError(t, g.Wait())
			got, err := s.GetProject(ctx, p.ID)
			require.NoError(t, err)
			assert.Equal(t, want, got.Properties)
			assert.Equal(t, p.Name, got.Name)
			assert.Equal(t, p.WorkspaceID, got.WorkspaceID)

			_, err = s.MergeProjectProperties(ctx, "absent", map[string]string{"mode": "auto"})
			assert.ErrorIs(t, err, sql.ErrNoRows)
		})
	}
}
