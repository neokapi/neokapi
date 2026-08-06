package backend

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/terms"
)

// templateStorePath is a content-store database whose schema is already built:
// the migration chain replayed exactly once, for the whole package.
//
// Building that schema is what this suite spends its time on. Under -race the
// pure-Go SQLite parser is instrumented, so parsing the store's migration SQL
// costs ~0.4s — per test, since every test opened its own :memory: store and
// replayed the whole chain. Three quarters of the suite's CPU went into
// building the same schema a couple of hundred times, and the total crossed the
// 120s test timeout on CI (a slower machine than a developer's), which is how a
// suite where no single test is slow, and none hangs, came to fail by timeout.
//
// A file-backed template costs one build and a file copy per test. The copy is
// a fresh database on disk, so tests stay as isolated as they were with
// :memory: — and closer to how the app opens its store in production.
var templateStorePath string

func TestMain(m *testing.M) {
	// Replace the OS keychain with an in-memory mock for the whole package, so
	// no test can ever read or write the developer's real bowrain tokens — even
	// one that forgets to call keyring.MockInit() itself. Defense in depth on
	// top of the per-test mocks.
	keyring.MockInit()

	// Isolate auth from the developer's real ~/.config/bowrain/auth.json. Desktop
	// credentials now persist through the shared bowrain/core/config store, which
	// resolves its metadata file via BOWRAIN_CONFIG_DIR; pin it to a throwaway
	// dir so no test reads or clobbers a real login. Individual auth tests
	// override this with their own temp dir.
	authDir, err := os.MkdirTemp("", "bowrain-test-auth")
	if err != nil {
		panic(err)
	}
	os.Setenv("BOWRAIN_CONFIG_DIR", authDir)

	// Isolate tests from locally-installed plugins.
	dir, err := os.MkdirTemp("", "bowrain-test-plugins")
	if err != nil {
		panic(err)
	}
	os.Setenv("KAPI_PLUGIN_DIR", dir)

	templateDir, err := os.MkdirTemp("", "bowrain-test-store-template")
	if err != nil {
		panic(err)
	}
	templateStorePath = filepath.Join(templateDir, "store.db")
	template, err := sqlitestore.NewSQLiteStore(templateStorePath)
	if err != nil {
		panic(fmt.Errorf("build template content store: %w", err))
	}
	// Closing checkpoints the WAL back into the main file, so the template is
	// one self-contained file for newTestApp to copy.
	if err := template.Close(); err != nil {
		panic(fmt.Errorf("close template content store: %w", err))
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.RemoveAll(authDir)
	os.RemoveAll(templateDir)
	os.Exit(code)
}

// newTestApp creates a Bowrain backend with its own SQLite store and no shared
// content memory/TB state. Each call returns a fully isolated App suitable for a
// single test.
func newTestApp(t *testing.T) *App {
	t.Helper()
	// Use a temp directory for the store, the content memory and TB so tests
	// don't share state.
	tmpDir := t.TempDir()

	storePath := filepath.Join(tmpDir, "store.db")
	copyFile(t, templateStorePath, storePath)
	// The schema is already in the copy, so this opens and finds its migration
	// bookkeeping current rather than replaying the chain.
	cs, err := sqlitestore.NewSQLiteStore(storePath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	app := newAppWithStore(cs)
	app.memoryPath = filepath.Join(tmpDir, "test.db")
	tb, err := terms.NewSQLiteStore(filepath.Join(tmpDir, "test-tb.db"))
	if err != nil {
		t.Fatalf("create test terms: %v", err)
	}
	app.tb = tb
	t.Cleanup(func() {
		if err := app.ServiceShutdown(); err != nil {
			t.Errorf("shutdown app: %v", err)
		}
	})
	return app
}

// copyFile duplicates the template database into a test's own directory.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open template store: %v", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatalf("copy template store: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close test store: %v", err)
	}
}
