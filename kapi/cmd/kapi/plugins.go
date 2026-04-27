//go:build !pure

package main

// Default kapi build pulls in the bowrain plugin so users get the full
// feature set out of the box (push, pull, status, project flows, MCP
// tools, recipe schema validation).
//
// Build with `-tags pure` to omit bowrain — useful when distributing a
// stripped framework-only binary or when running framework-isolation
// checks.
import _ "github.com/neokapi/neokapi/bowrain/plugin"
