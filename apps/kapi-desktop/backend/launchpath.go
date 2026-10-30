package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
)

// A project reaches the app from outside it in three ways. macOS hands it a
// path as an opened file (`open -a Kapi <folder>`, a drop on the dock icon).
// Linux and Windows start it with the path as its first argument. A second
// launch on Linux or Windows forwards its arguments to the window already
// open. `kapi open` uses the first two.

// LaunchPath is the project path among a launch's arguments: the first one
// that is not a flag and names a file or folder that exists, resolved against
// dir, the directory the launch ran in. It is empty when the launch named
// none.
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
		return OutsidePath(arg)
	}
	return ""
}

// OutsidePath is the path OpenProject takes for a path handed to the app from
// outside: the recipe of a folder that holds one, and the path as given
// otherwise.
func OutsidePath(path string) string {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return path
	}
	recipe := filepath.Join(path, project.RecipeFileName)
	if _, err := os.Stat(recipe); err == nil {
		return recipe
	}
	return path
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
	sum := sha256.Sum256([]byte(exe))
	return bundleID + "." + hex.EncodeToString(sum[:6])
}

// bundleID is the app's bundle identifier (build/config.yml).
const bundleID = "io.github.neokapi.kapi-desktop"
