package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envOf turns a map into the lookup seam dataDir takes, so a case names only
// the variables it sets and every other read answers empty.
func envOf(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

// Every resolution branch on every platform, so a rule that holds only where
// the test happens to run cannot ship.
func TestDataDir(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{
			name: "KAPI_DATA_DIR wins on darwin",
			goos: "darwin",
			env:  map[string]string{EnvDataDir: "/tmp/iso/data", "XDG_DATA_HOME": "/xdg", "HOME": "/fakehome"},
			want: "/tmp/iso/data",
		},
		{
			name: "KAPI_DATA_DIR wins on linux",
			goos: "linux",
			env:  map[string]string{EnvDataDir: "/tmp/iso/data", "XDG_DATA_HOME": "/xdg", "HOME": "/home/x"},
			want: "/tmp/iso/data",
		},
		{
			name: "KAPI_DATA_DIR wins on windows",
			goos: "windows",
			env:  map[string]string{EnvDataDir: `C:\iso\data`, "LOCALAPPDATA": `C:\fakeprofile\AppData\Local`},
			want: `C:\iso\data`,
		},
		{
			name: "XDG_DATA_HOME is honoured on darwin",
			goos: "darwin",
			env:  map[string]string{"XDG_DATA_HOME": "/xdg", "HOME": "/fakehome"},
			want: filepath.Join("/xdg", "kapi"),
		},
		{
			name: "XDG_DATA_HOME is honoured on linux",
			goos: "linux",
			env:  map[string]string{"XDG_DATA_HOME": "/xdg", "HOME": "/home/x"},
			want: filepath.Join("/xdg", "kapi"),
		},
		{
			name: "XDG_DATA_HOME is honoured on windows",
			goos: "windows",
			env:  map[string]string{"XDG_DATA_HOME": `C:\xdg`, "LOCALAPPDATA": `C:\fakeprofile\AppData\Local`},
			want: filepath.Join(`C:\xdg`, "kapi"),
		},
		{
			name: "darwin default",
			goos: "darwin",
			env:  map[string]string{"HOME": "/fakehome"},
			want: filepath.Join("/fakehome", "Library", "Application Support", "kapi"),
		},
		{
			name: "linux default",
			goos: "linux",
			env:  map[string]string{"HOME": "/home/x"},
			want: filepath.Join("/home/x", ".local", "share", "kapi"),
		},
		{
			name: "freebsd follows the linux default",
			goos: "freebsd",
			env:  map[string]string{"HOME": "/home/x"},
			want: filepath.Join("/home/x", ".local", "share", "kapi"),
		},
		{
			name: "windows default",
			goos: "windows",
			env:  map[string]string{"LOCALAPPDATA": `C:\fakeprofile\AppData\Local`, "USERPROFILE": `C:\fakeprofile`},
			want: filepath.Join(`C:\fakeprofile\AppData\Local`, "kapi"),
		},
		{
			name: "windows without LOCALAPPDATA falls back to the profile",
			goos: "windows",
			env:  map[string]string{"USERPROFILE": `C:\fakeprofile`},
			want: filepath.Join(`C:\fakeprofile`, "AppData", "Local", "kapi"),
		},
		{
			name: "an empty override is no override",
			goos: "linux",
			env:  map[string]string{EnvDataDir: "", "XDG_DATA_HOME": "", "HOME": "/home/x"},
			want: filepath.Join("/home/x", ".local", "share", "kapi"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, dataDir(envOf(tt.env), tt.goos))
		})
	}
}

// The exported entry point reads the real process environment, so the isolation
// contract's throwaway directory reaches it.
func TestDataDirReadsTheProcessEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDataDir, dir)
	assert.Equal(t, dir, DataDir())

	t.Setenv(EnvDataDir, "")
	t.Setenv("XDG_DATA_HOME", dir)
	assert.Equal(t, filepath.Join(dir, "kapi"), DataDir())
}

// Two spellings of one checkout normalize to the same string; two different
// checkouts do not.
func TestNormalizeCheckoutPath(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	require.NoError(t, os.MkdirAll(filepath.Join(real, "nested"), 0o755))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(real, link))

	want := NormalizeCheckoutPath(real)
	require.NotEmpty(t, want)

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "the path itself", path: real, want: want},
		{name: "an unclean spelling", path: filepath.Join(real, "nested", ".."), want: want},
		{name: "a trailing separator", path: real + string(filepath.Separator), want: want},
		{name: "through a symlink", path: link, want: want},
		{name: "through a symlinked parent", path: filepath.Join(link, "nested"), want: filepath.Join(want, "nested")},
		{name: "a child that does not exist yet", path: filepath.Join(link, "absent"), want: filepath.Join(want, "absent")},
		{name: "a deep path that does not exist yet", path: filepath.Join(link, "a", "b", "c"), want: filepath.Join(want, "a", "b", "c")},
		{name: "empty stays empty", path: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeCheckoutPath(tt.path))
		})
	}

	other := filepath.Join(root, "other")
	require.NoError(t, os.MkdirAll(other, 0o755))
	assert.NotEqual(t, want, NormalizeCheckoutPath(other), "two checkouts stay two checkouts")
}

// A relative path normalizes against the working directory, so a comparison
// does not depend on how the caller spelled it.
func TestNormalizeCheckoutPathIsAbsolute(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	assert.Equal(t, NormalizeCheckoutPath(dir), NormalizeCheckoutPath("."))
}
