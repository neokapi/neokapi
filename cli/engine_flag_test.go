package cli

import (
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestEngineFlag_FollowsTheFormatFlag builds kapi's command tree and holds
// every command that takes --format/-f for its input to --engine as well:
// --format names a format outright, --engine says which engine serves a
// format detection chooses, and a command that offers one without the other
// leaves a file resolving differently from the next command over.
func TestEngineFlag_FollowsTheFormatFlag(t *testing.T) {
	// `kapi content add --format` writes a format into the recipe; it detects
	// nothing, so it takes no engine.
	recipeFormat := map[string]bool{"add": true}
	a := &App{}
	a.InitRegistries()
	// A tool whose own parameters take the --engine name keeps it under
	// `kapi exec <tool>`; the porcelain command of the same name takes the
	// format engine.
	ownEngine := func(c *cobra.Command) bool {
		if c.Parent() == nil || c.Parent().Name() != "exec" {
			return false
		}
		s := a.ToolReg.Schema(registry.ToolID(c.Name()))
		if s == nil {
			return false
		}
		_, own := s.Properties[host.EngineFlagName]
		return own
	}
	seen := 0
	for _, cmd := range KapiCommandSet(a) {
		walkCommands(cmd, func(c *cobra.Command) {
			f := c.Flags().Lookup("format")
			engine := c.Flags().Lookup(host.EngineFlagName)
			if f == nil || f.Shorthand != "f" || recipeFormat[c.Name()] {
				assert.Nil(t, engine, "`kapi %s` takes --engine without an input --format", c.CommandPath())
				return
			}
			seen++
			if !assert.NotNil(t, engine, "`kapi %s` takes --format/-f but not --engine", c.CommandPath()) {
				return
			}
			if ownEngine(c) {
				assert.NotEqual(t, host.EngineFlagUsage, engine.Usage, "`kapi %s --engine` is the tool's own parameter", c.CommandPath())
				return
			}
			assert.Equal(t, host.EngineFlagUsage, engine.Usage, "`kapi %s --engine` is not the format engine flag", c.CommandPath())
			assert.Empty(t, engine.DefValue, "`kapi %s --engine` registers a default, which pflag writes into the bound field and a recipe's defaults.engine can then never be reached", c.CommandPath())
		})
	}
	assert.Greater(t, seen, 5, "the walk reached the commands that read files")
}
