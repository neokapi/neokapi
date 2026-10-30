package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// A project reaches the app from outside it in three ways. macOS hands it a
// path as an opened file (`open -a Kapi <folder>`, a drop on the dock icon).
// Linux and Windows start it with the path as its first argument. A second
// launch on Linux or Windows forwards its arguments to the window already
// open. `kapi open` uses the first two.

// LaunchPath is the project path among a launch's arguments: the first one
// that is not a flag and names a file or folder that exists, resolved against
// dir, the directory the launch ran in. It is empty when the launch named
// none. OpenProject takes the path as it is, a folder or a recipe.
func LaunchPath(args []string, dir string) string {
	for _, arg := range args {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		if !filepath.IsAbs(arg) {
			arg = filepath.Join(dir, arg)
		}
		if _, err := os.Stat(arg); err != nil {
			continue
		}
		return arg
	}
	return ""
}

// SingleInstanceID names this installation of the app for the single-instance
// lock: the bundle identifier and the executable's path, so a second launch of
// the same binary reaches the window already open while a development build
// beside an installed one keeps its own window.
func SingleInstanceID() string {
	exe, err := os.Executable()
	if err != nil {
		return bundleID
	}
	return singleInstanceIDFor(exe)
}

// singleInstanceIDFor is the id for the executable at exe. On Linux the id is
// a D-Bus name, whose elements may not start with a digit, so the digest's
// element starts with a letter.
func singleInstanceIDFor(exe string) string {
	sum := sha256.Sum256([]byte(exe))
	return bundleID + ".x" + hex.EncodeToString(sum[:6])
}

// SingleInstanceAvailable reports whether this session can hold the
// single-instance lock. On Linux the lock is a name on the D-Bus session bus,
// and a session with no bus (a bare X session, a container) starts without it.
func SingleInstanceAvailable(goos string, getenv func(string) string) bool {
	switch goos {
	case "darwin":
		return false
	case "linux":
		return getenv("DBUS_SESSION_BUS_ADDRESS") != ""
	}
	return true
}

// bundleID is the app's bundle identifier (build/config.yml).
const bundleID = "io.github.neokapi.kapi-desktop"

// OutsideOpener hands project paths from outside the app to the function that
// opens them. A second launch can forward its path before the window exists;
// that path waits here and opens once the window can show it.
type OutsideOpener struct {
	mu      sync.Mutex
	open    func(path string)
	pending []string
}

// Open opens path, or keeps it until SetOpen names how.
func (o *OutsideOpener) Open(path string) {
	o.mu.Lock()
	open := o.open
	if open == nil {
		o.pending = append(o.pending, path)
	}
	o.mu.Unlock()
	if open != nil {
		open(path)
	}
}

// SetOpen names how a path opens, and opens the paths that arrived before.
func (o *OutsideOpener) SetOpen(open func(path string)) {
	o.mu.Lock()
	o.open = open
	pending := o.pending
	o.pending = nil
	o.mu.Unlock()
	for _, path := range pending {
		open(path)
	}
}
