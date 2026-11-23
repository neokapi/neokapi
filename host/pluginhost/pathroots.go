package pluginhost

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rootpath"
)

// EnvPluginRoots names the directories a plugin daemon may be sent paths in,
// joined with the platform's path list separator. The host sets it on every
// daemon it starts.
//
// The roots are what a person chose, attached where the run begins
// (WithPathRoots): the project's root, explicit inputs and their -o
// outputs, or the MCP server's start directory, project and client roots.
// No root comes from the path a call sends.
//
// The host sends a path only when it lies beneath a root once its links are
// resolved, and the content inline otherwise; with no roots, never a path.
// What the plugin does with a path it is given, such as opening a companion
// file, stays its own check: okapi-bridge refuses one outside the roots.
const EnvPluginRoots = "KAPI_PLUGIN_ROOTS"

type pathRootsKey struct{}

// WithPathRoots returns ctx declaring roots, and only them, as the
// directories the daemon calls under ctx may send paths in. Each root is
// resolved as the plugin resolves it (links followed, a missing tail kept),
// and the list is sorted with no repeats, so two runs over one project share
// a daemon. The pool starts, or reuses, a daemon whose roots are exactly
// these.
func WithPathRoots(ctx context.Context, roots []string) context.Context {
	return context.WithValue(ctx, pathRootsKey{}, normalizeRoots(roots))
}

// AddPathRoots returns ctx declaring the roots ctx already declares and dirs.
func AddPathRoots(ctx context.Context, dirs ...string) context.Context {
	return WithPathRoots(ctx, append(slices.Clone(PathRootsFrom(ctx)), dirs...))
}

// PathRootsFrom returns the roots ctx declares, sorted; none when it declares
// none.
func PathRootsFrom(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	roots, _ := ctx.Value(pathRootsKey{}).([]string)
	return roots
}

// PathRootsFor returns the roots to declare for files and directories a
// person named: for each path, the root of the kapi project that holds it,
// or, for a path in no project, the directory itself or the directory that
// holds the file.
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
		//pathguard: a path a person named for the run, stated to tell a directory from a file; nothing is opened
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			dir = abs
		}
		if layout, err := project.ResolveLayout(dir); err == nil && rootpath.IsBeneath(layout.Root, abs) {
			dir = layout.Root
		}
		roots = append(roots, dir)
	}
	return normalizeRoots(roots)
}

// beneathPathRoots returns p with its links resolved when it lies beneath one
// of roots, and false otherwise, roots empty included. A p that does not
// exist yet (an output) is judged by its nearest existing ancestor.
func beneathPathRoots(roots []string, p string) (string, bool) {
	if p == "" {
		return "", false
	}
	for _, root := range roots {
		real, err := rootpath.Contain(root, p)
		if err == nil && rootpath.IsBeneath(root, real) {
			return real, true
		}
	}
	return "", false
}

// normalizeRoots resolves each root as the plugin resolves it, drops empty
// ones, and sorts the rest with no repeats.
func normalizeRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if r == "" {
			continue
		}
		abs, err := filepath.Abs(r)
		if err != nil {
			continue
		}
		out = append(out, resolveExistingPrefix(abs))
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// resolveExistingPrefix resolves the links on the longest existing prefix of
// abs and appends the names that do not exist yet, so an output directory a
// run has still to create is spelled as it will resolve.
func resolveExistingPrefix(abs string) string {
	var missing []string
	for cur := abs; ; cur = filepath.Dir(cur) {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			slices.Reverse(missing)
			return filepath.Join(append([]string{real}, missing...)...)
		}
		if !errors.Is(err, fs.ErrNotExist) || filepath.Dir(cur) == cur {
			return abs
		}
		missing = append(missing, filepath.Base(cur))
	}
}

// daemonKey is the pool's key for a plugin's daemon started with roots.
func daemonKey(plugin string, roots []string) string {
	return plugin + "\x00" + strings.Join(roots, string(os.PathListSeparator))
}
