package plugin_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	_ "github.com/neokapi/neokapi/bowrain/plugin"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// A push stages the context ledger of a project whose context is on Bowrain,
// so the plugin binary has to carry the bowrain backend itself.
func TestPluginRegistersBowrainContextBackend(t *testing.T) {
	assert.True(t, host.HasContextRemote(project.ContextBackendBowrain))
}
