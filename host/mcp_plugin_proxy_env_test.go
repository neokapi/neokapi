package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A plugin's MCP server resolves a call that names no project to the project
// `kapi mcp` started in, and to none when kapi started in none.
func TestPluginMCPEnviron(t *testing.T) {
	env := []string{"PATH=/bin", "KAPI_NO_PROJECT=1", "KAPI_PROJECT=/elsewhere/kapi.yaml"}

	assert.Equal(t, env, pluginMCPEnviron(env, ""),
		"with no start project the plugin inherits KAPI_NO_PROJECT and finds none")

	assert.Equal(t, []string{"PATH=/bin", "KAPI_PROJECT=/work/app/kapi.yaml"},
		pluginMCPEnviron(env, "/work/app/kapi.yaml"),
		"the start project replaces any other and outranks KAPI_NO_PROJECT, which kapi has already applied")
}
