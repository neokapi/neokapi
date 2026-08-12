package graph

import (
	"testing"

	"github.com/neokapi/neokapi/core/contextgraph/graphtest"
)

// The local arm of the shared query-shape suite. The same table runs against the
// server's Postgres store, which is what makes "the same queries answer locally
// and on the server" a checked claim rather than a stated one.
func TestSQLiteContextGraphQueryShapes(t *testing.T) {
	graphtest.RunQueryShapes(t, newTestStore(t))
}
