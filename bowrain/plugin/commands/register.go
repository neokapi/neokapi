// Package commands holds the bowrain CLI command implementations.
//
// Each subcommand file registers itself via init() using
// cli.RegisterCommandFactory so the host binary (kapi or bowrain CLI)
// picks them up via cli.ApplyCommandFactories.
package commands

import (
	"github.com/neokapi/neokapi/cli"
)

func init() {
	// Capture the App for command bodies that still rely on the package
	// global. This runs after Init() has populated FormatReg, ToolReg,
	// PluginHost, etc.
	cli.RegisterAppInitializer(func(a *cli.App) {
		app = a
		a.FallbackRunE = projectFlowFallback
		a.ExtraFlows = listProjectFlows
	})
}
