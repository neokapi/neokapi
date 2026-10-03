//go:build js

package host

// The browser build serves no MCP: the files that import the MCP SDK, the
// tools they register and the plugin servers they spawn are built for every
// other target, so the SDK stays out of the WebAssembly engine. The App keeps
// its bookkeeping for plugin MCP servers on every target, and in the browser
// it spawns none.

// pluginMCPSession is a spawned plugin MCP server, which the browser build
// never spawns.
type pluginMCPSession struct{}

// closePluginMCPSessions has no plugin MCP server to shut down in the
// browser.
func (a *App) closePluginMCPSessions() {}

// ResetMCPToolFactoriesForTest has no MCP tool registry to clear in the
// browser.
func ResetMCPToolFactoriesForTest() {}
