package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
)

// An empty collection id on the way in means "unspecified", and what it
// resolves to depends on whether the item is already filed somewhere. A first
// store falls to the project's default collection — every project created
// through the API has one (#1786). A re-store does not: the caller that could
// not name a collection this time (a transient lookup failure on a re-push)
// must not move content out of the collection it was filed under.
//
// Both store backends resolve this in the upsert, on the raw incoming id.
// Resolving the default before the statement runs — which is what both did —
// hands the upsert a non-empty id, and the existing binding is overwritten by
// a lookup failure.
func runStoreItemCollectionBindingCases(t *testing.T, s platstore.ContentStore) {
	t.Helper()

	const (
		wantDefault = "default"
		wantDocs    = "docs"
		wantSite    = "site"
	)

	tests := []struct {
		name string
		// bound files the item under "docs" before the store call under test.
		bound bool
		// incoming is the collection the store call under test carries:
		// "docs"/"site", or "" for an id the caller could not resolve.
		incoming string
		want     string
	}{
		{
			name:     "a first store with no collection falls to the default",
			incoming: "",
			want:     wantDefault,
		},
		{
			name:     "a first store names its collection",
			incoming: wantSite,
			want:     wantSite,
		},
		{
			name:     "a re-store that could not resolve a collection keeps the binding",
			bound:    true,
			incoming: "",
			want:     wantDocs,
		},
		{
			name:     "a re-store that names another collection moves the item",
			bound:    true,
			incoming: wantSite,
			want:     wantSite,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()

			// A project per case, so one case's bindings cannot answer another's.
			projectID := fmt.Sprintf("binding-%d", i)
			require.NoError(t, s.CreateProject(ctx, &platstore.Project{ID: projectID, Name: "Binding"}))

			ids := map[string]string{}
			for _, c := range []struct {
				key       string
				name      string
				isDefault bool
			}{
				{wantDefault, "Default", true},
				{wantDocs, "Docs", false},
				{wantSite, "Site", false},
			} {
				coll := &platstore.Collection{
					ID:        fmt.Sprintf("c-%s-%d", c.key, i),
					ProjectID: projectID,
					Name:      c.name,
					IsDefault: c.isDefault,
				}
				require.NoError(t, s.CreateCollection(ctx, coll))
				ids[c.key] = coll.ID
			}
			require.NotEqual(t, ids[wantDefault], ids[wantDocs])

			if tc.bound {
				require.NoError(t, s.StoreItem(ctx, projectID, "main",
					&platstore.Item{Name: "guide.md", Format: "markdown", CollectionID: ids[wantDocs]}))
				before, err := s.GetItem(ctx, projectID, "main", "guide.md")
				require.NoError(t, err)
				require.Equal(t, ids[wantDocs], before.CollectionID, "the item must start out filed under Docs")
			}

			var incomingID string
			if tc.incoming != "" {
				incomingID = ids[tc.incoming]
			}
			require.NoError(t, s.StoreItem(ctx, projectID, "main",
				&platstore.Item{Name: "guide.md", Format: "markdown", CollectionID: incomingID}))

			got, err := s.GetItem(ctx, projectID, "main", "guide.md")
			require.NoError(t, err)
			assert.Equal(t, ids[tc.want], got.CollectionID,
				"the stored collection id is what governs the item")
		})
	}
}

func TestStoreItemCollectionBinding_SQLite(t *testing.T) {
	s, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	runStoreItemCollectionBindingCases(t, s)
}

func TestStoreItemCollectionBinding_Postgres(t *testing.T) {
	db := pgtest.NewTestDB(t)
	s, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	runStoreItemCollectionBindingCases(t, s)
}
