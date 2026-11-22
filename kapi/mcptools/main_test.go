package mcptools

import (
	"os"
	"testing"
)

// TestMain keeps every test off the project the repository's own recipe
// declares: a call that names no project would otherwise find it by the
// upward walk from the package directory and run inside it.
func TestMain(m *testing.M) {
	if err := os.Setenv("KAPI_NO_PROJECT", "1"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
