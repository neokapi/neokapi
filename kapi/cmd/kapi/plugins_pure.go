//go:build pure

package main

// Pure framework-only kapi: no bowrain plugin. The CLI exposes only the
// framework's built-in commands (run, formats, plugins, tools, presets,
// termbase, version, MCP, etc.). Used for isolation checks and any
// downstream binary that only wants the open-source framework surface.
