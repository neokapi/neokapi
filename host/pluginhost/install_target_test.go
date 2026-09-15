package pluginhost_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallTargetFor(t *testing.T) {
	sep := string(os.PathListSeparator)
	data := t.TempDir()
	dataTarget := filepath.Join(data, "kapi", "plugins")

	for _, tc := range []struct {
		name       string
		only       string
		pluginsDir string
		want       string
		wantErr    error
	}{
		{name: "default is the data dir", want: dataTarget},
		{name: "plugins dir on its own leaves the data dir", pluginsDir: "/p/one", want: dataTarget},
		{name: "only: the first entry", only: "1", pluginsDir: "/p/one" + sep + "/p/two", want: filepath.Clean("/p/one")},
		{name: "only: empty entries are skipped as discovery skips them", only: "1", pluginsDir: sep + "/p/one", want: filepath.Clean("/p/one")},
		{name: "only: any non-empty value counts", only: "0", pluginsDir: "/p/one", want: filepath.Clean("/p/one")},
		{name: "only: an empty plugins dir is refused", only: "1", wantErr: pluginhost.ErrEmptyPluginsDir},
		{name: "only: separators alone are refused", only: "1", pluginsDir: sep + sep, wantErr: pluginhost.ErrEmptyPluginsDir},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", data)
			t.Setenv("KAPI_PLUGINS_DIR_ONLY", tc.only)

			got, err := pluginhost.InstallTargetFor(tc.pluginsDir)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestInstallTarget_ReadsPluginsDirFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", dir)

	got, err := pluginhost.InstallTarget()
	require.NoError(t, err)
	assert.Equal(t, dir, got)
}

// Under KAPI_PLUGINS_DIR_ONLY the install target is the one root discovery
// scans, for every value that yields a target.
func TestInstallTarget_IsTheOnlyDiscoveryRoot(t *testing.T) {
	sep := string(os.PathListSeparator)
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")

	for _, pluginsDir := range []string{"/p/one", "/p/one" + sep + "/p/two", sep + "/p/one" + sep} {
		target, err := pluginhost.InstallTargetFor(pluginsDir)
		require.NoError(t, err)
		roots := pluginhost.Roots(pluginhost.DiscoverOptions{EnvPluginsDir: pluginsDir})
		require.NotEmpty(t, roots)
		assert.Equal(t, roots[0].Path, target, "pluginsDir %q", pluginsDir)
		for _, r := range roots {
			assert.Equal(t, 1, r.Order, "KAPI_PLUGINS_DIR_ONLY scans $KAPI_PLUGINS_DIR entries only")
		}
	}
}
