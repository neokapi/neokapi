package host

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/change"
)

// A refused change set exits with the code the kapi binary gives the same
// meaning: a malformed invocation, an attempt that did not land, or a backend
// that did not answer.
func TestChangeExitCodesKeepTheBinarysMeanings(t *testing.T) {
	assert.Equal(t, ExitUsage, change.ExitInvalid)
	assert.Equal(t, ExitGate, change.ExitRefused)
	assert.Equal(t, ExitUnreachable, change.ExitUnreachable)
}
