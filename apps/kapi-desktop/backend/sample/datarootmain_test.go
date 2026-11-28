package sample

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/dataroot"
)

// TestMain removes the data root this test binary was given once its tests
// have run: writing a sample's recipe takes the recipe's lock under it.
func TestMain(m *testing.M) {
	code := m.Run()
	dataroot.RemoveTestDir()
	os.Exit(code)
}
