// plugin-format-csv is an example neokapi plugin that reads CSV files.
// It demonstrates how to use the plugin server helpers to expose a
// DataFormatReader as a standalone plugin process.
//
// Build:
//
//	go build -o neokapi-plugin-csv ./examples/plugin-format-csv
//
// Usage: Place the binary in the plugin directory and run the host application.
package main

import (
	"github.com/neokapi/neokapi/core/plugin/server"
)

func main() {
	server.ServeFormatReader(NewCSVReader())
}
