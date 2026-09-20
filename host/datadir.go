package host

import (
	"os"
	"path/filepath"
	"runtime"
)

// kapi keeps two per-user roots apart. ConfigDir (host/resource.go) holds what
// a person configures and can hand-edit: the global kapi.yaml, providers.json,
// named stores, installed plugins. DataDir holds what kapi records for itself
// across projects, which nobody edits and which follows the platform's data
// convention rather than its configuration one.

// EnvDataDir names the environment variable that overrides the data root
// outright. Setting it to a throwaway directory is how a test, a sandbox or an
// in-repo invocation keeps out of the developer's own data.
const EnvDataDir = "KAPI_DATA_DIR"

// dataDirName is the directory kapi owns under whichever root the platform
// gives it.
const dataDirName = "kapi"

// DataDir returns kapi's per-user data root, resolved in four steps:
//
//	$KAPI_DATA_DIR            the whole path, used as given
//	$XDG_DATA_HOME/kapi       on every platform, when the variable is set
//	macOS                     ~/Library/Application Support/kapi
//	Linux and the rest        ~/.local/share/kapi
//	Windows                   %LocalAppData%\kapi
//
// XDG_DATA_HOME is honoured off Linux too, because that is the variable the
// in-repo isolation contract already sets and a macOS run that ignored it would
// write into the developer's real data root while every other path stayed
// sandboxed.
//
// The directory is named, not created. A caller that writes there creates it.
func DataDir() string {
	return dataDir(os.Getenv, runtime.GOOS)
}

// dataDir is DataDir with its two environment seams injected, so the platform
// branches are reachable from a test on any host.
func dataDir(getenv func(string) string, goos string) string {
	if dir := getenv(EnvDataDir); dir != "" {
		return dir
	}
	if dir := getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, dataDirName)
	}
	home := homeDir(getenv, goos)
	switch goos {
	case "windows":
		if local := getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, dataDirName)
		}
		return filepath.Join(home, "AppData", "Local", dataDirName)
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", dataDirName)
	default:
		return filepath.Join(home, ".local", "share", dataDirName)
	}
}

// homeDir reads the home directory from the variable the platform sets, which
// is what os.UserHomeDir consults as well.
func homeDir(getenv func(string) string, goos string) string {
	if goos == "windows" {
		return getenv("USERPROFILE")
	}
	return getenv("HOME")
}

// NormalizeCheckoutPath renders a directory in the one spelling two paths to
// the same place share, so a comparison between them answers what a person
// means by "the same checkout".
//
// A path is made absolute and then resolved through every symlink on it. That
// second step is what the comparison needs: on macOS a temporary directory is
// handed out as /var/... and resolves to /private/var/..., and a checkout
// reached through a symlinked parent spells the same tree two ways.
//
// A path that does not exist resolves as far as its nearest existing ancestor
// and keeps the rest verbatim, so a directory about to be created compares
// equal to the same directory once it is there. With nothing on the path
// resolvable, the cleaned absolute path stands.
func NormalizeCheckoutPath(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}

	// Walk up to the deepest ancestor that exists, resolve that, and rejoin
	// what was below it.
	rest := ""
	dir := abs
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			continue
		}
		return filepath.Join(resolved, rest)
	}
}
