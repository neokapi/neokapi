package host

import (
	"testing"

	"github.com/neokapi/neokapi/core/devenvtest"
)

// These tests build projects in temporary directories, rely on discovery to
// find them, and expect a workspace of their own, so they run without the kapi
// variables a developer's shell may export (see devenvtest). The data root
// this binary got, and the plugin binary it built, are removed once they have
// run.
func TestMain(m *testing.M) { devenvtest.Main(m, RemoveTestDataDir, removeSourcecodePlugin) }
