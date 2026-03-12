// plugin-format-csv is an example gokapi plugin that reads CSV files.
// It demonstrates how to use the plugin server helpers to expose a
// DataFormatReader as a standalone plugin process.
//
// Build:
//
//	go build -o gokapi-plugin-csv ./examples/plugin-format-csv
//
// Usage: Place the binary in the plugin directory and run the host application.
package main

import (
	"github.com/gokapi/gokapi/core/plugin/server"
)

func main() {
	server.ServeFormatReader(NewCSVReader())
}
