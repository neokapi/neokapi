// Package plugin is the bowrain feature anchor.
//
// A host binary (kapi or bowrain CLI) blank-imports this single path:
//
//	import _ "github.com/neokapi/neokapi/bowrain/plugin"
//
// The blank import causes Go to load every sub-package this file depends
// on, which in turn causes their init() functions to run:
//
//   - bowrain/plugin/schema     → registers extension decoders
//   - bowrain/plugin/commands   → registers CLI command factories
//   - bowrain/plugin/mcp        → registers MCP tool factories
//
// After the host calls cli.ApplyAppInitializers / cli.ApplyCommandFactories
// the bowrain feature set is wired in.
//
// Builds that want to exclude bowrain (e.g. a stripped kapi for
// non-bowrain users) just skip this import — the framework happily runs
// without it.
package plugin

import (
	_ "github.com/neokapi/neokapi/bowrain/plugin/commands"
	_ "github.com/neokapi/neokapi/bowrain/plugin/mcp"
	_ "github.com/neokapi/neokapi/bowrain/plugin/schema"
)
