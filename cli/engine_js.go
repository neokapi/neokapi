//go:build js

package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// NewEngineCmd is the WASM stub: the engine gRPC service needs a Unix
// socket, which the browser/wasm runtime has no access to. The command
// exists so shared command wiring compiles, and reports the limitation
// when invoked.
func (a *App) NewEngineCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "engine",
		Short:  "Serve the content engine as a local gRPC API",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("kapi engine serve is not available in the browser build")
		},
	}
}
