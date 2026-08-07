package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/project/projecttest"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const requiresRecipe = `version: v2
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

// isolatePluginRoots cuts these tests off from the developer's real plugin
// roots. LoadProjectInteractive re-discovers plugins after an install and
// registers their schema extensions, so without this a bowrain plugin actually
// installed on the machine — via Homebrew, say — satisfies `requires: bowrain`
// and the tests assert against the wrong world. Same isolation switch the
// in-repo dogfood contract uses.
func isolatePluginRoots(t *testing.T) {
	t.Helper()
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
}

func TestLoadProjectInteractive_NoRequires_Loads(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)

	path := writeRecipe(t, "version: v2\n")
	app := &App{}
	proj, err := app.LoadProjectInteractive(context.Background(), path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
	})
	require.NoError(t, err)
	assert.Equal(t, project.CurrentVersion, proj.Version)
}

func TestLoadProjectInteractive_NonTTY_NoYes_ReturnsActionableError(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)

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
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)

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
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)

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
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)

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
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)

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
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)

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
			got, err := Confirm(strings.NewReader(tc.in), discardWriter{}, "ok? ")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestConfirmDefaultNo_RequiresAnExplicitYes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"y", "y\n", true},
		{"Y", "Y\n", true},
		{"yes", "yes\n", true},
		{"YES", "YES\n", true},
		// The cases that separate it from Confirm: nothing typed, and a
		// closed stdin, are both no.
		{"empty", "\n", false},
		{"eof", "", false},
		{"n", "n\n", false},
		{"garbage", "abc\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConfirmDefaultNo(strings.NewReader(tc.in), discardWriter{}, "ok? ")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// execRecipe names a step that runs a command, which is what arms the trust
// gate. Nothing in this repository's own recipes does.
const execRecipe = `version: v2
flows:
  default:
    steps:
      - tool: external-command
        config:
          command: /bin/echo
          args: ["hi"]
`

// isolateTrustRecord points the execution-trust record at a throwaway config
// dir. Without it these tests would read — and write — the developer's real
// decisions, which is the same isolation the in-repo dogfood contract demands
// of any kapi invocation.
func isolateTrustRecord(t *testing.T) {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_TRUST_EXEC", "")
}

func loadExecRecipe(t *testing.T, app *App, path string, opts LoadProjectInteractiveOptions) (*project.KapiProject, error) {
	t.Helper()
	return app.LoadProjectInteractive(context.Background(), path, opts)
}

// TestLoadProjectInteractive_ExecStep_NonTTY_Refuses: discovery binds a recipe
// by walking up from the working directory, so an unattended run can meet a
// recipe nobody chose. With no terminal there is nobody to ask, and the answer
// to an unanswerable question is no.
func TestLoadProjectInteractive_ExecStep_NonTTY_Refuses(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)

	path := writeRecipe(t, execRecipe)
	_, err := loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
		Out:     discardWriter{},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, host.ErrExecNotTrusted)
	assert.Contains(t, err.Error(), "KAPI_TRUST_EXEC")
}

// TestLoadProjectInteractive_ExecStep_AssumeYesDoesNotGrant is the decision
// this gate turns on. --yes already rides on every unattended script in order
// to auto-install plugins; if it also granted execution trust, shipping the
// gate would have granted the thing it exists to gate, to exactly the
// population that runs untrusted checkouts.
func TestLoadProjectInteractive_ExecStep_AssumeYesDoesNotGrant(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)

	path := writeRecipe(t, execRecipe)
	_, err := loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn:   func() bool { return false },
		AssumeYes: true,
		Out:       discardWriter{},
	})
	require.Error(t, err, "--yes must not grant the right to run commands")
	require.ErrorIs(t, err, host.ErrExecNotTrusted)
}

// TestLoadProjectInteractive_ExecStep_EnvGrantsForCI: the opt-in a pipeline
// author sets deliberately, named for what it grants.
func TestLoadProjectInteractive_ExecStep_EnvGrantsForCI(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)
	t.Setenv("KAPI_TRUST_EXEC", "1")

	path := writeRecipe(t, execRecipe)
	proj, err := loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
		Out:     discardWriter{},
	})
	require.NoError(t, err)
	require.NotNil(t, proj)
}

// TestLoadProjectInteractive_ExecStep_EnvNeedsAnAffirmativeValue: a switch that
// grants code execution reads 0 as no, rather than as "set, therefore yes".
func TestLoadProjectInteractive_ExecStep_EnvNeedsAnAffirmativeValue(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)
	t.Setenv("KAPI_TRUST_EXEC", "0")

	path := writeRecipe(t, execRecipe)
	_, err := loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
		Out:     discardWriter{},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, host.ErrExecNotTrusted)
}

// TestLoadProjectInteractive_ExecStep_TTY_DeclineRefuses: an empty answer is a
// decline, so a distracted Return does not grant execution.
func TestLoadProjectInteractive_ExecStep_TTY_DeclineRefuses(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)

	for _, answer := range []string{"n\n", "\n"} {
		path := writeRecipe(t, execRecipe)
		_, err := loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
			IsTTYFn: func() bool { return true },
			In:      strings.NewReader(answer),
			Out:     discardWriter{},
		})
		require.Error(t, err, "answer %q", answer)
		require.ErrorIs(t, err, host.ErrExecNotTrusted)
	}
}

// TestLoadProjectInteractive_ExecStep_TTY_PromptShowsTheCommand: the prompt is
// only meaningful if it says what would run.
func TestLoadProjectInteractive_ExecStep_TTY_PromptShowsTheCommand(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)

	path := writeRecipe(t, execRecipe)
	var out strings.Builder
	_, err := loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return true },
		In:      strings.NewReader("n\n"),
		Out:     &out,
	})
	require.Error(t, err)
	assert.Contains(t, out.String(), "external-command")
	assert.Contains(t, out.String(), "/bin/echo")
	assert.Contains(t, out.String(), path)
}

// TestLoadProjectInteractive_ExecStep_DecisionIsSticky: the answer is asked
// once and remembered, including a decline — re-asking a question already
// answered no is how a person is trained to answer yes.
func TestLoadProjectInteractive_ExecStep_DecisionIsSticky(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)

	path := writeRecipe(t, execRecipe)
	opts := func(in string) LoadProjectInteractiveOptions {
		return LoadProjectInteractiveOptions{
			IsTTYFn: func() bool { return true },
			In:      strings.NewReader(in),
			Out:     discardWriter{},
		}
	}

	// Approve once.
	_, err := loadExecRecipe(t, &App{Quiet: true}, path, opts("y\n"))
	require.NoError(t, err)

	// A later run does not ask again: stdin holds a decline, and the load
	// still succeeds because the recorded answer is used instead.
	_, err = loadExecRecipe(t, &App{Quiet: true}, path, opts("n\n"))
	require.NoError(t, err, "an approved project must not ask again")

	// It also holds with no terminal at all, which is what makes a CI run
	// after a local approval work.
	_, err = loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
		Out:     discardWriter{},
	})
	require.NoError(t, err)
}

// TestLoadProjectInteractive_ExecStep_DeclineIsRemembered: a declined project
// stays declined, and says how to change the answer.
func TestLoadProjectInteractive_ExecStep_DeclineIsRemembered(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)

	path := writeRecipe(t, execRecipe)
	_, err := loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return true },
		In:      strings.NewReader("n\n"),
		Out:     discardWriter{},
	})
	require.Error(t, err)

	_, err = loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return true },
		In:      strings.NewReader("y\n"),
		Out:     discardWriter{},
	})
	require.Error(t, err, "a declined project is not re-asked into an approval")
	assert.Contains(t, err.Error(), host.ExecTrustPath())
}

// TestLoadProjectInteractive_ExecStep_EditingTheCommandReAsks is what makes the
// record safe to keep: approval attaches to the argv that was shown, not to the
// project forever. A recipe edited to run something else is a new question.
func TestLoadProjectInteractive_ExecStep_EditingTheCommandReAsks(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)

	path := writeRecipe(t, execRecipe)
	_, err := loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return true },
		In:      strings.NewReader("y\n"),
		Out:     discardWriter{},
	})
	require.NoError(t, err)

	// An edit that does not change what runs keeps the approval.
	require.NoError(t, os.WriteFile(path, []byte(execRecipe+`defaults:
  target_languages: [nb]
`), 0o644))
	_, err = loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
		Out:     discardWriter{},
	})
	require.NoError(t, err, "an unrelated edit must not re-ask")

	// Changing the command does not.
	require.NoError(t, os.WriteFile(path,
		[]byte(strings.Replace(execRecipe, "/bin/echo", "/bin/sh", 1)), 0o644))
	_, err = loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
		Out:     discardWriter{},
	})
	require.Error(t, err, "a changed command must re-ask")
	require.ErrorIs(t, err, host.ErrExecNotTrusted)
}

// TestLoadProjectInteractive_ExecStep_TrustIsPerProject: approving one checkout
// does not approve another copy of it. Path is part of the record's key, so a
// second clone is a second decision.
func TestLoadProjectInteractive_ExecStep_TrustIsPerProject(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)

	approved := writeRecipe(t, execRecipe)
	_, err := loadExecRecipe(t, &App{Quiet: true}, approved, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return true },
		In:      strings.NewReader("y\n"),
		Out:     discardWriter{},
	})
	require.NoError(t, err)

	other := writeRecipe(t, execRecipe)
	_, err = loadExecRecipe(t, &App{Quiet: true}, other, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
		Out:     discardWriter{},
	})
	require.Error(t, err, "an identical recipe elsewhere is a separate decision")
}

// TestLoadProjectInteractive_OrdinaryRecipe_NeverPrompts: the gate must be
// invisible to every recipe that runs no commands — which is all of them, in
// this repository and in normal use.
func TestLoadProjectInteractive_OrdinaryRecipe_NeverPrompts(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	isolatePluginRoots(t)
	isolateTrustRecord(t)

	path := writeRecipe(t, `version: v2
defaults:
  source_language: en
  target_languages: [nb]
flows:
  default:
    steps:
      - tool: translate
`)
	var out strings.Builder
	_, err := loadExecRecipe(t, &App{Quiet: true}, path, LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
		Out:     &out,
	})
	require.NoError(t, err)
	assert.Empty(t, out.String())
	// Nothing was decided, so nothing was written.
	_, statErr := os.Stat(host.ExecTrustPath())
	assert.True(t, os.IsNotExist(statErr), "an ordinary recipe writes no trust record")
}

// discardWriter is a minimal io.Writer that drops everything — used
// instead of io.Discard to avoid pulling extra packages into the test
// imports.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
