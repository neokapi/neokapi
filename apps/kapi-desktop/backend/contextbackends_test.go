package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// The desktop opens every context backend a recipe can declare, as the kapi
// binary does.
func TestTheDesktopOpensEveryContextBackend(t *testing.T) {
	for _, kind := range project.ContextBackendKinds {
		assert.True(t, host.HasContextRemote(kind), "no %s backend in the desktop build", kind)
	}
}
