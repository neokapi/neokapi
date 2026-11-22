package mcptools

import (
	"testing"

	"github.com/neokapi/neokapi/core/devenvtest"
	"github.com/neokapi/neokapi/host"
)

// These tests open workspaces through host, which gives this test binary a
// data root of its own under the temporary directory. It is removed once they
// have run, and the tests run without the kapi variables a developer's shell
// may export (see devenvtest).
func TestMain(m *testing.M) { devenvtest.Main(m, host.RemoveTestDataDir) }
