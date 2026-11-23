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
		"baseline": {termsPostgresBaseline, termsschema.RenderTermsPostgresBaseline("workspace_id")},
		"v6":       {termsPostgresV6, termsschema.RenderTermsPostgresV6()},
		"v7":       {termsPostgresV7, termsschema.RenderTermsPostgresV7()},
		"v8":       {termsPostgresV8, termsschema.RenderTermsPostgresV8()},
		"v10":      {termsPostgresV10, termsschema.RenderTermsPostgresV10()},
	} {
		assert.Equal(t, tc[1], tc[0], name)
	}
}
