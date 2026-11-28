package project

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/dataroot"
)

// TestMain removes the data root this test binary was given once its tests
// have run: a recipe save takes its file's lock under it.
func TestMain(m *testing.M) {
	code := m.Run()
	dataroot.RemoveTestDir()
	os.Exit(code)
}
