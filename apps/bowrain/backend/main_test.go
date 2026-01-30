package backend

import (
	"os"
	"testing"
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
