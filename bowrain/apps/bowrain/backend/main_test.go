package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/termbase"
)

func TestMain(m *testing.M) {
	// Replace the OS keychain with an in-memory mock for the whole package, so
	// no test can ever read or write the developer's real bowrain tokens — even
	// one that forgets to call keyring.MockInit() itself. Defense in depth on
	// top of the per-test mocks and the config-dir-namespaced keyringService.
	keyring.MockInit()

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
	cs, err := sqlitestore.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("create in-memory store: %v", err)
	}
	app := newAppWithStore(cs)
	// Use a temp directory for the TM and TB so tests don't share state.
	tmpDir := t.TempDir()
	app.tmPath = filepath.Join(tmpDir, "test.db")
	tb, err := termbase.NewSQLiteTermBase(filepath.Join(tmpDir, "test-tb.db"))
	if err != nil {
		t.Fatalf("create test termbase: %v", err)
	}
	app.tb = tb
	t.Cleanup(func() {
		if err := app.ServiceShutdown(); err != nil {
			t.Errorf("shutdown app: %v", err)
		}
	})
	return app
}
