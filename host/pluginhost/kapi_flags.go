package pluginhost

import (
	"github.com/spf13/pflag"

	"github.com/neokapi/neokapi/host/output"
)

// KapiPersistentFlags returns a new flag set holding the flags kapi's root
// command defines for every command (cli.AddPersistentFlags). A plugin route
// disables cobra's flag parsing, so these reach a dispatcher among the route's
// own arguments, wherever they were typed. The source-connector dispatcher
// parses them, so a flag's value is never read as a path, and passes none of
// them to the daemon. A test in the cli module pins this set to the root's.
func KapiPersistentFlags() *pflag.FlagSet {
	fs := pflag.NewFlagSet("kapi", pflag.ContinueOnError)
	fs.StringP("config", "c", "", "config file path")
	fs.BoolP("verbose", "v", false, "verbose output")
	fs.BoolP("quiet", "q", false, "suppress output")
	fs.BoolP("yes", "y", false, "assume yes for confirmation prompts")
	fs.String("plugin-dir", "", "plugin directory")
	fs.String("lang", "", "UI locale")
	fs.String("explain-prompts", "", "show the prompts sent to the LLM")
	fs.Lookup("explain-prompts").NoOptDefVal = "-"
	output.AddPersistentFlags(fs)
	return fs
}
