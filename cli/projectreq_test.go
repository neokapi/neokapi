package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const requiresRecipe = `version: v1
requires:
  bowrain: "^1.0"
`

func writeRecipe(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadProjectInteractive_NoRequires_Loads(t *testing.T) {
	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	path := writeRecipe(t, "version: v1\n")
	app := &App{}
	proj, err := app.LoadProjectInteractive(context.Background(), path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
	})
	require.NoError(t, err)
	assert.Equal(t, project.CurrentVersion, proj.Version)
}

func TestLoadProjectInteractive_NonTTY_NoYes_ReturnsActionableError(t *testing.T) {
	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	path := writeRecipe(t, requiresRecipe)
	app := &App{}
	_, err := app.LoadProjectInteractive(context.Background(), path, LoadProjectInteractiveOptions{
		IsTTYFn:   func() bool { return false },
		AssumeYes: false,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `recipe requires plugin "bowrain"`)
	assert.Contains(t, err.Error(), "kapi plugin install bowrain")
}

func TestLoadProjectInteractive_TTY_Confirm_InstallsAndRevalidates(t *testing.T) {
	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	path := writeRecipe(t, requiresRecipe)

	installCalls := 0
	stubInstall := func(_ context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
		installCalls++
		// Simulate the side-effect of installing: register the
		// extension group so post-install ValidateRequires passes.
		project.RegisterExtension(project.Extension{
			Name:  "stub",
			Scope: project.ScopeProject,
			Group: opts.PluginName,
		})
		return &pluginhost.InstallResult{
			PluginName: opts.PluginName,
			Version:    "1.0.0",
			InstallDir: t.TempDir(),
			Manifest: &manifest.Manifest{
				Plugin:  opts.PluginName,
				Version: "1.0.0",
			},
		}, nil
	}

	in := strings.NewReader("y\n")
	app := &App{Quiet: true}
	proj, err := app.LoadProjectInteractive(context.Background(), path, LoadProjectInteractiveOptions{
		IsTTYFn:   func() bool { return true },
		In:        in,
		Out:       discardWriter{},
		InstallFn: stubInstall,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, installCalls)
	assert.Equal(t, "^1.0", proj.Requires["bowrain"])
}

func TestLoadProjectInteractive_TTY_DeclineConfirm_ReturnsActionableError(t *testing.T) {
	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	path := writeRecipe(t, requiresRecipe)

	installCalls := 0
	stubInstall := func(_ context.Context, _ pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
		installCalls++
		return nil, nil
	}

	in := strings.NewReader("n\n")
	app := &App{Quiet: true}
	_, err := app.LoadProjectInteractive(context.Background(), path, LoadProjectInteractiveOptions{
		IsTTYFn:   func() bool { return true },
		In:        in,
		Out:       discardWriter{},
		InstallFn: stubInstall,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `recipe requires plugin "bowrain"`)
	assert.Equal(t, 0, installCalls, "decline should skip install entirely")
}

func TestLoadProjectInteractive_AssumeYes_NonTTY_Installs(t *testing.T) {
	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	path := writeRecipe(t, requiresRecipe)

	installCalls := 0
	stubInstall := func(_ context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
		installCalls++
		project.RegisterExtension(project.Extension{
			Name:  "stub",
			Scope: project.ScopeProject,
			Group: opts.PluginName,
		})
		return &pluginhost.InstallResult{
			PluginName: opts.PluginName,
			Version:    "1.0.0",
			InstallDir: t.TempDir(),
			Manifest:   &manifest.Manifest{Plugin: opts.PluginName, Version: "1.0.0"},
		}, nil
	}

	app := &App{Quiet: true}
	proj, err := app.LoadProjectInteractive(context.Background(), path, LoadProjectInteractiveOptions{
		IsTTYFn:   func() bool { return false },
		AssumeYes: true,
		Out:       discardWriter{},
		InstallFn: stubInstall,
	})
	require.NoError(t, err, "--yes should bypass the TTY check and install")
	assert.Equal(t, 1, installCalls)
	require.NotNil(t, proj)
}

func TestLoadProjectInteractive_InstallFails_PropagatesError(t *testing.T) {
	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	path := writeRecipe(t, requiresRecipe)

	installErr := errors.New("registry unreachable")
	stubInstall := func(_ context.Context, _ pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
		return nil, installErr
	}

	app := &App{Quiet: true}
	_, err := app.LoadProjectInteractive(context.Background(), path, LoadProjectInteractiveOptions{
		IsTTYFn:   func() bool { return false },
		AssumeYes: true,
		Out:       discardWriter{},
		InstallFn: stubInstall,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auto-install bowrain")
	assert.Contains(t, err.Error(), "registry unreachable")
}

func TestLoadProjectInteractive_PostInstallValidateRequires_FailsWhenStillMissing(t *testing.T) {
	project.ResetExtensionsForTest()
	defer project.ResetExtensionsForTest()

	path := writeRecipe(t, requiresRecipe)

	// Install succeeds but does not register the extension group, so
	// the second pass through ValidateRequires fails.
	stubInstall := func(_ context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
		return &pluginhost.InstallResult{
			PluginName: opts.PluginName,
			Version:    "1.0.0",
			InstallDir: t.TempDir(),
			Manifest:   &manifest.Manifest{Plugin: opts.PluginName, Version: "1.0.0"},
		}, nil
	}

	app := &App{Quiet: true}
	_, err := app.LoadProjectInteractive(context.Background(), path, LoadProjectInteractiveOptions{
		IsTTYFn:   func() bool { return false },
		AssumeYes: true,
		Out:       discardWriter{},
		InstallFn: stubInstall,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `recipe requires plugin "bowrain"`)
}

func TestConfirm_AcceptsYesAndEmpty(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "\n", true},
		{"y", "y\n", true},
		{"Y", "Y\n", true},
		{"yes", "yes\n", true},
		{"YES", "YES\n", true},
		{"n", "n\n", false},
		{"no", "no\n", false},
		{"garbage", "abc\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := confirm(strings.NewReader(tc.in), discardWriter{}, "ok? ")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// discardWriter is a minimal io.Writer that drops everything — used
// instead of io.Discard to avoid pulling extra packages into the test
// imports.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
