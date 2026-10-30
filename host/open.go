package host

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/neokapi/neokapi/core/project"
)

// `kapi open`: Kapi Desktop on a project folder.
//
// The app takes the folder or its recipe from outside in two ways. On macOS,
// `open -a` hands it the path, which the app receives as an opened file. On
// Linux and Windows the path is the app's first argument, and a second launch
// passes it to the window already open.

// DesktopCask is the Homebrew cask that installs Kapi Desktop, and
// DesktopDownloadURL the page that lists every download.
const (
	DesktopCask        = "neokapi/tap/kapi"
	DesktopDownloadURL = "https://neokapi.github.io/kapi/get-started/installation#install-kapi-desktop"
)

// ErrDesktopNotInstalled reports that Kapi Desktop was not found on this
// machine.
var ErrDesktopNotInstalled = errors.New("kapi desktop is not installed")

// DesktopEnv is what finding and launching Kapi Desktop reads from the
// machine. The zero value reads this machine; tests set each field.
type DesktopEnv struct {
	// GOOS is the operating system: darwin, linux or windows.
	GOOS string
	// Getenv reads an environment variable.
	Getenv func(string) string
	// Exists reports whether a path exists.
	Exists func(string) bool
	// LookPath finds an executable on PATH.
	LookPath func(string) (string, error)
}

// withDefaults fills the fields a caller left empty from this machine.
func (e DesktopEnv) withDefaults(goos string) DesktopEnv {
	if e.GOOS == "" {
		e.GOOS = goos
	}
	if e.Getenv == nil {
		e.Getenv = os.Getenv
	}
	if e.Exists == nil {
		e.Exists = func(p string) bool { _, err := os.Stat(p); return err == nil }
	}
	if e.LookPath == nil {
		e.LookPath = exec.LookPath
	}
	return e
}

// desktopCandidates are the places an install puts Kapi Desktop, in the order
// they are tried: the macOS app bundle, the Windows installer's two scopes,
// the binary the Linux archive holds.
func desktopCandidates(e DesktopEnv) []string {
	home := e.Getenv("HOME")
	switch e.GOOS {
	case "darwin":
		out := []string{"/Applications/Kapi.app"}
		if home != "" {
			out = append(out, filepath.Join(home, "Applications", "Kapi.app"))
		}
		return out
	case "windows":
		var out []string
		if dir := e.Getenv("LOCALAPPDATA"); dir != "" {
			out = append(out, dir+`\Programs\Kapi\Kapi.exe`)
		}
		if dir := e.Getenv("ProgramFiles"); dir != "" {
			out = append(out, dir+`\Kapi\Kapi.exe`)
		}
		return out
	}
	var out []string
	if home != "" {
		out = append(out, filepath.Join(home, ".local", "bin", "Kapi"))
	}
	return append(out, "/opt/Kapi/Kapi", "/usr/local/bin/Kapi")
}

// FindDesktop returns where Kapi Desktop is installed, or
// ErrDesktopNotInstalled.
func FindDesktop(env DesktopEnv) (string, error) {
	e := env.withDefaults(runtime.GOOS)
	for _, p := range desktopCandidates(e) {
		if e.Exists(p) {
			return p, nil
		}
	}
	if e.GOOS != "darwin" {
		name := "Kapi"
		if e.GOOS == "windows" {
			name = "Kapi.exe"
		}
		if p, err := e.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", ErrDesktopNotInstalled
}

// DesktopOpenCommand is the command that opens target in the Kapi Desktop
// installed at app.
func DesktopOpenCommand(goos, app, target string) []string {
	if goos == "darwin" {
		return []string{"open", "-a", app, target}
	}
	return []string{app, target}
}

// DesktopTarget is what Kapi Desktop is handed for a folder: its recipe when
// it has one, the folder itself when it has none.
func DesktopTarget(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		if filepath.Base(abs) == project.RecipeFileName {
			return abs, nil
		}
		abs = filepath.Dir(abs)
	}
	recipe := filepath.Join(abs, project.RecipeFileName)
	if _, err := os.Stat(recipe); err == nil {
		return recipe, nil
	}
	return abs, nil
}

// DesktopNotInstalledError says where to get Kapi Desktop.
func DesktopNotInstalledError(goos string) error {
	var b strings.Builder
	b.WriteString("Kapi Desktop is not installed on this machine.\n")
	if goos == "darwin" {
		fmt.Fprintf(&b, "Install it with: brew install --cask %s\n", DesktopCask)
	}
	fmt.Fprintf(&b, "Downloads for every system: %s", DesktopDownloadURL)
	return WithExitCode(ExitError, errors.New(b.String()))
}
