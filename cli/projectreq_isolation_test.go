package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/project/projecttest"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recipe's auto-install under KAPI_PLUGINS_DIR_ONLY writes to the directory
// discovery reads, and is refused when that directory is empty.

func TestLoadProjectInteractive_OnlyEnvDir_AutoInstallsIntoPluginsDir(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	pluginsDir := t.TempDir()
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", pluginsDir)

	gotTarget := ""
	stubInstall := func(_ context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
		gotTarget = opts.TargetDir
		project.RegisterExtension(project.Extension{Name: "stub", Scope: project.ScopeProject, Group: opts.PluginName})
		return &pluginhost.InstallResult{PluginName: opts.PluginName, Version: "1.0.0", InstallDir: filepath.Join(opts.TargetDir, opts.PluginName)}, nil
	}

	app := &App{Quiet: true}
	_, err := app.LoadProjectInteractive(context.Background(), writeRecipe(t, requiresRecipe), LoadProjectInteractiveOptions{
		IsTTYFn:   func() bool { return false },
		AssumeYes: true,
		Out:       discardWriter{},
		InstallFn: stubInstall,
	})
	require.NoError(t, err)
	assert.Equal(t, pluginsDir, gotTarget, "an isolated auto-install writes where discovery reads")
}

func TestLoadProjectInteractive_OnlyEnvDir_EmptyPluginsDirRefusesAutoInstall(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")

	app := &App{Quiet: true}
	_, err := app.LoadProjectInteractive(context.Background(), writeRecipe(t, requiresRecipe), LoadProjectInteractiveOptions{
		IsTTYFn:   func() bool { return false },
		AssumeYes: true,
		Out:       discardWriter{},
		InstallFn: func(context.Context, pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
			t.Fatal("must not install into a directory discovery never reads")
			return nil, nil
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "KAPI_PLUGINS_DIR is empty")
}
