//go:build !js

package cli

import host "github.com/neokapi/neokapi/host"

// The MCP tool registry, re-exported for the binaries that serve MCP. It is
// built for every target but js: the browser build serves no MCP, and leaving
// the registry out keeps the MCP SDK and every tool it would register out of
// the WebAssembly engine.
var (
	ApplyMCPToolFactories  = host.ApplyMCPToolFactories
	RegisterMCPToolFactory = host.RegisterMCPToolFactory
)
