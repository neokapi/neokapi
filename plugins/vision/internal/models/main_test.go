package models

import (
	"os"
	"testing"
)

// The test model cache root (testModelsDir) is removed once the tests have
// run, so a run leaves nothing under the temporary directory.
func TestMain(m *testing.M) {
	code := m.Run()
	removeTestModelsDir()
	os.Exit(code)
}
