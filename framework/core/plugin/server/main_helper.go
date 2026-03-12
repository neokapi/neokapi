package server

import (
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/tool"
)

// handshakeConfig must match the host side.
var handshakeConfig = goplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "NEOKAPI_PLUGIN",
	MagicCookieValue: "neokapi-v1",
}

// ServeFormatReader starts serving a DataFormatReader as a plugin.
// Call this from the plugin's main() function.
//
//	func main() {
//	    server.ServeFormatReader(myReader)
//	}
func ServeFormatReader(impl format.DataFormatReader) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			"format-reader": &FormatReaderServerPlugin{Impl: impl},
		},
	})
}

// ServeFormatWriter starts serving a DataFormatWriter as a plugin.
// Call this from the plugin's main() function.
//
//	func main() {
//	    server.ServeFormatWriter(myWriter)
//	}
func ServeFormatWriter(impl format.DataFormatWriter) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			"format-writer": &FormatWriterServerPlugin{Impl: impl},
		},
	})
}

// ServeTool starts serving a Tool as a plugin.
// Call this from the plugin's main() function.
//
//	func main() {
//	    server.ServeTool(myTool)
//	}
func ServeTool(impl tool.Tool) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			"tool": &ToolServerPlugin{Impl: impl},
		},
	})
}
