package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/tool"
)

func TestMain(m *testing.M) {
	// The immutability hash backstop ships off by default; the built-in tool
	// tests keep it on so a tool that mutates block content in place (instead
	// of going through its capability-typed view) fails loudly here, before
	// it ships.
	tool.EnforceImmutability = true //nolint:staticcheck // deliberate: test-time enable of the deprecated compatibility knob
	m.Run()
}
