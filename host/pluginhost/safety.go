package pluginhost

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
)

// A plugin is an executable kapi runs as the person, so kapi checks its
// binary, directory and root, in every root, at discovery and before a run.
// A path passes when the person or root owns it and nobody else may write it;
// group write is allowed only for root's or an administrators' group.
//
// Not covered: the directories above a plugin root; Windows, where access is
// an ACL kapi does not read, so the check refuses nothing there; and the
// moment between the check and the exec. web/docs/contribute/plugins.md says
// the same.

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

// fileFacts are what the rule reads about one path.
type fileFacts struct {
	mode     fs.FileMode
	uid, gid int
	hasOwner bool // the platform reports a numeric owner and group
}

// refusePath returns why a path with these facts must not lead to a binary
// kapi runs, or nil when it may, on the platform goos. me is the uid running
// kapi; adminGroup reports a group whose members can act as root.
func refusePath(goos, path string, f fileFacts, me int, adminGroup func(gid int) bool) error {
	if goos == "windows" {
		return nil // not covered, see above
	}
	path = filepath.Clean(path)
	perm := f.mode.Perm()
	if f.hasOwner && f.uid != me && f.uid != 0 {
		return fmt.Errorf("%s belongs to another user (uid %d), so another user could replace what kapi runs", path, f.uid)
	}
	if perm&0o002 != 0 {
		return fmt.Errorf("%s is writable by every user (mode %s), so another user could replace what kapi runs: run chmod o-w %s", path, perm, path)
	}
	if perm&0o020 != 0 && (!f.hasOwner || !adminGroup(f.gid)) {
		return fmt.Errorf("%s is writable by its group (gid %d, mode %s), so another user could replace what kapi runs: run chmod g-w %s", path, f.gid, perm, path)
	}
	return nil
}

// statFacts reads the facts of path, following links, and reports whether it
// exists.
var statFacts = func(path string) (fileFacts, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return fileFacts{}, false
	}
	uid, gid, ok := fileOwner(info)
	return fileFacts{mode: info.Mode(), uid: uid, gid: gid, hasOwner: ok}, true
}

// checkPath refuses a path the rule refuses on this platform. A path that
// does not exist is left to the caller.
func checkPath(path string) error {
	f, ok := statFacts(path)
	if !ok {
		return nil
	}
	return refusePath(runtime.GOOS, path, f, os.Getuid(), isAdminGroup)
}

// unsafePlugin returns why p must not be loaded or run, or nil when it may.
func unsafePlugin(p *Plugin) error {
	for _, path := range []string{p.BinaryPath, p.Dir, p.Source.Path} {
		if path == "" {
			continue
		}
		if err := checkPath(path); err != nil {
			return err
		}
	}
	return nil
}

// CheckLaunch is the check every launch of a plugin's binary makes at the
// moment it runs it, so a binary or directory changed since discovery (or a
// plugin taken from the dispatch cache) is refused rather than run.
func CheckLaunch(p *Plugin) error {
	if err := unsafePlugin(p); err != nil {
		return fmt.Errorf("plugin %s is not run: %w", p.Name(), err)
	}
	return nil
}

// adminGroupNames are the groups whose members can act as root, so group
// write by them gives nobody more than root already has.
var adminGroupNames = []string{"admin", "wheel", "sudo"}

var (
	adminGIDsOnce sync.Once
	adminGIDs     map[int]bool
)

// isAdminGroup reports root's group or an administrators' group.
func isAdminGroup(gid int) bool {
	if gid == 0 {
		return true
	}
	adminGIDsOnce.Do(func() {
		adminGIDs = map[int]bool{}
		for _, name := range adminGroupNames {
			if g, err := user.LookupGroup(name); err == nil {
				if id, err := strconv.Atoi(g.Gid); err == nil {
					adminGIDs[id] = true
				}
			}
		}
	})
	return adminGIDs[gid]
}

// Refusal is a plugin found in a discovery root and not loaded, with why.
type Refusal struct {
	Name   string `json:"name"`
	Dir    string `json:"dir"`
	Binary string `json:"binary,omitempty"`
	Reason string `json:"reason"`
}

// safePlugins splits plugins into the ones the rule lets kapi load and the
// ones it refuses, each refusal also reported through warn.
func safePlugins(plugins []*Plugin, warn func(string)) ([]*Plugin, []Refusal) {
	out := plugins[:0:0]
	var refused []Refusal
	for _, p := range plugins {
		if err := unsafePlugin(p); err != nil {
			warn(fmt.Sprintf("plugin %s is not loaded: %v", p.Name(), err))
			refused = append(refused, Refusal{Name: p.Name(), Dir: p.Dir, Binary: p.BinaryPath, Reason: err.Error()})
			continue
		}
		out = append(out, p)
	}
	return out, refused
}
