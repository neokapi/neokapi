package host

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
// Inside a `go test` binary the resolution is different: unless $KAPI_DATA_DIR
// names a root outright, the answer is a directory of this process's own under
// the system temporary directory. The data root holds the workspace — the
// terms, the voice profiles, the content memory and the recorded decisions of
// every project on the machine — so a test that opens a project would otherwise
// write into the developer's own context and read it back on the next run.
// $KAPI_DATA_DIR stays the way a test names a root deliberately, and it is what
// the isolation contract sets on a kapi it launches as a subprocess, since a
// released binary is not a test binary and resolves the platform default.
//
// The directory is named, not created. A caller that writes there creates it.
func DataDir() string {
	if dir := os.Getenv(EnvDataDir); dir != "" {
		return dir
	}
	if underTest() {
		return testDataDir()
	}
	return dataDir(os.Getenv, runtime.GOOS)
}

// underTest reports whether this process is a test binary.
//
// The testing package installs its flags before any test runs, so `test.v`
// existing is the signal; the executable's name is the belt for a binary that
// has not reached testing.Init yet.
func underTest() bool {
	if flag.Lookup("test.v") != nil {
		return true
	}
	base := filepath.Base(os.Args[0])
	return strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe")
}

// testDataDir is the data root a test binary gets: one directory per process,
// under the system temporary directory, so two packages running in parallel do
// not share a workspace and neither reaches the developer's.
func testDataDir() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("kapi-test-data-%d", os.Getpid()))
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

// WorkspacesDirName holds the workspaces under the data root, one directory
// each, and DefaultWorkspaceName is the one every machine account has without
// asking for it.
//
// A workspace holds the context of every project worked on from this account:
// the project registry, the context graph, and one context store per project
// (core/workspace). It is created on first use, and a recipe carries no binding
// to it, so moving a checkout or cloning it again reaches the same context.
const (
	WorkspacesDirName    = "workspaces"
	DefaultWorkspaceName = "default"
)

// DefaultWorkspaceDir returns this machine account's implicit workspace.
func DefaultWorkspaceDir() string {
	return filepath.Join(DataDir(), WorkspacesDirName, DefaultWorkspaceName)
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
