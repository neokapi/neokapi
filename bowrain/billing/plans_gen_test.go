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

// TestLandingPlanCatalogNoDrift is the modelcheck-style alarm: the committed
// landing catalog must be byte-identical to what plans.go produces now. If a
// credit allowance, limit, or plan flag changes in plans.go without a
// regenerated plans.json, this fails and tells the developer exactly how to fix
// it — so the landing facts and the billing source of truth can never silently
// diverge.
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
