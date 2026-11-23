package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/memory/schema"
)

// A shipped migration's SQL is frozen as it shipped. The schema descriptors
// still describe the same tables: when one of these fails, the descriptors
// moved, and the change belongs in a new numbered migration, never in the
// frozen text.
func TestFrozenMigrationsMatchTheDescriptors(t *testing.T) {
	assert.Equal(t, memoryPostgresBaseline, schema.RenderMemoryPostgresBaseline("workspace_id"))
	assert.Equal(t, memoryPostgresVersionChain, schema.RenderMemoryPostgresVersionChain("workspace_id"))
	revision := storage.RevisionColumn("tm_entries", 8)
	require.Len(t, revision, 5)
	for i, frozen := range []string{memoryPostgresRevision1, memoryPostgresRevision2, memoryPostgresRevision3, memoryPostgresRevision4, memoryPostgresRevision5} {
		assert.Equal(t, frozen, revision[i].SQL, "revision step %d", i+1)
		assert.Equal(t, revision[i].Description, Migrations[2+i].Description, "revision step %d", i+1)
		assert.Equal(t, revision[i].Version, Migrations[2+i].Version, "revision step %d", i+1)
	}
}
