// Command kapi-hello is a minimal reference plugin demonstrating the
// kapi plugin protocol v1. It declares one Mode-A command and one
// Mode-B MCP tool. See docs/internals/plugin-protocol-v1.md for the
// full spec.
//
// Run from the repo root:
//
//	go build -o examples/plugins/hello/kapi-hello ./examples/plugins/hello
//	KAPI_PLUGINS_DIR=$PWD/examples/plugins kapi --help
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const pluginVersion = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "kapi-hello: usage: kapi-hello {command|mcp-server|version}")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "command":
		runCommand(os.Args[2:])
	case "mcp-server":
		runMCPServer()
	case "version":
		fmt.Println(pluginVersion)
	default:
		fmt.Fprintf(os.Stderr, "kapi-hello: unknown subcommand %q\n", os.Args[1])
		os.Exit(2)
	}
}

func runCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "kapi-hello: command requires a name")
		os.Exit(2)
	}
	switch args[0] {
	case "hello":
		who := "world"
		if len(args) >= 2 {
			who = args[1]
		}
		fmt.Printf("Hello, %s!\n", who)
	default:
		fmt.Fprintf(os.Stderr, "kapi-hello: unknown command %q\n", args[0])
		os.Exit(2)
	}
}

// runMCPServer is a minimal stub that completes one round-trip of the
// MCP initialize handshake and exits when stdin closes. Production
// plugins should use a full MCP server SDK; this exists so kapi's
// plugin-discovery tests can exercise Mode B without bringing in the
// MCP go-sdk in the example.
func runMCPServer() {
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for {
		var msg map[string]any
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintln(os.Stderr, "kapi-hello: mcp decode error:", err)
			return
		}
		method, _ := msg["method"].(string)
		id := msg["id"]
		switch method {
		case "initialize":
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"serverInfo":      map[string]any{"name": "kapi-hello", "version": pluginVersion},
					"capabilities":    map[string]any{"tools": map[string]any{}},
				},
			})
		case "tools/list":
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "say_hello", "description": "Return a friendly greeting"},
					},
				},
			})
		case "tools/call":
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "Hello from kapi-hello!"}},
				},
			})
		default:
			if id != nil {
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"error":   map[string]any{"code": -32601, "message": "method not found"},
				})
			}
		}
	}
}
