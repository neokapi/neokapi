package dataroot

import (
	"os"
	"testing"
)

// TestMain removes the data root this test binary was given once its tests
// have run, as every package that resolves one does.
func TestMain(m *testing.M) {
	code := m.Run()
	RemoveTestDir()
	os.Exit(code)
}
