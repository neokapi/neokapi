package commands

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/cli"
)

// kapi parses argv against its own `up` before it execs this plumbing, and it
// forwards only the flags it parsed. A flag declared here and nowhere else is
// rejected as an unknown flag before this command ever runs, so the whole
// surface below has to exist on the built-in command too.
//
// The direction matters: this package may import cli, and cli may not import
// anything under bowrain/, so this is the only side the check can live on.
func TestUpFlagSurface_EveryFlagHereExistsOnTheBuiltinUp(t *testing.T) {
	builtin := cli.NewUpCmd(&cli.App{})

	// LocalFlags: the flags a parent command lends (--yes, --config) are the
	// host root's and reach both commands alike, as does the --help cobra adds
	// to any command it has run.
	upCmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" {
			return
		}
		assert.NotNil(t, builtin.Flags().Lookup(f.Name),
			"`kapi up --%s` is rejected as an unknown flag before this plumbing runs; register it in cli/up.go as well", f.Name)
	})
}
