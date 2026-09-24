package billing

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// committedLandingCatalogPath is the plans.json the landing build imports,
// relative to this package directory (bowrain/billing).
const committedLandingCatalogPath = "../web/landing/src/generated/plans.json"

// TestLandingPlanCatalogNoDrift verifies that the committed landing plan catalog
// matches generated output byte for byte. Changes to allowances, limits or flags
// must be followed by regeneration.
func TestLandingPlanCatalogNoDrift(t *testing.T) {
	t.Parallel()

	want, err := MarshalLandingPlanCatalog()
	require.NoError(t, err)

	got, err := os.ReadFile(committedLandingCatalogPath)
	require.NoError(t, err,
		"read committed landing catalog; run `go generate ./...` in the bowrain module")

	assert.Equal(t, string(want), string(got),
		"landing plan catalog is stale: run `go generate ./...` in the bowrain module to regenerate %s from plans.go",
		committedLandingCatalogPath)
}
