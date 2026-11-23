package dataroot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A test that leaves HOME alone must not reach the developer's configuration
// or home: both resolve under this test binary's own data root.
func TestUserDirsStayUnderTheTestRootWhenHomeIsUntouched(t *testing.T) {
	root := testDataDir()

	cfg := UserConfigDir()
	assert.Equal(t, filepath.Join(root, "user", "config"), cfg)

	home, err := UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "user", "home"), home)

	real, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.NotContains(t, cfg, real+string(filepath.Separator)+"Library")
}

// A test that points HOME at a directory of its own resolves through the
// platform under that directory, as a user would.
func TestUserDirsFollowAHomeTheTestChose(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, home, got)

	want, err := os.UserConfigDir()
	require.NoError(t, err)
	assert.Equal(t, want, UserConfigDir())
	assert.True(t, filepath.IsAbs(UserConfigDir()))
	assert.Contains(t, UserConfigDir(), home)
}

// A test that sets only XDG_CONFIG_HOME chose its configuration tree.
func TestUserConfigDirFollowsAnXDGConfigHomeTheTestChose(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	want, err := os.UserConfigDir()
	require.NoError(t, err)
	assert.Equal(t, want, UserConfigDir())
}
