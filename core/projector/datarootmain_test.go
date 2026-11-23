package projector_test

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/dataroot"
)

// TestMain removes the data root this test binary was given once its tests
// have run: the tests here resolve a checkout's state directory under it.
func TestMain(m *testing.M) {
	code := m.Run()
	dataroot.RemoveTestDir()
	os.Exit(code)
}
