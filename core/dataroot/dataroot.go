// Package dataroot resolves kapi's per-user data root, below every module that
// needs it: the host, the project store and the plugins all reach the same
// directory through Dir.
//
// kapi keeps two per-user roots apart. The configuration directory
// (host/resource.go) holds what a person configures and can hand-edit: the
// global kapi.yaml, providers.json, named stores, installed plugins. The data
// root holds what kapi records for itself, which nobody edits and which
// follows the platform's data convention rather than its configuration one.
package dataroot

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EnvDir names the environment variable that overrides the data root
// outright. Setting it to a throwaway directory is how a test, a sandbox or an
// in-repo invocation keeps out of the developer's own data.
const EnvDir = "KAPI_DATA_DIR"

// dataDirName is the directory kapi owns under whichever root the platform
// gives it.
const dataDirName = "kapi"

// Dir returns kapi's per-user data root, resolved in four steps:
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
// Inside a `go test` binary, unless $KAPI_DATA_DIR names a root outright, the
// answer is a directory of this process's own under the system temporary
// directory, so a test never writes into the developer's workspace. A released
// binary resolves the platform default, which is why the isolation contract
// sets $KAPI_DATA_DIR on a kapi it launches.
//
// Outside a test binary the directory is named, not created; a caller that
// writes there creates it. A test binary's root is created, private to its
// user, when it is first resolved.
func Dir() string {
	if dir := os.Getenv(EnvDir); dir != "" {
		return dir
	}
	if UnderTest() {
		return testDataDir()
	}
	return dataDir(os.Getenv, runtime.GOOS)
}

// UnderTest reports whether this process is a test binary.
//
// The testing package installs its flags before any test runs, so `test.v`
// existing is the signal; the executable's name is the belt for a binary that
// has not reached testing.Init yet.
func UnderTest() bool {
	if flag.Lookup("test.v") != nil {
		return true
	}
	base := filepath.Base(os.Args[0])
	return strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe")
}

// testDataDirPrefix begins the name of a test binary's data root under the
// system temporary directory. The process id follows it.
const testDataDirPrefix = "kapi-test-data-"

// testRootOnce sweeps and creates the test binary's data root the first time
// it is resolved, before anything has been written there; testRootErr holds
// why the root could not be made safe.
var (
	testRootOnce sync.Once
	testRootErr  error
)

// testDataDir is the data root a test binary gets: one directory per process,
// under the system temporary directory, so two packages running in parallel do
// not share a workspace and neither reaches the developer's.
//
// The first call sweeps the roots dead test binaries left behind (a killed
// binary, or one with no TestMain calling RemoveTestDir, leaves its root,
// often hundreds of megabytes).
//
// The root is created with mode 0700 and checked once per process
// (ensurePrivateDir). Its name carries the process id the sweep reads, so on a
// shared temporary directory another user could create it first. A root that
// is a link, belongs to another user, or is open to others stops the test
// binary with a panic.
func testDataDir() string {
	root := testDataRoot(os.TempDir(), os.Getpid())
	testRootOnce.Do(func() {
		sweepTestDataDirs(os.TempDir(), os.Getpid(), processAlive, processStart)
		testRootErr = ensurePrivateDir(root)
	})
	if testRootErr != nil {
		panic("dataroot: the test data root is unsafe to use: " + testRootErr.Error())
	}
	return root
}

// TestUserDir answers, in a test binary, a directory of this process's own
// for one kind of per-user location ("config", "home"), under the test
// binary's data root, and true. Outside a test binary it answers "" and false,
// and the caller resolves the platform's location.
//
// A test that points HOME (or, for "config", XDG_CONFIG_HOME) somewhere other
// than where the process started has chosen its own tree, and gets "" and false
// too, so it resolves through the platform exactly as a user would, under the
// directory it chose. A test that leaves them alone would otherwise reach the
// developer's own configuration, caches, plugins or home; it reaches a
// throwaway directory instead. The sweep and RemoveTestDir clean it with the
// rest of the root.
func TestUserDir(kind string) (string, bool) {
	if !UnderTest() {
		return "", false
	}
	if os.Getenv("HOME") != startHome || (kind == "config" && os.Getenv("XDG_CONFIG_HOME") != startXDGConfig) {
		return "", false
	}
	user := filepath.Join(testDataDir(), "user")
	dir := filepath.Join(user, kind)
	errAny, _ := testUserDirs.LoadOrStore(kind, sync.OnceValue(func() error {
		if err := ensurePrivateDir(user); err != nil {
			return err
		}
		return ensurePrivateDir(dir)
	}))
	if err := errAny.(func() error)(); err != nil {
		panic("dataroot: the test user directory is unsafe to use: " + err.Error())
	}
	return dir, true
}

// testUserDirs holds, per kind, the once-per-process check that creates the
// kind's directory under the test root and makes sure it is private.
var testUserDirs sync.Map

// The home and configuration variables as the process started, before any
// test changed them.
var (
	startHome      = os.Getenv("HOME")
	startXDGConfig = os.Getenv("XDG_CONFIG_HOME")
)

// UserConfigDir is the platform's per-user configuration base
// (os.UserConfigDir, $HOME/.config when it has none), or in a test binary the
// process's own (TestUserDir). Every kapi path under the user's configuration
// resolves through it, after its own explicit override.
func UserConfigDir() string {
	if d, ok := TestUserDir("config"); ok {
		return d
	}
	d, err := os.UserConfigDir()
	if err != nil || d == "" {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return d
}

// UserHomeDir is the user's home directory (os.UserHomeDir), or in a test
// binary the process's own (TestUserDir). The caches, the per-user plugin
// root and the agent configuration kapi writes under the home resolve through
// it, after their own explicit override.
func UserHomeDir() (string, error) {
	if d, ok := TestUserDir("home"); ok {
		return d, nil
	}
	return os.UserHomeDir()
}

// testDataRoot names the data root of the test binary with this process id.
func testDataRoot(tmp string, pid int) string {
	return filepath.Join(tmp, testDataDirPrefix+strconv.Itoa(pid))
}

// RemoveTestDir removes the data root this test binary was given, once its
// tests have run. A package's TestMain calls it through devenvtest.Main, so a
// test run leaves nothing behind under the system temporary directory. Outside
// a test binary it does nothing.
func RemoveTestDir() {
	if !UnderTest() {
		return
	}
	root := testDataRoot(os.TempDir(), os.Getpid())
	if info, err := os.Lstat(root); err != nil || checkOwnedDir(root, info) != nil {
		return
	}
	_ = os.RemoveAll(root)
}

// processStartSlack is how much earlier than its process's start a data root
// may have been last written and still count as that process's, which absorbs
// the clock's resolution.
const processStartSlack = 2 * time.Second

// sweepTestDataDirs removes from tmp the data root of every test binary that is
// no longer running, and returns the names it removed. alive answers whether a
// process id is running, and started when it started, where the platform says.
//
// The root named for self is removed as well. The sweep runs before this
// process has resolved its own root, so a directory already carrying its id
// was left by an earlier process that had the same id, and its workspaces
// would otherwise leak into this run's tests.
//
// A root whose process is running is kept, unless that process started after
// the root was last written: a binary writes its root after it starts, so such
// a process took the id over from the binary that wrote the root, which has
// exited. Without a start time, such a root is kept until that process ends.
func sweepTestDataDirs(tmp string, self int, alive func(int) bool, started func(int) (time.Time, bool)) []string {
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return nil
	}
	var removed []string
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), testDataDirPrefix)
		if !ok {
			continue
		}
		// Only a real directory this user owns: never a link, never another
		// user's directory, so the sweep cannot be steered into deleting
		// anything else.
		path := filepath.Join(tmp, e.Name())
		if info, err := os.Lstat(path); err != nil || checkOwnedDir(path, info) != nil {
			continue
		}
		pid, err := strconv.Atoi(rest)
		if err != nil || pid <= 0 || strconv.Itoa(pid) != rest {
			continue
		}
		if pid != self && alive(pid) && !reusedID(e, pid, started) {
			continue
		}
		if os.RemoveAll(path) == nil {
			removed = append(removed, e.Name())
		}
	}
	return removed
}

// reusedID reports whether the running process with this id started after the
// data root e was last written, so that it cannot be the binary that wrote it.
func reusedID(e os.DirEntry, pid int, started func(int) (time.Time, bool)) bool {
	at, ok := started(pid)
	if !ok {
		return false
	}
	info, err := e.Info()
	if err != nil {
		return false
	}
	return info.ModTime().Before(at.Add(-processStartSlack))
}

// dataDir is DataDir with its two environment seams injected, so the platform
// branches are reachable from a test on any host.
func dataDir(getenv func(string) string, goos string) string {
	if dir := getenv(EnvDir); dir != "" {
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

// NormalizePath renders a file or directory in the one spelling two
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
// The workspace registry, the diff scope keys and the pre-edit write guard all
// compare paths through it.
func NormalizePath(p string) string {
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
