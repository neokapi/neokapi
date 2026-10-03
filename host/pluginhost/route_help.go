package pluginhost

import "github.com/spf13/pflag"

// RouteHelp is the error Prepare returns when a route's arguments ask for
// help. It matches pflag.ErrHelp. Usage is the argument synopsis that follows
// the command name, and Flags holds the flags the route takes, for the help a
// front end prints.
type RouteHelp struct {
	Op    string
	Usage string
	Flags *pflag.FlagSet
}

func (h *RouteHelp) Error() string { return "help requested for " + h.Op }

// Is reports whether target is pflag.ErrHelp.
func (h *RouteHelp) Is(target error) bool { return target == pflag.ErrHelp }
