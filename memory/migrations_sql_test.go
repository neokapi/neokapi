package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/memory/schema"
)

// A shipped migration's SQL is frozen as it shipped. The schema descriptors
// still describe the same tables: when one of these fails, the descriptors
// moved, and the change belongs in a new numbered migration, never in the
// frozen text.
func TestFrozenMigrationsMatchTheDescriptors(t *testing.T) {
	p := storage.FTSWordTokenizerPlaceholder
	for name, tc := range map[string][2]string{
		"v1": {memorySQLiteV1, schema.RenderMemorySQLiteV1(p)},
		"v2": {memorySQLiteV2, schema.RenderMemorySQLiteV2()},
		"v3": {memorySQLiteV3, schema.RenderMemorySQLiteV3()},
		"v4": {memorySQLiteV4, schema.RenderMemorySQLiteV4()},
		"v5": {memorySQLiteV5, schema.RenderMemorySQLiteV5()},
		"v6": {memorySQLiteV6, schema.RenderMemorySQLiteV6(p)},
	} {
		assert.Equal(t, tc[1], tc[0], name)
	}
}
