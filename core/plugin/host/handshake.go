// Package host implements the plugin host side of the gokapi plugin system.
// It discovers, launches, and communicates with plugin processes using
// HashiCorp's go-plugin framework over net/rpc.
package host

import (
	"net/rpc"

	"github.com/hashicorp/go-plugin"
)

// HandshakeConfig is the shared handshake configuration for all gokapi plugins.
// Both the host and the plugin process must agree on these values.
var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "GOKAPI_PLUGIN",
	MagicCookieValue: "gokapi-v1",
}

// FormatReaderPluginName is the plugin type identifier for format readers.
const FormatReaderPluginName = "format-reader"

// FormatWriterPluginName is the plugin type identifier for format writers.
const FormatWriterPluginName = "format-writer"

// ToolPluginName is the plugin type identifier for tools.
const ToolPluginName = "tool"

// FormatReaderPlugin is the go-plugin.Plugin implementation for format readers.
type FormatReaderPlugin struct{}

// Server returns nil because the host side never serves.
func (p *FormatReaderPlugin) Server(*plugin.MuxBroker) (any, error) {
	return nil, nil
}

// Client returns the RPC client for the format reader plugin.
func (p *FormatReaderPlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (any, error) {
	return &FormatReaderRPCClient{client: c}, nil
}

// FormatWriterPlugin is the go-plugin.Plugin implementation for format writers.
type FormatWriterPlugin struct{}

// Server returns nil because the host side never serves.
func (p *FormatWriterPlugin) Server(*plugin.MuxBroker) (any, error) {
	return nil, nil
}

// Client returns the RPC client for the format writer plugin.
func (p *FormatWriterPlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (any, error) {
	return &FormatWriterRPCClient{client: c}, nil
}

// ToolPlugin is the go-plugin.Plugin implementation for tools.
type ToolPlugin struct{}

// Server returns nil because the host side never serves.
func (p *ToolPlugin) Server(*plugin.MuxBroker) (any, error) {
	return nil, nil
}

// Client returns the RPC client for the tool plugin.
func (p *ToolPlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (any, error) {
	return &ToolRPCClient{client: c}, nil
}

// PluginMap returns the plugin map used by the host to discover plugin types.
func PluginMap() map[string]plugin.Plugin {
	return map[string]plugin.Plugin{
		FormatReaderPluginName: &FormatReaderPlugin{},
		FormatWriterPluginName: &FormatWriterPlugin{},
		ToolPluginName:         &ToolPlugin{},
	}
}
