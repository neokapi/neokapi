package terms

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/storage"
	termsschema "github.com/neokapi/neokapi/terms/schema"
)

// A shipped migration's SQL is frozen as it shipped. The schema descriptors
// still describe the same tables: when one of these fails, the descriptors
// moved, and the change belongs in a new numbered migration, never in the
// frozen text.
func TestFrozenMigrationsMatchTheDescriptors(t *testing.T) {
	for name, tc := range map[string][2]string{
		"baseline": {termsPostgresBaseline, termsschema.RenderTermsPostgresBaseline("workspace_id")},
		"v6":       {termsPostgresV6, termsschema.RenderTermsPostgresV6()},
		"v7":       {termsPostgresV7, termsschema.RenderTermsPostgresV7()},
		"v8":       {termsPostgresV8, termsschema.RenderTermsPostgresV8()},
		"v10":      {termsPostgresV10, termsschema.RenderTermsPostgresV10()},
	} {
		assert.Equal(t, tc[1], tc[0], name)
	}
}

// The frozen revision steps are the ones storage.RevisionColumn renders for
// tb_concepts. When this fails, the renderer moved, and the change belongs in
// a new numbered migration, never in the frozen text.
func TestFrozenRevisionMigrationsMatchTheRenderer(t *testing.T) {
	revision := storage.RevisionColumn("tb_concepts", 13)
	require.Len(t, revision, 5)
	byVersion := map[int]storage.Migration{}
	for _, m := range Migrations {
		byVersion[m.Version] = m
	}
	for _, want := range revision {
		got, ok := byVersion[want.Version]
		require.True(t, ok, "version %d is listed", want.Version)
		assert.Equal(t, want.Description, got.Description, "version %d", want.Version)
		assert.Equal(t, want.SQL, got.SQL, "version %d", want.Version)
	}
}
