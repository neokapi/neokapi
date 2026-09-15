package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/pluginhost"
)

// A plugin route disables cobra's flag parsing, so kapi's persistent flags
// reach the source-connector dispatcher among the route's own arguments,
// wherever they were typed. The dispatcher takes them under the same names,
// shorthands and value types: a missing one refuses `kapi push -v`, and a
// boolean taken as a string reads the path after `--json` as its value.
func TestSourceConnectorRoutesTakeKapiPersistentFlags(t *testing.T) {
	root := &cobra.Command{Use: "kapi"}
	AddPersistentFlags(&App{}, root)
	taken := pluginhost.KapiPersistentFlags()

	count := 0
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		count++
		got := taken.Lookup(f.Name)
		if !assert.NotNil(t, got, "the source-connector routes take --%s", f.Name) {
			return
		}
		assert.Equal(t, f.Shorthand, got.Shorthand, "--%s shorthand", f.Name)
		assert.Equal(t, f.Value.Type(), got.Value.Type(), "--%s value type", f.Name)
		assert.Equal(t, f.NoOptDefVal, got.NoOptDefVal, "--%s bare value", f.Name)
	})
	require.NotZero(t, count)
	taken.VisitAll(func(f *pflag.Flag) {
		assert.NotNil(t, root.PersistentFlags().Lookup(f.Name), "--%s is one of kapi's persistent flags", f.Name)
	})
}
