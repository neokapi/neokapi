package pluginhost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// A plugin is an executable kapi runs as the person, so everything on the
// path to its binary must be writable by that person alone: anyone else who
// can write there can replace what kapi executes. kapi creates its own plugin
// directories with mode 0700 (pluginDirMode). A plugin under a directory that
// group or others can write is not loaded, except in a system root (Homebrew,
// /usr/local, /usr/share), which the system's package manager owns and kapi
// did not create.

// pluginDirMode is the mode kapi creates a plugin root with.
const pluginDirMode = 0o700

// installFileMask caps the mode of every file kapi unpacks from a plugin
// archive: whatever the archive records, nothing kapi writes is writable by
// group or others, so a plugin kapi installed is never refused below.
const installFileMask = 0o755

// errNoUserDataDir is returned when no per-user data directory can be
// determined to install plugins into.
var errNoUserDataDir = errors.New("cannot determine a per-user directory to install plugins into: " +
	"set XDG_DATA_HOME (or HOME), or name one with KAPI_PLUGINS_DIR and KAPI_PLUGINS_DIR_ONLY=1")

// unsafePlugin returns why p must not be loaded, or nil when it may: a plugin
// outside a system root whose binary, directory or root is writable by group
// or others, or owned by another user.
func unsafePlugin(p *Plugin) error {
	if runtime.GOOS == "windows" || p.Source.Order >= systemSourceOrder {
		return nil
	}
	for _, path := range []string{p.BinaryPath, p.Dir, p.Source.Path} {
		if path == "" {
			continue
		}
		if err := ownedAndPrivate(path); err != nil {
			return err
		}
	}
	return nil
}

// ownedAndPrivate refuses a path that group or others can write, or that
// belongs to another user. A path that does not exist is left to the caller.
func ownedAndPrivate(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s is writable by other users (mode %s), so another user could replace what kapi runs: run chmod go-w %s",
			filepath.Clean(path), info.Mode().Perm(), filepath.Clean(path))
	}
	// Root owns what the system installs; any other owner but the person
	// running kapi could replace the plugin.
	if owner, ok := fileOwnerOf(info); ok && owner != os.Getuid() && owner != 0 {
		return fmt.Errorf("%s belongs to another user (uid %d), so another user could replace what kapi runs", filepath.Clean(path), owner)
	}
	return nil
}

// fileOwnerOf is fileOwner, replaceable in a test that cannot chown.
var fileOwnerOf = fileOwner

// safePlugins is plugins without the ones unsafePlugin refuses, each refusal
// reported through warn.
func safePlugins(plugins []*Plugin, warn func(string)) []*Plugin {
	out := plugins[:0:0]
	for _, p := range plugins {
		if err := unsafePlugin(p); err != nil {
			warn(fmt.Sprintf("plugin %s is not loaded: %v", p.Name(), err))
			continue
		}
		out = append(out, p)
	}
	return out
}
