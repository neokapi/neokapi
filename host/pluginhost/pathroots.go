package pluginhost

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rootpath"
)

// EnvPluginRoots names the directories a plugin daemon may be sent paths in,
// joined with the platform's path list separator. The host sets it on every
// daemon it starts. A plugin that confines what it opens (okapi-bridge)
// refuses a path outside them, and with none declared refuses every path;
// content sent inline is unaffected.
const EnvPluginRoots = "KAPI_PLUGIN_ROOTS"

type pathRootsKey struct{}

// WithPathRoots returns ctx carrying the directories the paths of the next
// daemon call lie under (PathRootsFor). The pool starts, or reuses, a daemon
// whose roots are exactly these.
func WithPathRoots(ctx context.Context, roots []string) context.Context {
	return context.WithValue(ctx, pathRootsKey{}, roots)
}

// PathRootsFrom returns the roots ctx carries, sorted; none when it carries
// none.
func PathRootsFrom(ctx context.Context) []string {
	roots, _ := ctx.Value(pathRootsKey{}).([]string)
	return roots
}

// PathRootsFor returns the directories a daemon must be allowed to open
// paths in: for each path, the root of the kapi project that holds it, or,
// for a path in no project (a file a person named), the directory that holds
// it. Each is the real path, as the plugin resolves it, and the list is
// sorted with no repeats, so two calls over one project share a daemon.
func PathRootsFor(paths ...string) []string {
	var roots []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		dir := filepath.Dir(abs)
		if layout, err := project.ResolveLayout(dir); err == nil && rootpath.IsBeneath(layout.Root, abs) {
			dir = layout.Root
		}
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		roots = append(roots, dir)
	}
	slices.Sort(roots)
	return slices.Compact(roots)
}

// daemonKey is the pool's key for a plugin's daemon started with roots. The
// roots are quoted, so a root holding the path list separator stays one root.
func daemonKey(plugin string, roots []string) poolKey {
	return poolKey{plugin: plugin, roots: fmt.Sprintf("%q", roots)}
}

// poolKey is what the daemon pool files a daemon under: the plugin and the
// roots it was started with.
type poolKey struct{ plugin, roots string }
