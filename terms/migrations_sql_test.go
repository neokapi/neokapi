package terms

import (
	"testing"

	"github.com/stretchr/testify/assert"

	termsschema "github.com/neokapi/neokapi/terms/schema"
)

// A shipped migration's SQL is frozen as it shipped. The schema descriptors
// still describe the same tables: when one of these fails, the descriptors
// moved, and the change belongs in a new numbered migration, never in the
// frozen text.
func TestFrozenMigrationsMatchTheDescriptors(t *testing.T) {
	for name, tc := range map[string][2]string{
		"v1": {termsSQLiteV1, termsschema.RenderTermsSQLiteV1()},
		"v2": {termsSQLiteV2, termsschema.RenderTermsSQLiteV2()},
		"v3": {termsSQLiteV3, termsschema.RenderTermsSQLiteV3()},
		"v4": {termsSQLiteV4, termsschema.RenderTermsSQLiteV4()},
		"v5": {termsSQLiteV5, termsschema.RenderTermsSQLiteV5()},
		"v6": {termsSQLiteV6, termsschema.RenderTermsSQLiteV6()},
		"v7": {termsSQLiteV7, termsschema.RenderTermsSQLiteV7()},
	} {
		assert.Equal(t, tc[1], tc[0], name)
	}
}
