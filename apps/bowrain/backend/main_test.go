package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/core/store"
)

func TestMain(m *testing.M) {
	// Isolate tests from locally-installed plugins.
	dir, err := os.MkdirTemp("", "bowrain-test-plugins")
	if err != nil {
		panic(err)
	}
	os.Setenv("KAPI_PLUGIN_DIR", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// newTestApp creates a Bowrain backend with an in-memory SQLite store
// and no shared TM/TB state. Each call returns a fully isolated App
// suitable for a single test.
func newTestApp(t *testing.T) *App {
	t.Helper()
	cs, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("create in-memory store: %v", err)
	}
	app := newAppWithStore(cs)
	// Use a temp directory for the TM so tests don't share state.
	tmpDir := t.TempDir()
	app.tmPath = filepath.Join(tmpDir, "test.db")
	t.Cleanup(func() {
		app.ServiceShutdown()
	})
	return app
}
