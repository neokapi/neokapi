package host

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

// testDataDirPrefix begins the name of a test binary's data root under the
// system temporary directory. The process id follows it.
const testDataDirPrefix = "kapi-test-data-"

// sweepTestDataDirsOnce runs the sweep the first time a test binary resolves
// its data root, before anything has been written there.
var sweepTestDataDirsOnce sync.Once

// testDataDir is the data root a test binary gets: one directory per process,
// under the system temporary directory, so two packages running in parallel do
// not share a workspace and neither reaches the developer's.
//
// The first call sweeps the roots dead test binaries left behind, so a run
// does not add to them without bound: a root holds every workspace its
// binary's tests opened, often hundreds of megabytes, and a binary that is
// killed, or whose package has no TestMain calling RemoveTestDataDir, leaves
// its root behind.
func testDataDir() string {
	sweepTestDataDirsOnce.Do(func() {
		sweepTestDataDirs(os.TempDir(), os.Getpid(), processAlive)
	})
	return testDataRoot(os.TempDir(), os.Getpid())
}

// testDataRoot names the data root of the test binary with this process id.
func testDataRoot(tmp string, pid int) string {
	return filepath.Join(tmp, testDataDirPrefix+strconv.Itoa(pid))
}

// RemoveTestDataDir removes the data root this test binary was given, once its
// tests have run. A package's TestMain calls it through devenvtest.Main, so a
// test run leaves nothing behind under the system temporary directory. Outside
// a test binary it does nothing.
func RemoveTestDataDir() {
	if !underTest() {
		return
	}
	_ = os.RemoveAll(testDataRoot(os.TempDir(), os.Getpid()))
}

// sweepTestDataDirs removes from tmp the data root of every test binary that is
// no longer running, and returns the names it removed. alive answers whether a
// process id is running.
//
// The root named for self is removed as well. The sweep runs before this
// process has resolved its own root, so a directory already carrying its id
// was left by an earlier process that had the same id, and its workspaces
// would otherwise leak into this run's tests.
//
// A root whose process is running is kept, and so is one whose id another
// process has taken since: the next sweep after that process ends removes it.
func sweepTestDataDirs(tmp string, self int, alive func(int) bool) []string {
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return nil
	}
	var removed []string
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), testDataDirPrefix)
		if !ok || !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(rest)
		if err != nil || pid <= 0 || strconv.Itoa(pid) != rest {
			continue
		}
		if pid != self && alive(pid) {
			continue
		}
		if os.RemoveAll(filepath.Join(tmp, e.Name())) == nil {
			removed = append(removed, e.Name())
		}
	}
	return removed
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

// NormalizeCheckoutPath renders a file or directory in the one spelling two
// paths to the same place share, so a comparison between them answers what a
// person means by "the same checkout", "the same file".
//
// A path is made absolute and then resolved through every symlink on it. That
// second step is what the comparison needs: on macOS a temporary directory is
// handed out as /var/... and resolves to /private/var/..., and a checkout
// reached through a symlinked parent spells the same tree two ways.
//
// A path that does not exist resolves as far as its nearest existing ancestor
// and keeps the rest verbatim, so a file about to be written compares equal to
// the same file once it is there. With nothing on the path resolvable, the
// cleaned absolute path stands.
//
// It is the one normalizer: the workspace registry, the diff scope keys and the
// pre-edit write guard all compare paths through it.
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
