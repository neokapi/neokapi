package host

import (
	"testing"

	"github.com/neokapi/neokapi/core/devenvtest"
)

// These tests build projects in temporary directories, rely on discovery to
// find them, and expect a workspace of their own, so they run without the kapi
// variables a developer's shell may export (see devenvtest).
func TestMain(m *testing.M) { devenvtest.Main(m) }
