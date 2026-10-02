package bowrainmcp

import (
	"testing"

	"github.com/neokapi/neokapi/core/devenvtest"
)

// These tests build projects in temporary directories and rely on discovery
// to find them, so they run without the kapi variables a developer's shell may
// export (see devenvtest).
func TestMain(m *testing.M) { devenvtest.Main(m) }
