package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/devenvtest"
)

// These tests expect a workspace of their own, so they run without the kapi
// variables a developer's shell may export (see devenvtest).
func TestMain(m *testing.M) { devenvtest.Main(m) }
