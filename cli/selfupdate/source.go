// Package selfupdate gives the kapi CLI a claude-code-style update story:
// detect how the binary was installed, check (cached, non-blocking) whether a
// newer release exists, and either self-replace the binary (for direct/tarball
// installs the CLI owns) or nudge the user toward the exact package-manager
// command (for brew/winget/apt installs the package manager owns).
//
// The cardinal rule: a binary installed by a package manager must be updated
// *by that package manager* — overwriting it in place corrupts the manager's
// recorded state. So the install source decides which path is legal.
package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/neokapi/neokapi/core/version"
)

// Source is how this binary was distributed.
type Source string

const (
	// SourceHomebrew, SourceWinget, SourceScoop, SourceDeb, SourceRPM are
	// package-manager installs: the manager owns the file, so the CLI must
	// nudge rather than self-replace.
	SourceHomebrew Source = "homebrew"
	SourceWinget   Source = "winget"
	SourceScoop    Source = "scoop"
	SourceDeb      Source = "deb"
	SourceRPM      Source = "rpm"

	// SourceTarball is a direct download / curl|sh install the CLI owns and
	// may self-replace.
	SourceTarball Source = "tarball"

	// SourceUnknown is a build-from-source / `go install` / unrecognized
	// layout. Self-replace is allowed only if the binary's directory is
	// writable; otherwise we fall back to a nudge.
	SourceUnknown Source = "unknown"
)

// Managed reports whether a package manager owns this install (so the CLI
// must defer the upgrade to it rather than self-replacing).
func (s Source) Managed() bool {
	switch s {
	case SourceHomebrew, SourceWinget, SourceScoop, SourceDeb, SourceRPM:
		return true
	default:
		return false
	}
}

// Detect resolves the install source using, in order of confidence:
//
//  1. the KAPI_INSTALL_SOURCE env override (tests, power users);
//  2. the build-time version.InstallSource ldflag (most reliable — stamped per
//     packaging channel at release time);
//  3. path heuristics on the running executable;
//  4. SourceUnknown.
func Detect() Source {
	if env := strings.TrimSpace(os.Getenv("KAPI_INSTALL_SOURCE")); env != "" {
		return Source(strings.ToLower(env))
	}
	if stamped := strings.TrimSpace(version.InstallSource); stamped != "" {
		return Source(strings.ToLower(stamped))
	}
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		if s := sourceFromPath(exe); s != SourceUnknown {
			return s
		}
	}
	return SourceUnknown
}

// sourceFromPath infers the install source from the executable's location.
// Package-manager layouts are unambiguous; deb/rpm are deliberately left to
// the build-time stamp (a binary in /usr/bin tells us nothing reliable).
func sourceFromPath(exe string) Source {
	// Normalize both separators so Windows backslash paths match regardless of
	// the host OS running this check (filepath.ToSlash only rewrites the host's
	// own separator).
	p := strings.ReplaceAll(strings.ToLower(exe), `\`, "/")
	switch {
	case strings.Contains(p, "/cellar/"),
		strings.Contains(p, "/homebrew/"),
		strings.Contains(p, "/linuxbrew/"):
		return SourceHomebrew
	case strings.Contains(p, "/winget/packages/"):
		return SourceWinget
	case strings.Contains(p, "/scoop/"):
		return SourceScoop
	default:
		return SourceUnknown
	}
}

// CanSelfReplace reports whether the CLI may overwrite its own binary in place.
// Managed installs never qualify. Direct installs qualify only when the binary
// lives in a writable directory (a non-writable dir means a system-wide install
// we shouldn't touch without elevation — nudge instead).
func CanSelfReplace(s Source) bool {
	if s.Managed() {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return dirWritable(filepath.Dir(exe))
}

// dirWritable reports whether dir is writable by the current process by
// attempting to create (and immediately remove) a temp file in it.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".kapi-update-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// ExecutablePath returns the resolved path of the running binary.
func ExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// binaryName is the on-disk name of the kapi binary for this platform.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "kapi.exe"
	}
	return "kapi"
}
