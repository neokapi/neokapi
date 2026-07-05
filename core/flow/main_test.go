package flow

import (
	"testing"

	"github.com/neokapi/neokapi/core/tool"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// The immutability hash backstop ships off by default; flow tests keep it
	// on so executors constructed in tests (which seed their default from the
	// global) catch tools mutating content in place, as they always have.
	tool.EnforceImmutability = true //nolint:staticcheck // deliberate: test-time enable of the deprecated compatibility knob
	goleak.VerifyTestMain(m)
}
