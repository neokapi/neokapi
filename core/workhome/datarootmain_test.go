package workhome_test

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/dataroot"
)

// TestMain removes the data root this test binary was given once its tests
// have run: a file home that names no lock directory locks under it.
func TestMain(m *testing.M) {
	code := m.Run()
	dataroot.RemoveTestDir()
	os.Exit(code)
}
