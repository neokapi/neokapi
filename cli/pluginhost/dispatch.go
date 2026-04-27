package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// SourceConnectorDispatcher is implemented by per-plugin code that knows
// how to translate a command name + args into a gRPC RPC against the
// plugin's running daemon. Plugins register a dispatcher with
// RegisterSourceConnectorDispatcher; the kapi host calls it when a
// source-connector-bearing command (e.g., "push") needs to run.
//
// This indirection keeps the cli/pluginhost package free of a direct
// dependency on the bowrain proto (or any other plugin-specific proto).
//
// The dispatcher receives a live DaemonClient. Its Conn field is a
// ready-to-use *grpc.ClientConn. The dispatcher MUST NOT close the
// client — the pool owns its lifetime.
type SourceConnectorDispatcher interface {
	// Plugin returns the plugin name this dispatcher belongs to.
	Plugin() string

	// Dispatch runs the named source-connector op (e.g., "push", "pull",
	// "status", "ls") against the daemon. args are the user-provided
	// argv tail (positional + flags) the plugin's CLI knew about.
	//
	// The implementation typically creates a gRPC client stub from
	// client.Conn and issues the appropriate RPC.
	Dispatch(ctx context.Context, client *DaemonClient, op string, args []string) error
}

var (
	dispatchMu         sync.RWMutex
	dispatchByPlugin   = map[string]SourceConnectorDispatcher{}
	dispatchOpByPlugin = map[string]map[string]bool{} // plugin → set of op names
)

// RegisterSourceConnectorDispatcher records a dispatcher for one plugin.
// Calling twice for the same plugin replaces the previous registration.
//
// The ops slice declares which command names this dispatcher claims.
// The pluginhost calls dispatcher.Dispatch only when the command name
// matches one of these ops.
func RegisterSourceConnectorDispatcher(d SourceConnectorDispatcher, ops ...string) {
	dispatchMu.Lock()
	defer dispatchMu.Unlock()
	dispatchByPlugin[d.Plugin()] = d
	set := map[string]bool{}
	for _, o := range ops {
		set[o] = true
	}
	dispatchOpByPlugin[d.Plugin()] = set
}

// LookupSourceConnectorDispatcher returns the dispatcher for a plugin,
// or nil if none registered.
func LookupSourceConnectorDispatcher(plugin string) SourceConnectorDispatcher {
	dispatchMu.RLock()
	defer dispatchMu.RUnlock()
	return dispatchByPlugin[plugin]
}

// SupportsModeCDispatch reports whether the named op for the named
// plugin can be dispatched via Mode C. It returns true when (a) a
// dispatcher is registered for the plugin and (b) the op is in its
// declared set.
func SupportsModeCDispatch(plugin, op string) bool {
	dispatchMu.RLock()
	defer dispatchMu.RUnlock()
	if _, ok := dispatchByPlugin[plugin]; !ok {
		return false
	}
	ops := dispatchOpByPlugin[plugin]
	if ops == nil {
		return false
	}
	return ops[op]
}

// DispatchViaDaemon acquires a daemon for the plugin and routes the op
// through its registered dispatcher. Returns an error if the plugin has
// no daemon block, no dispatcher, or the dispatcher doesn't claim the op.
func DispatchViaDaemon(ctx context.Context, pool *DaemonPool, plugin *Plugin, op string, args []string) error {
	if pool == nil {
		return errors.New("daemon pool not initialized")
	}
	if plugin == nil {
		return errors.New("plugin is nil")
	}
	if !plugin.Manifest.IsModeC() || plugin.Manifest.Daemon == nil {
		return fmt.Errorf("plugin %q does not declare a Mode-C daemon block", plugin.Name())
	}
	d := LookupSourceConnectorDispatcher(plugin.Name())
	if d == nil {
		return fmt.Errorf("plugin %q has no source-connector dispatcher registered", plugin.Name())
	}
	if !SupportsModeCDispatch(plugin.Name(), op) {
		return fmt.Errorf("plugin %q dispatcher does not handle op %q", plugin.Name(), op)
	}
	client, err := pool.Acquire(ctx, plugin)
	if err != nil {
		return fmt.Errorf("acquire daemon for %q: %w", plugin.Name(), err)
	}
	return d.Dispatch(ctx, client, op, args)
}
