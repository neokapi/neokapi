package pluginhost

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// DiscoverOptions configures plugin discovery.
type DiscoverOptions struct {
	// EnvPluginsDir is the value of $KAPI_PLUGINS_DIR (highest precedence).
	// Multiple paths separated by os-specific list separator (`:` on
	// Unix, `;` on Windows). Empty disables this discovery root.
	EnvPluginsDir string

	// XDGDataHome is $XDG_DATA_HOME. Empty falls back to ~/.local/share.
	XDGDataHome string

	// HomeDir is the user's home directory. Empty falls back to os.UserHomeDir.
	HomeDir string

	// SystemDirs are absolute paths to scan as system-installed plugin
	// roots (Homebrew, /usr/local, /usr/share). Empty defaults to the
	// platform-appropriate set.
	SystemDirs []string

	// OnlyEnvDir restricts discovery to EnvPluginsDir alone, skipping the
	// user (XDG) and system roots. Use it to dogfood an in-repo kapi without
	// picking up the developer's or the machine's globally-installed plugins.
	// Also enabled by a non-empty $KAPI_PLUGINS_DIR_ONLY (matching the
	// KAPI_NO_PROJECT isolation convention), so any front-end honours it.
	OnlyEnvDir bool

	// OnWarn is called for non-fatal discovery warnings (skipped manifests,
	// invalid JSON, etc.). Optional.
	OnWarn func(msg string)
}

// DefaultSystemDirs returns the platform-default list of system plugin
// roots, in precedence order (all share the same precedence tier).
func DefaultSystemDirs() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/opt/homebrew/share/kapi/plugins",
			"/usr/local/share/kapi/plugins",
		}
	case "linux":
		return []string{
			"/usr/local/share/kapi/plugins",
			"/usr/share/kapi/plugins",
		}
	default:
		return nil
	}
}

// Discover scans configured plugin roots and returns every successfully
// parsed plugin. Manifests that fail to parse or fail Validate are
// skipped with a warning; discovery never returns a fatal error
// (missing dirs are silently ignored).
func Discover(opts DiscoverOptions) []*Plugin {
	warn := opts.OnWarn
	if warn == nil {
		warn = func(string) {}
	}
	roots := assembleRoots(opts)

	var out []*Plugin
	for _, r := range roots {
		entries, err := os.ReadDir(r.Path)
		if err != nil {
			if !os.IsNotExist(err) {
				warn(fmt.Sprintf("plugin discovery: read %s: %v", r.Path, err))
			}
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			pluginDir := filepath.Join(r.Path, e.Name())
			manifestPath := filepath.Join(pluginDir, "manifest.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				if !os.IsNotExist(err) {
					warn(fmt.Sprintf("plugin discovery: read %s: %v", manifestPath, err))
				}
				continue
			}
			m, err := manifest.Parse(data)
			if err != nil {
				warn(fmt.Sprintf("plugin discovery: parse %s: %v", manifestPath, err))
				continue
			}
			if m.Plugin != e.Name() {
				warn(fmt.Sprintf("plugin discovery: %s declares plugin name %q but is installed as %q — skipping",
					manifestPath, m.Plugin, e.Name()))
				continue
			}
			binPath := filepath.Join(pluginDir, m.Binary)
			out = append(out, &Plugin{
				Dir:        pluginDir,
				Source:     r,
				Manifest:   m,
				BinaryPath: binPath,
			})
		}
	}
	return out
}

// assembleRoots resolves the discovery list from options + defaults.
// Lower Order is higher precedence.
func assembleRoots(opts DiscoverOptions) []Source {
	var roots []Source

	// Order 1: $KAPI_PLUGINS_DIR. May be a list.
	if opts.EnvPluginsDir != "" {
		paths := splitPathList(opts.EnvPluginsDir)
		for _, p := range paths {
			roots = append(roots, Source{
				Order: 1,
				Label: "$KAPI_PLUGINS_DIR",
				Path:  filepath.Clean(p),
			})
		}
	}

	// Dogfood / dev isolation: with OnlyEnvDir (or a non-empty
	// $KAPI_PLUGINS_DIR_ONLY) the user and system roots are skipped entirely,
	// so an in-repo kapi can't pick up globally-installed plugins.
	if opts.OnlyEnvDir || os.Getenv("KAPI_PLUGINS_DIR_ONLY") != "" {
		return roots
	}

	// Order 2: XDG data home / kapi / plugins.
	xdg := opts.XDGDataHome
	if xdg == "" {
		xdg = os.Getenv("XDG_DATA_HOME")
	}
	if xdg == "" {
		home := opts.HomeDir
		if home == "" {
			if h, err := os.UserHomeDir(); err == nil {
				home = h
			}
		}
		if home != "" {
			xdg = filepath.Join(home, ".local", "share")
		}
	}
	if xdg != "" {
		roots = append(roots, Source{
			Order: 2,
			Label: filepath.Join(xdg, "kapi", "plugins"),
			Path:  filepath.Join(xdg, "kapi", "plugins"),
		})
	}

	// Order 3: system dirs.
	systemDirs := opts.SystemDirs
	if systemDirs == nil {
		systemDirs = DefaultSystemDirs()
	}
	for _, p := range systemDirs {
		roots = append(roots, Source{
			Order: 3,
			Label: p,
			Path:  filepath.Clean(p),
		})
	}
	return roots
}

func splitPathList(s string) []string {
	sep := string(os.PathListSeparator)
	parts := strings.Split(s, sep)
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Roots returns the discovery roots that would be scanned for the given
// options. Useful for `kapi plugin list` to show users where plugins
// were searched for.
func Roots(opts DiscoverOptions) []Source {
	return assembleRoots(opts)
}
