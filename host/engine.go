package host

import (
	"fmt"

	"github.com/spf13/pflag"
)

// EngineFlagName is the name of the --engine flag, and EngineFlagUsage its
// help text.
const (
	EngineFlagName  = "engine"
	EngineFlagUsage = "format engine for detected files: native or a plugin name (default: native, then plugins)"
)

// AddEngineFlag registers --engine on f, bound to the App's EngineFlag. Every
// command that takes --format takes it: --format names the format outright,
// --engine says which engine serves a format detection chooses.
func (a *App) AddEngineFlag(f *pflag.FlagSet) {
	f.StringVar(&a.EngineFlag, EngineFlagName, "", EngineFlagUsage)
}

// ApplyEngineSelection hands the registry the engine the config prefers and
// the one --engine forces, checking that the latter names an installed
// engine. It runs once the plugin host has registered its formats, so the
// check sees every engine.
func (a *App) ApplyEngineSelection() error {
	a.FormatReg.SetDefaultEngine(a.Config.FormatEngine())
	if err := a.FormatReg.CheckEngine(a.EngineFlag); err != nil {
		return fmt.Errorf("--%s: %w", EngineFlagName, err)
	}
	a.FormatReg.SetEngineOverride(a.EngineFlag)
	return nil
}
