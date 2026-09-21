package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/host"
)

// openProjectStore opens a project's store the way a command does: through an
// App, which binds the project to the workspace holding its terms, voice
// profiles, content memory and decisions.
//
// Tests seed and inspect through here. A store opened straight from
// core/projectdb has no workspace, so it keeps its context tables beside the
// projection in the checkout and no command reads them.
func openProjectStore(t *testing.T, root string) *projectdb.DB {
	t.Helper()
	a := &host.App{}
	t.Cleanup(a.Shutdown)
	db, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	return db
}

// withProjectStore opens a project's store through an App, hands it to fn, and
// releases it. It is openProjectStore for a caller that writes and is done.
func withProjectStore(t *testing.T, root string, fn func(db *projectdb.DB)) {
	t.Helper()
	a := &host.App{}
	defer a.Shutdown()
	db, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	fn(db)
}
