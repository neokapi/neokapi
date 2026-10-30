package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// fakeDesktop is a machine with Kapi Desktop installed at the given paths,
// and nothing on PATH.
func fakeDesktop(goos string, installed ...string) host.DesktopEnv {
	have := map[string]bool{}
	for _, p := range installed {
		have[p] = true
	}
	env := map[string]string{
		"HOME":         "/fake-home",
		"LOCALAPPDATA": `D:\AppData\Local`,
		"ProgramFiles": `C:\Program Files`,
	}
	return host.DesktopEnv{
		GOOS:     goos,
		Getenv:   func(k string) string { return env[k] },
		Exists:   func(p string) bool { return have[p] },
		LookPath: func(string) (string, error) { return "", errors.New("not on PATH") },
	}
}

// runOpen runs `kapi open` with a launcher that records what it would start.
func runOpen(t *testing.T, env host.DesktopEnv, args ...string) (string, [][]string, error) {
	t.Helper()
	var ran [][]string
	cmd := newOpenCmd(nil, desktopLauncher{env: env, run: func(_ context.Context, argv []string) error {
		ran = append(ran, argv)
		return nil
	}})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), ran, err
}

func TestOpenCmd_DryRunPrintsTheCommandForEachSystem(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	project, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(project, "kapi.yaml"), []byte("version: v1\nname: open\n"), 0o644))
	folder, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipe := filepath.Join(project, "kapi.yaml")

	for _, tt := range []struct {
		name      string
		env       host.DesktopEnv
		path      string
		want      string
		wantStart []string
	}{
		{name: "macOS, a project", env: fakeDesktop("darwin", "/Applications/Kapi.app"), path: project,
			want: "open -a /Applications/Kapi.app " + recipe + "\n", wantStart: []string{"open", "-a", "/Applications/Kapi.app", recipe}},
		{name: "macOS, an app in the home folder", env: fakeDesktop("darwin", "/fake-home/Applications/Kapi.app"), path: folder,
			want: "open -a /fake-home/Applications/Kapi.app " + folder + "\n"},
		{name: "macOS, the recipe itself", env: fakeDesktop("darwin", "/Applications/Kapi.app"), path: recipe,
			want: "open -a /Applications/Kapi.app " + recipe + "\n"},
		{name: "Linux", env: fakeDesktop("linux", "/fake-home/.local/bin/Kapi"), path: folder,
			want: "/fake-home/.local/bin/Kapi " + folder + "\n", wantStart: []string{"/fake-home/.local/bin/Kapi", folder}},
		{name: "Linux, /opt", env: fakeDesktop("linux", "/opt/Kapi/Kapi"), path: folder,
			want: "/opt/Kapi/Kapi " + folder + "\n"},
		{name: "Windows, a per-user install", env: fakeDesktop("windows", `D:\AppData\Local\Programs\Kapi\Kapi.exe`), path: folder,
			want: `D:\AppData\Local\Programs\Kapi\Kapi.exe ` + folder + "\n"},
		{name: "Windows, a machine install", env: fakeDesktop("windows", `C:\Program Files\Kapi\Kapi.exe`), path: folder,
			want: `"C:\Program Files\Kapi\Kapi.exe" ` + folder + "\n", wantStart: []string{`C:\Program Files\Kapi\Kapi.exe`, folder}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, ran, err := runOpen(t, tt.env, tt.path, "--dry-run")
			require.NoError(t, err)
			assert.Equal(t, tt.want, out)
			assert.Empty(t, ran, "--dry-run starts nothing")

			if tt.wantStart == nil {
				return
			}
			out, ran, err = runOpen(t, tt.env, tt.path)
			require.NoError(t, err)
			assert.Equal(t, [][]string{tt.wantStart}, ran)
			assert.Contains(t, out, "in Kapi Desktop\n")
		})
	}
}

// TestShellLine prints the command the way the system's shell takes it.
func TestShellLine(t *testing.T) {
	assert.Equal(t, `open -a /Applications/Kapi.app '/Volumes/team/My Site/kapi.yaml'`,
		shellLine("darwin", []string{"open", "-a", "/Applications/Kapi.app", "/Volumes/team/My Site/kapi.yaml"}))
	assert.Equal(t, `/opt/Kapi/Kapi '/srv/it'\''s here'`, shellLine("linux", []string{"/opt/Kapi/Kapi", "/srv/it's here"}))
	assert.Equal(t, `"C:\Program Files\Kapi\Kapi.exe" "D:\My Site"`,
		shellLine("windows", []string{`C:\Program Files\Kapi\Kapi.exe`, `D:\My Site`}))
	assert.Equal(t, `D:\AppData\Local\Programs\Kapi\Kapi.exe D:\site`,
		shellLine("windows", []string{`D:\AppData\Local\Programs\Kapi\Kapi.exe`, `D:\site`}), "a backslash is no quote in cmd")
}

func TestOpenCmd_FindsTheAppOnPath(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	folder := t.TempDir()
	env := fakeDesktop("linux")
	env.LookPath = func(name string) (string, error) {
		if name == "Kapi" {
			return "/usr/bin/Kapi", nil
		}
		return "", errors.New("not on PATH")
	}
	out, _, err := runOpen(t, env, folder, "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "/usr/bin/Kapi ")
}

func TestOpenCmd_NotInstalled(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	folder := t.TempDir()
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			_, ran, err := runOpen(t, fakeDesktop(goos), folder)
			require.Error(t, err)
			assert.Empty(t, ran)
			assert.Equal(t, 1, host.ExitCode(nil, err))
			assert.Contains(t, err.Error(), "Kapi Desktop is not installed on this machine.")
			assert.Contains(t, err.Error(), host.DesktopDownloadURL)
			if goos == "darwin" {
				assert.Contains(t, err.Error(), "brew install --cask neokapi/tap/kapi")
			} else {
				assert.NotContains(t, err.Error(), "brew")
			}
		})
	}
}
