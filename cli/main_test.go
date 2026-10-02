package cli

import (
	"testing"

	"github.com/neokapi/neokapi/core/devenvtest"
	"github.com/neokapi/neokapi/host"
)

// These tests build projects and plugin roots in temporary directories and
// rely on discovery to find them, so they run without the kapi variables a
// developer's shell may export (see devenvtest). The data root this binary got
// is removed once they have run.
func TestMain(m *testing.M) { devenvtest.Main(m, host.RemoveTestDataDir) }
