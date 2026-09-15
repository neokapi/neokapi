package host_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The missing-plugin offer under KAPI_PLUGINS_DIR_ONLY installs into the
// directory discovery reads, and is not made when that directory is empty.

func TestResolveMissingPluginCommand_OnlyEnvDir_InstallsIntoPluginsDir(t *testing.T) {
	app := &host.App{AssumeYes: true}
	cmd := newMissingPluginCmd(t)
	pluginsDir := t.TempDir()
	t.Setenv("KAPI_PLUGINS_DIR", pluginsDir)

	gotTarget := ""
	handled, err := app.ResolveMissingPluginCommand(cmd, "push", nil, host.MissingPluginOptions{
		Out:          &bytes.Buffer{},
		IsTTYFn:      func() bool { return false },
		RecipeExtras: venueRecipe(),
		InstallFn: func(_ context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
			gotTarget = opts.TargetDir
			return &pluginhost.InstallResult{PluginName: opts.PluginName, Version: "1.2.0", InstallDir: filepath.Join(opts.TargetDir, opts.PluginName)}, nil
		},
		DispatchFn: func(context.Context, string, []string) error { return nil },
	})

	require.True(t, handled)
	require.NoError(t, err)
	assert.Equal(t, pluginsDir, gotTarget, "an isolated install writes where discovery reads")
}

func TestResolveMissingPluginCommand_OnlyEnvDir_EmptyPluginsDirRefuses(t *testing.T) {
	app := &host.App{}
	cmd := newMissingPluginCmd(t)
	t.Setenv("KAPI_PLUGINS_DIR", "")
	var out bytes.Buffer

	handled, err := app.ResolveMissingPluginCommand(cmd, "push", nil, host.MissingPluginOptions{
		Out:          &out,
		In:           strings.NewReader("y\n"),
		IsTTYFn:      func() bool { return true },
		RecipeExtras: venueRecipe(),
		InstallFn: func(context.Context, pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
			t.Fatal("must not install into a directory discovery never reads")
			return nil, nil
		},
		DispatchFn: func(context.Context, string, []string) error {
			t.Fatal("must not run the verb")
			return nil
		},
	})

	require.True(t, handled)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "KAPI_PLUGINS_DIR is empty")
	assert.NotContains(t, out.String(), "[Y/n]", "an install that would be refused is not offered")
}
