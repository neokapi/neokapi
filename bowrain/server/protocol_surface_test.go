package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gate. Every endpoint a shipped client calls has to be registered on both
// route groups, because a plugin reaches whichever one its server URL names.
//
// This is what nothing checked when /push/diff was deleted. The in-tree client
// stopped calling it in the same commit, so TestBowrainClientRoutesAreRegistered
// (which reads the current client) stayed green, and no test spoke for the
// versions already installed elsewhere.
func TestShippedClientEndpointsAreRegistered(t *testing.T) {
	srv, _ := newTestServer(t)

	registered := map[string]bool{}
	for _, r := range srv.GetEcho().Routes() {
		registered[r.Method+" "+r.Path] = true
	}

	for _, ep := range shippedStreamEndpoints {
		flat := ep.Method + " /api/v1/projects/:id/sync/:ref" + ep.Path
		scoped := ep.Method + " /api/v1/:ws/:id/sync/:ref" + ep.Path

		assert.True(t, registered[flat],
			"%s %s is called by %s and the flat route does not serve it; "+
				"a client that reaches it gets a bare 404 naming whichever file it sent first",
			ep.Method, ep.Path, ep.Clients)
		assert.True(t, registered[scoped],
			"%s %s is called by %s and the workspace-scoped route does not serve it",
			ep.Method, ep.Path, ep.Clients)
	}
}

// The table is only as good as its coverage of what the current client does.
// A new endpoint added to the client without a row here would leave the next
// deletion unguarded, so this asserts the table has caught up with the tree.
func TestShippedEndpointsCoverTheCurrentClient(t *testing.T) {
	declared := map[string]bool{}
	for _, ep := range shippedStreamEndpoints {
		declared[ep.Path] = true
	}

	// The paths the in-tree client builds from streamPrefix(), in Echo's
	// parameter form. Kept beside the client rather than derived from it: the
	// point of the table is to outlive the client that motivated each row.
	current := []string{
		"/pull", "/blocks", "/status", "/ref", "/tree",
		"/push/init", "/push/commit", "/push/uploads",
		"/push/chunks/:uploadId/:chunkIndex",
	}
	for _, path := range current {
		assert.True(t, declared[path],
			"the client calls %s but no row declares it; add one, so deleting the route later fails this test",
			path)
	}
}

// A row that names no caller cannot be reasoned about when someone later asks
// whether the route is still needed, which is the question that deletes it.
func TestShippedEndpointRowsNameTheirCallers(t *testing.T) {
	require.NotEmpty(t, shippedStreamEndpoints)
	for _, ep := range shippedStreamEndpoints {
		assert.NotEmpty(t, ep.Clients, "%s %s declares no client versions", ep.Method, ep.Path)
		assert.True(t, strings.HasPrefix(ep.Path, "/"), "%s is not a path", ep.Path)
	}
}
