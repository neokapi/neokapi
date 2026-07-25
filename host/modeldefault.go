package host

import (
	"fmt"
	"io"

	"github.com/neokapi/neokapi/host/config"
)

// ModelDefaultOutput is the structured result of `kapi models default`, showing
// or setting the shared default AI provider/model.
//
// It reports the *resolved* default (config.ResolveAIDefault), including where
// the value came from, so what the command prints is exactly what a run will
// use. Printing a value read separately from what the resolver returns is how
// the two came to disagree.
type ModelDefaultOutput struct {
	Default config.AIDefault `json:"default"`
	// Set is true when this invocation changed the default.
	Set bool `json:"set,omitempty"`
}

// FormatText renders the default for a terminal.
func (o ModelDefaultOutput) FormatText(w io.Writer) error {
	if o.Set {
		fmt.Fprintf(w, "Default AI model set: %s\n", o.Default.Describe())
		return nil
	}
	if !o.Default.Any() {
		fmt.Fprintln(w, "No default AI model set (AI tools fall back to the built-in anthropic default).")
		fmt.Fprintln(w, "Set one with: kapi models default <model>")
		return nil
	}
	fmt.Fprintf(w, "provider: %s\n", OrNone(o.Default.Provider))
	fmt.Fprintf(w, "model:    %s\n", OrNone(o.Default.Model))
	switch o.Default.Source {
	case config.AIDefaultEnv:
		fmt.Fprintf(w, "source:   %s (environment)\n", config.EnvAIProvider)
	case config.AIDefaultConfig:
		fmt.Fprintf(w, "source:   %s\n", o.Default.File)
	}
	return nil
}
