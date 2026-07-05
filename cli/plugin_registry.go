package cli

import (
	"sync"

	"github.com/spf13/cobra"
)

// CommandFactory adds top-level cobra subcommands to the root command.
// Plugins register factories via init() (typically blank-imported by the
// binary's main package); the binary's entry point calls
// ApplyCommandFactories after wiring built-in commands.
type CommandFactory func(parent *cobra.Command, app *App)

// AppInitializer mutates an *App after construction. Plugins use this to
// install fields like FallbackRunE / ExtraFlows hooks that don't fit a
// dedicated registry. Called once per App after InitRegistries.
type AppInitializer func(app *App)

var (
	regMu            sync.RWMutex
	commandFactories []CommandFactory
	appInitializers  []AppInitializer
)

// RegisterCommandFactory queues a factory. Safe from init().
func RegisterCommandFactory(f CommandFactory) {
	regMu.Lock()
	defer regMu.Unlock()
	commandFactories = append(commandFactories, f)
}

// RegisterAppInitializer queues an initializer. Safe from init().
func RegisterAppInitializer(f AppInitializer) {
	regMu.Lock()
	defer regMu.Unlock()
	appInitializers = append(appInitializers, f)
}

// ApplyCommandFactories invokes every registered CommandFactory in
// registration order. The binary's main package calls this after
// constructing built-in commands.
func ApplyCommandFactories(parent *cobra.Command, app *App) {
	regMu.RLock()
	fs := append([]CommandFactory(nil), commandFactories...)
	regMu.RUnlock()
	for _, f := range fs {
		f(parent, app)
	}
}

// ApplyAppInitializers invokes every registered AppInitializer in
// registration order.
func ApplyAppInitializers(app *App) {
	regMu.RLock()
	fs := append([]AppInitializer(nil), appInitializers...)
	regMu.RUnlock()
	for _, f := range fs {
		f(app)
	}
}

// ResetPluginRegistriesForTest clears the registries. Test-only.
func ResetPluginRegistriesForTest() {
	regMu.Lock()
	defer regMu.Unlock()
	commandFactories = nil
	appInitializers = nil
	ResetMCPToolFactoriesForTest()
}
