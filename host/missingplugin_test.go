package host_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// venueRecipe returns the Extras override standing in for a recipe whose
// top-level `bowrain:` block survived because the bowrain plugin is absent.
func venueRecipe() map[string]any {
	return map[string]any{"bowrain": map[string]any{"url": "https://example.test/acme/demo"}}
}

// newMissingPluginCmd returns a synthetic command and cuts the test off from
// the machine's real plugin roots and caches: the resolver re-discovers plugins
// after an install, so without this a plugin actually installed here (via
// Homebrew, say) would answer for the fixture and the discovery cache in the
// developer's XDG dirs would be rewritten.
func newMissingPluginCmd(t *testing.T) *host.EnvCommand {
	t.Helper()
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cmd := host.NewEnvCommand(context.Background(), "push")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd
}

func TestResolveMissingPluginCommand_NonInteractiveExplainsAndExitsUsage(t *testing.T) {
	app := &host.App{}
	var out bytes.Buffer

	handled, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "push", nil, host.MissingPluginOptions{
		Out:          &out,
		IsTTYFn:      func() bool { return false },
		RecipeExtras: venueRecipe(),
		InstallFn: func(context.Context, pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
			t.Fatal("must not install without a TTY or --yes")
			return nil, nil
		},
	})

	require.True(t, handled, "a bowrain verb in a venue-backed project must be handled")
	require.Error(t, err)
	assert.Equal(t, host.ExitUsage, host.ExitCode(nil, err),
		"a missing plugin is a usage problem, distinct from an operational error")
	assert.Contains(t, err.Error(), "kapi plugin install bowrain")
	assert.Contains(t, err.Error(), "brew install neokapi/tap/bowrain-cli")
	assert.Contains(t, err.Error(), "`bowrain:`", "the message must name the recipe key that implies the plugin")
	assert.Empty(t, out.String(), "non-interactive guidance travels as the error, not as side-channel output")
}

func TestResolveMissingPluginCommand_QuietIsNonInteractiveEvenOnATTY(t *testing.T) {
	app := &host.App{Quiet: true}
	var out bytes.Buffer

	handled, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "push", nil, host.MissingPluginOptions{
		Out:          &out,
		In:           strings.NewReader("y\n"),
		IsTTYFn:      func() bool { return true },
		RecipeExtras: venueRecipe(),
		InstallFn: func(context.Context, pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
			t.Fatal("--quiet must not prompt or install")
			return nil, nil
		},
	})

	require.True(t, handled)
	require.Error(t, err)
	assert.Equal(t, host.ExitUsage, host.ExitCode(nil, err))
	assert.Empty(t, out.String())
}

func TestResolveMissingPluginCommand_PromptAcceptInstallsAndRunsVerb(t *testing.T) {
	app := &host.App{}
	var out bytes.Buffer
	installed := ""
	dispatched := ""
	var dispatchedArgs []string

	handled, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "push", []string{"--force"}, host.MissingPluginOptions{
		Out:          &out,
		In:           strings.NewReader("\n"), // bare Enter accepts the [Y/n] default
		IsTTYFn:      func() bool { return true },
		RecipeExtras: venueRecipe(),
		InstallFn: func(_ context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
			installed = opts.PluginName
			return &pluginhost.InstallResult{PluginName: opts.PluginName, Version: "1.2.0", InstallDir: "/tmp/bowrain"}, nil
		},
		DispatchFn: func(_ context.Context, verb string, args []string) error {
			dispatched, dispatchedArgs = verb, args
			return nil
		},
	})

	require.True(t, handled)
	require.NoError(t, err)
	assert.Equal(t, "bowrain", installed)
	assert.Equal(t, "push", dispatched, "accepting the offer must run the command the user actually typed")
	assert.Equal(t, []string{"--force"}, dispatchedArgs, "the verb's own flags must reach the plugin")
	assert.Contains(t, out.String(), "Install bowrain now? [Y/n]")
	assert.Contains(t, out.String(), "Running `kapi push`")
}

func TestResolveMissingPluginCommand_AssumeYesSkipsPrompt(t *testing.T) {
	app := &host.App{AssumeYes: true}
	var out bytes.Buffer
	dispatched := false

	handled, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "pull", nil, host.MissingPluginOptions{
		Out:          &out,
		IsTTYFn:      func() bool { return false }, // --yes works headless, e.g. in CI
		RecipeExtras: venueRecipe(),
		InstallFn: func(_ context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
			return &pluginhost.InstallResult{PluginName: opts.PluginName, Version: "1.2.0", InstallDir: "/tmp/bowrain"}, nil
		},
		DispatchFn: func(context.Context, string, []string) error { dispatched = true; return nil },
	})

	require.True(t, handled)
	require.NoError(t, err)
	assert.True(t, dispatched)
	assert.NotContains(t, out.String(), "[Y/n]", "--yes must not ask")
}

// --yes and --quiet are still at their defaults on this path: cobra rejects an
// unknown verb inside Find(), before execute() parses any flag. The resolver
// recovers them from argv, wherever the user put them.
func TestResolveMissingPluginCommand_RecoversFlagsFromArgv(t *testing.T) {
	tests := []struct {
		name       string
		argv       []string
		wantPrompt bool
		wantErr    bool
	}{
		{"yes after the verb", []string{"push", "--yes"}, false, false},
		{"short yes after the verb", []string{"push", "-y"}, false, false},
		{"yes before the verb", []string{"--yes", "push"}, false, false},
		{"yes past an unknown flag", []string{"push", "--force", "--yes"}, false, false},
		{"quiet is non-interactive", []string{"push", "--quiet"}, false, true},
		{"neither: prompt", []string{"push"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			app := &host.App{}
			_, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "push", nil, host.MissingPluginOptions{
				Argv:         tt.argv,
				Out:          &out,
				In:           strings.NewReader("y\n"),
				IsTTYFn:      func() bool { return true },
				RecipeExtras: venueRecipe(),
				InstallFn: func(_ context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
					return &pluginhost.InstallResult{PluginName: opts.PluginName, Version: "1.2.0", InstallDir: "/tmp/bowrain"}, nil
				},
				DispatchFn: func(context.Context, string, []string) error { return nil },
			})
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, out.String(), "--quiet must stay silent")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPrompt, strings.Contains(out.String(), "[Y/n]"))
		})
	}
}

func TestResolveMissingPluginCommand_DeclinedPromptExitsUsage(t *testing.T) {
	app := &host.App{}
	var out bytes.Buffer

	handled, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "push", nil, host.MissingPluginOptions{
		Out:          &out,
		In:           strings.NewReader("n\n"),
		IsTTYFn:      func() bool { return true },
		RecipeExtras: venueRecipe(),
		InstallFn: func(context.Context, pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
			t.Fatal("declining must not install")
			return nil, nil
		},
	})

	require.True(t, handled)
	require.Error(t, err)
	assert.Equal(t, host.ExitUsage, host.ExitCode(nil, err))
	assert.Contains(t, err.Error(), "kapi plugin install bowrain")
}

func TestResolveMissingPluginCommand_InstallFailureSurfaces(t *testing.T) {
	app := &host.App{AssumeYes: true}
	var out bytes.Buffer

	handled, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "push", nil, host.MissingPluginOptions{
		Out:          &out,
		IsTTYFn:      func() bool { return false },
		RecipeExtras: venueRecipe(),
		InstallFn: func(context.Context, pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
			return nil, errors.New("registry unreachable")
		},
	})

	require.True(t, handled)
	require.ErrorContains(t, err, "install bowrain: registry unreachable")
}

func TestResolveMissingPluginCommand_UnrelatedVerbIsNotHandled(t *testing.T) {
	app := &host.App{}

	handled, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "psh", nil, host.MissingPluginOptions{
		IsTTYFn:      func() bool { return true },
		RecipeExtras: venueRecipe(),
	})

	assert.False(t, handled, "a typo must keep cobra's error and its did-you-mean suggestions")
	assert.NoError(t, err)
}

func TestResolveMissingPluginCommand_RecipeWithoutVenueBlockIsNotHandled(t *testing.T) {
	app := &host.App{}

	handled, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "push", nil, host.MissingPluginOptions{
		IsTTYFn:      func() bool { return true },
		RecipeExtras: map[string]any{}, // a recipe, but no bowrain: block
	})

	assert.False(t, handled, "without a bowrain: block there is nothing to push to — cobra's error stands")
	assert.NoError(t, err)
}

// A recipe on disk is the real path: kapi resolves it by walking upward from
// the cwd, and a plugin-owned key survives in Extras because the plugin that
// would decode it is absent.
func TestResolveMissingPluginCommand_DetectsVenueBlockFromRecipeOnDisk(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(
		"version: v1\ndefaults:\n  source_language: en-US\n  target_languages: [fr-FR]\nbowrain:\n  url: https://example.test/acme/demo\n"), 0o644))

	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	app := &host.App{}
	handled, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "push", nil, host.MissingPluginOptions{
		Out:     &bytes.Buffer{},
		IsTTYFn: func() bool { return false },
	})

	require.True(t, handled, "the bowrain: block must be detected from the recipe on disk")
	require.Error(t, err)
	assert.Equal(t, host.ExitUsage, host.ExitCode(nil, err))
}

func TestResolveMissingPluginCommand_NoProjectIsNotHandled(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("KAPI_NO_PROJECT", "1")

	app := &host.App{}
	handled, err := app.ResolveMissingPluginCommand(newMissingPluginCmd(t), "push", nil, host.MissingPluginOptions{
		IsTTYFn: func() bool { return true },
	})

	assert.False(t, handled, "outside a project `kapi push` really is an unknown command")
	assert.NoError(t, err)
}

func TestPluginHintForVerb(t *testing.T) {
	h, ok := host.PluginHintForVerb("push")
	require.True(t, ok)
	assert.Equal(t, "bowrain", h.Plugin)
	assert.Equal(t, "bowrain", h.RecipeKey)

	_, ok = host.PluginHintForVerb("extract")
	assert.False(t, ok, "built-in verbs must never resolve to a plugin hint")
}
