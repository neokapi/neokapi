package backend

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// TestLaunchPath reads the project path a launch names: the first argument
// that is not a flag and exists, resolved against the launch's directory. A
// folder is handed on as a folder; OpenProject finds its recipe.
func TestLaunchPath(t *testing.T) {
	withRecipe := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(withRecipe, project.RecipeFileName), []byte("version: v1\nname: x\n"), 0o644))
	plain := t.TempDir()
	parent := filepath.Dir(plain)

	for _, tt := range []struct {
		name string
		args []string
		dir  string
		want string
	}{
		{name: "none", args: nil, want: ""},
		{name: "a folder with a recipe", args: []string{withRecipe}, want: withRecipe},
		{name: "a folder with none", args: []string{plain}, want: plain},
		{name: "the recipe itself", args: []string{filepath.Join(withRecipe, project.RecipeFileName)}, want: filepath.Join(withRecipe, project.RecipeFileName)},
		{name: "relative to the launch", args: []string{filepath.Base(plain)}, dir: parent, want: plain},
		{name: "flags are skipped", args: []string{"-psn_0_12345", "--verbose", plain}, want: plain},
		{name: "a path that does not exist", args: []string{filepath.Join(plain, "gone")}, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, LaunchPath(tt.args, tt.dir))
		})
	}
}

// TestSingleInstanceID names the installation: the bundle identifier and a
// short digest of the executable's path.
func TestSingleInstanceID(t *testing.T) {
	id := SingleInstanceID()
	assert.True(t, strings.HasPrefix(id, bundleID+".x"), id)
	assert.Equal(t, id, SingleInstanceID(), "one executable, one id")

	// On Linux the id is a D-Bus name, and Wails refuses one whose element
	// is empty, starts with a digit or holds anything but letters, digits,
	// underscores and hyphens. Every path below, among them the ones whose
	// digest starts with a digit, gives a name it accepts, and no two share
	// one.
	seen := map[string]string{}
	digitFirst := 0
	defer func() {
		assert.Positive(t, digitFirst, "some path's digest starts with a digit, the case the leading letter is for")
	}()
	for _, exe := range []string{
		"/opt/Kapi/Kapi", "/usr/local/bin/Kapi", "/usr/bin/Kapi",
		"/srv/kapi/bin/Kapi", `C:\Program Files\Kapi\Kapi.exe`,
		"/Applications/Kapi.app/Contents/MacOS/Kapi", "bin/KapiDesktop",
	} {
		id := singleInstanceIDFor(exe)
		if digest := strings.TrimPrefix(id, bundleID+".x"); digest[0] >= '0' && digest[0] <= '9' {
			digitFirst++
		}
		require.NoError(t, dbusNameError(id), "%s gives %s", exe, id)
		assert.NotContains(t, seen, id, "%s and %s share an id", exe, seen[id])
		seen[id] = exe
	}
}

// dbusNameError applies the element rules Wails v3 checks a Linux
// single-instance id against before it claims the name on the session bus.
func dbusNameError(id string) error {
	for element := range strings.SplitSeq(id, ".") {
		if element == "" {
			return errors.New("empty element")
		}
		if element[0] >= '0' && element[0] <= '9' {
			return fmt.Errorf("element %q starts with a digit", element)
		}
		for _, r := range element {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
				continue
			}
			return fmt.Errorf("element %q holds %q", element, r)
		}
	}
	return nil
}

// TestSingleInstanceAvailable: macOS keeps one instance itself, Linux takes
// the lock only with a session bus, Windows always takes it.
func TestSingleInstanceAvailable(t *testing.T) {
	bus := func(k string) string {
		if k == "DBUS_SESSION_BUS_ADDRESS" {
			return "unix:path=/run/user/1000/bus"
		}
		return ""
	}
	none := func(string) string { return "" }
	assert.False(t, SingleInstanceAvailable("darwin", bus))
	assert.True(t, SingleInstanceAvailable("linux", bus))
	assert.False(t, SingleInstanceAvailable("linux", none), "a Linux session with no bus starts without the lock")
	assert.True(t, SingleInstanceAvailable("windows", none))
}

// TestOutsideOpener: a path that arrives before the window can open it
// waits, and opens, in order, once it can; later paths open at once.
func TestOutsideOpener(t *testing.T) {
	var o OutsideOpener
	var opened []string
	o.Open("/early/one")
	o.Open("/early/two")
	assert.Empty(t, opened)
	o.SetOpen(func(path string) { opened = append(opened, path) })
	assert.Equal(t, []string{"/early/one", "/early/two"}, opened)
	o.Open("/later")
	assert.Equal(t, []string{"/early/one", "/early/two", "/later"}, opened)
}

// TestShareWritesWhatTheShareSectionWrites: `kapi share --via folder` and the
// desktop's Share section write the same `context:` block for the same folder.
//
// The desktop's spec is what ShareSetupDialog.shareSpec sends for each place
// with only its required fields filled: a folder's path, nothing for git, an S3
// bucket and prefix. The folder share runs whole, sync included, and the two
// recipes' blocks are compared as written. Git and S3 compare the block the
// share would write (host.ShareBlock), since a sync through them needs a
// remote this test does not have.
func TestShareWritesWhatTheShareSectionWrites(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "team-share")
	for _, tt := range []struct {
		name     string
		desktop  project.ContextBackend
		via      string
		location string
		repo     bool
		whole    bool
	}{
		{name: "folder", desktop: project.ContextBackend{Backend: project.ContextBackendFile, Path: shared},
			via: host.ShareViaFolder, location: shared, whole: true},
		{name: "git", desktop: project.ContextBackend{Backend: project.ContextBackendGit},
			via: host.ShareViaGit, repo: true},
		{name: "s3", desktop: project.ContextBackend{Backend: project.ContextBackendS3, Bucket: "acme-kapi", Prefix: "site/"},
			via: host.ShareViaS3, location: "s3://acme-kapi/site/"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := newWorkspaceApp(t)
			tab, desktopRecipe := openSharingProject(t, app, "Desktop")
			cliDir, cliRecipe := scaffoldNamedProject(t, filepath.Join(t.TempDir(), "cli"), project.NewID(), "Desktop")
			if tt.repo {
				if _, err := exec.LookPath("git"); err != nil {
					t.Skip("git is not installed")
				}
				out, err := exec.Command("git", "init", "--quiet", cliDir).CombinedOutput()
				require.NoError(t, err, "%s", out)
			}

			_, err := app.SetProjectSharing(tab, tt.desktop)
			require.NoError(t, err)
			fromDesktop, err := project.Load(desktopRecipe)
			require.NoError(t, err)

			req := host.ShareRequest{Project: cliRecipe, Via: tt.via, Location: tt.location}
			if !tt.whole {
				block, err := app.hostEngine().ShareBlock(t.Context(), req)
				require.NoError(t, err)
				assert.Equal(t, fromDesktop.Context, &block)
				return
			}
			_, err = app.hostEngine().Share(t.Context(), req)
			require.NoError(t, err)
			fromCLI, err := project.Load(cliRecipe)
			require.NoError(t, err)
			assert.Equal(t, fromDesktop.Context, fromCLI.Context)

			block := func(path string) []byte {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				i := bytes.Index(data, []byte("context:"))
				require.GreaterOrEqual(t, i, 0, "%s names a place:\n%s", path, data)
				return data[i:]
			}
			assert.Equal(t, string(block(desktopRecipe)), string(block(cliRecipe)), "the same block, spelled the same way")
		})
	}
}
