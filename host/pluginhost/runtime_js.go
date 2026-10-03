//go:build js

package pluginhost

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/comment"
)

// The browser build runs no plugin. A plugin is a separate executable that
// kapi starts as a subprocess and reaches over gRPC, and a page can do
// neither. The files that start a plugin or talk to one (the daemon pool, the
// format, segmenter, comment and source-connector clients, the subprocess
// launches) are built for every other target, which keeps gRPC and the plugin
// protobuf out of the WebAssembly engine. Discovery and the manifest-driven
// Host are built for the browser too: they only read manifests, and a page has
// none to read. The names below stand in for the ones the rest of the build
// calls, and each answers that the browser starts no plugin.

// errNoPluginInBrowser is what every plugin launch answers in the browser.
var errNoPluginInBrowser = errors.New("plugins are separate executables, which the browser cannot run")

// DaemonPool is the Mode-C daemon pool's name in the browser build, which
// starts no daemon and so holds none.
type DaemonPool struct{}

// DaemonPool returns nil in the browser, which starts no plugin daemon.
func (r *Runtime) DaemonPool() *DaemonPool { return nil }

// Shutdown has no daemon to stop in the browser.
func (r *Runtime) Shutdown() {}

// wire registers the recipe schema extensions the plugins' manifests declare.
// Formats, segmenters and source connectors run in a plugin's daemon, which
// the browser cannot start, so none of them is registered.
func (r *Runtime) wire(host *Host) {
	RegisterSchemaExtensions(host, r.onWarn)
}

// SupportsModeCDispatch reports false in the browser, where no dispatcher is
// ever registered.
func SupportsModeCDispatch(string, string) bool { return false }

// DispatchViaDaemon refuses in the browser, which starts no plugin daemon.
func DispatchViaDaemon(_ context.Context, _ *DaemonPool, plugin *Plugin, op string, _ []string) error {
	if plugin == nil {
		return errors.New("plugin is nil")
	}
	return fmt.Errorf("plugin %q %s: %w", plugin.Name(), op, errNoPluginInBrowser)
}

// ExecPluginCommand refuses in the browser, which starts no subprocess.
func ExecPluginCommand(_ context.Context, route *CommandRoute, _ []string) error {
	return fmt.Errorf("plugin %q: %w", route.Plugin.Name(), errNoPluginInBrowser)
}

// ExecPluginSubcommandPath refuses in the browser, which starts no subprocess.
func ExecPluginSubcommandPath(_ context.Context, route *CommandRoute, _, _ []string) error {
	return fmt.Errorf("plugin %q: %w", route.Plugin.Name(), errNoPluginInBrowser)
}

// RunContributionSubprocess refuses in the browser, which starts no
// subprocess.
func RunContributionSubprocess(_ context.Context, p *Plugin, _ []string, _ string) error {
	return fmt.Errorf("plugin %q: %w", p.Name(), errNoPluginInBrowser)
}

// CaptureStdout refuses in the browser, which starts no subprocess.
func (r *CommandRoute) CaptureStdout(context.Context, ...string) ([]byte, error) {
	return nil, fmt.Errorf("plugin %q: %w", r.Plugin.Name(), errNoPluginInBrowser)
}

// StreamStdout refuses in the browser, which starts no subprocess.
func (r *CommandRoute) StreamStdout(context.Context, func([]byte), ...string) error {
	return fmt.Errorf("plugin %q: %w", r.Plugin.Name(), errNoPluginInBrowser)
}

// Locate refuses in the browser: the plugin locates a language's comments in
// its daemon, which the browser cannot start.
func (p *daemonCommentProvider) Locate(string, []byte) (*comment.File, error) {
	return nil, fmt.Errorf("locate %s comments (plugin %q): %w", p.lang.Language, p.plugin.Name(), errNoPluginInBrowser)
}
