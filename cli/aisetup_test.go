package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/config"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// fakeSetupIO builds a scripted AISetupIO: stdin lines, captured output, a
// canned detection, a recording live check, and an in-memory config sink.
func fakeSetupIO(stdin string, det AIDetection) (AISetupIO, *bytes.Buffer, map[string]string, *[]string) {
	out := &bytes.Buffer{}
	saved := map[string]string{}
	var checks []string
	io := AISetupIO{
		In:     strings.NewReader(stdin),
		Out:    out,
		IsTTY:  func() bool { return true },
		Detect: func(context.Context) AIDetection { return det },
		LiveCheck: func(_ context.Context, provider, model, _ string) error {
			checks = append(checks, provider+"/"+model)
			return nil
		},
		SetConfig: func(key, value string) error { saved[key] = value; return nil },
	}
	return io, out, saved, &checks
}

func TestAISetupWizardSelectsDetectedClaudeCode(t *testing.T) {
	// Empty selection accepts the default (1 = Claude Code); "y" runs the check.
	io, out, saved, checks := fakeSetupIO("\ny\n", AIDetection{ClaudeCode: true})
	a := &App{Config: config.NewAppConfig()}

	provider, err := a.RunAISetupWizard(context.Background(), io, false)
	require.NoError(t, err)
	assert.Equal(t, "claude-code", provider)
	assert.Equal(t, "claude-code", saved[config.KeyAIProvider])
	assert.Equal(t, aiprovider.DefaultClaudeCodeModel, saved[config.KeyAIModel])
	assert.Equal(t, []string{"claude-code/sonnet"}, *checks)

	text := out.String()
	assert.Contains(t, text, "✓ Claude Code detected — uses your Claude subscription")
	assert.Contains(t, text, "✓ kapi will translate with Claude Code (sonnet). Try: kapi up")

	// The in-memory config mirrors the choice so the current run continues.
	assert.Equal(t, "claude-code", a.Config.GetString(config.KeyAIProvider))
}

func TestAISetupWizardSkipsLiveCheck(t *testing.T) {
	io, _, saved, checks := fakeSetupIO("1\nn\n", AIDetection{ClaudeCode: true})
	a := &App{Config: config.NewAppConfig()}

	_, err := a.RunAISetupWizard(context.Background(), io, false)
	require.NoError(t, err)
	assert.Empty(t, *checks, "answering n must skip the live check")
	assert.Equal(t, "claude-code", saved[config.KeyAIProvider])
}

func TestAISetupWizardDemoNeedsNoCheck(t *testing.T) {
	det := AIDetection{} // nothing detected
	io, out, saved, checks := fakeSetupIO("6\n", det)
	a := &App{Config: config.NewAppConfig()}

	// With nothing detected the order is anthropic, openai, gemini, ollama
	// (not running), demo → demo is 5.
	choices := BuildAISetupChoices(det)
	demoIdx := 0
	for i, c := range choices {
		if c.Provider == "demo" {
			demoIdx = i + 1
		}
	}
	io.In = strings.NewReader(strings.Repeat(" ", 0) + fmtInt(demoIdx) + "\n")

	provider, err := a.RunAISetupWizard(context.Background(), io, false)
	require.NoError(t, err)
	assert.Equal(t, "demo", provider)
	assert.Equal(t, "demo", saved[config.KeyAIProvider])
	assert.Empty(t, *checks, "demo engine runs no live check")
	assert.NotContains(t, out.String(), "test call")
}

func fmtInt(i int) string { return string(rune('0' + i)) }

// newTestUpLikeCmd builds a minimal command shaped like `kapi up` for the
// ensure-provider hook (context set, no AI flags unless a test adds them).
func newTestUpLikeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "up", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SetContext(context.Background())
	return cmd
}

// writeExecutable creates an empty executable file (a stand-in binary for
// PATH/LookPath detection).
func writeExecutable(path string) error {
	return os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755)
}

func TestAISetupWizardNonTTY(t *testing.T) {
	io, _, _, _ := fakeSetupIO("", AIDetection{})
	io.IsTTY = func() bool { return false }
	a := &App{}

	_, err := a.RunAISetupWizard(context.Background(), io, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a terminal")
	assert.Contains(t, err.Error(), "ANTHROPIC_API_KEY")
}

func TestAISetupWizardLiveCheckFailureSurfaces(t *testing.T) {
	io, _, saved, _ := fakeSetupIO("1\ny\n", AIDetection{ClaudeCode: true})
	io.LiveCheck = func(context.Context, string, string, string) error {
		return &aiprovider.ClaudeCodeError{Kind: aiprovider.ClaudeCodeErrAuth}
	}
	a := &App{Config: config.NewAppConfig()}

	_, err := a.RunAISetupWizard(context.Background(), io, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Claude Code isn't signed in")
	assert.Empty(t, saved, "a failed live check must not persist the choice")
}

func TestBuildAISetupChoicesOrdering(t *testing.T) {
	det := AIDetection{ClaudeCode: true, Ollama: true, EnvKeyProviders: []string{"openai"}}
	choices := BuildAISetupChoices(det)
	require.NotEmpty(t, choices)
	assert.Equal(t, "claude-code", choices[0].Provider, "detected Claude Code ranks first")
	assert.Equal(t, "openai", choices[1].Provider, "env-key provider next")
	assert.Equal(t, "ollama", choices[2].Provider)
	assert.Equal(t, "demo", choices[len(choices)-1].Provider, "demo is always last")

	// No duplicates when a provider is both env-keyed and key-entry.
	seen := map[string]int{}
	for _, c := range choices {
		seen[c.Provider]++
	}
	for p, n := range seen {
		assert.Equal(t, 1, n, "provider %s listed once", p)
	}
}

func TestDetectAIOptions(t *testing.T) {
	// Claude Code: point the binary env at a real file.
	dir := t.TempDir()
	shim := filepath.Join(dir, "claude")
	require.NoError(t, writeExecutable(shim))
	t.Setenv("KAPI_CLAUDE_CODE_BIN", shim)

	// Ollama: a fake server that answers /api/version.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"0.5.0"}`))
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("AZURE_OPENAI_API_KEY", "")

	a := &App{Config: config.NewAppConfig()}
	a.Config.Set(config.KeyAIProvider, "claude-code")

	det := a.DetectAIOptions(context.Background(), srv.URL)
	assert.True(t, det.ClaudeCode)
	assert.Equal(t, shim, det.ClaudeCodePath)
	assert.True(t, det.Ollama)
	assert.Equal(t, []string{"anthropic"}, det.EnvKeyProviders)
	assert.Equal(t, "claude-code", det.DefaultProvider)
	assert.True(t, det.Configured())
}

func TestDetectAIOptionsNothing(t *testing.T) {
	t.Setenv("KAPI_CLAUDE_CODE_BIN", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("AZURE_OPENAI_API_KEY", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusBadGateway)
	}))
	srv.Close() // closed server → unreachable

	a := &App{Config: config.NewAppConfig()}
	det := a.DetectAIOptions(context.Background(), srv.URL)
	assert.False(t, det.ClaudeCode)
	assert.False(t, det.Ollama)
	assert.Empty(t, det.EnvKeyProviders)
	assert.False(t, det.Configured())
}

func TestEnsureAIProviderInteractive(t *testing.T) {
	t.Run("already configured skips wizard", func(t *testing.T) {
		io, out, _, _ := fakeSetupIO("", AIDetection{DefaultProvider: "ollama"})
		a := &App{AISetupIOOverride: &io, Config: config.NewAppConfig()}
		cmd := newTestUpLikeCmd()
		require.NoError(t, a.EnsureAIProviderInteractive(cmd))
		assert.Empty(t, out.String(), "no prompts when configured")
	})

	t.Run("env key counts as configured", func(t *testing.T) {
		io, out, _, _ := fakeSetupIO("", AIDetection{EnvKeyProviders: []string{"anthropic"}})
		a := &App{AISetupIOOverride: &io, Config: config.NewAppConfig()}
		require.NoError(t, a.EnsureAIProviderInteractive(newTestUpLikeCmd()))
		assert.Empty(t, out.String())
	})

	t.Run("non-TTY unchanged", func(t *testing.T) {
		io, out, _, _ := fakeSetupIO("", AIDetection{})
		io.IsTTY = func() bool { return false }
		a := &App{AISetupIOOverride: &io, Config: config.NewAppConfig()}
		require.NoError(t, a.EnsureAIProviderInteractive(newTestUpLikeCmd()),
			"non-TTY must not error here — downstream keys-only error stays")
		assert.Empty(t, out.String())
	})

	t.Run("unconfigured TTY runs compact wizard", func(t *testing.T) {
		io, out, saved, _ := fakeSetupIO("\nn\n", AIDetection{ClaudeCode: true})
		a := &App{AISetupIOOverride: &io, Config: config.NewAppConfig()}
		require.NoError(t, a.EnsureAIProviderInteractive(newTestUpLikeCmd()))
		assert.Contains(t, out.String(), "No AI provider is configured yet")
		assert.Equal(t, "claude-code", saved[config.KeyAIProvider])
	})

	t.Run("explicit provider flag skips wizard", func(t *testing.T) {
		io, out, _, _ := fakeSetupIO("", AIDetection{})
		a := &App{AISetupIOOverride: &io, Config: config.NewAppConfig()}
		cmd := newTestUpLikeCmd()
		cmd.Flags().String("provider", "", "")
		require.NoError(t, cmd.Flags().Set("provider", "demo"))
		require.NoError(t, a.EnsureAIProviderInteractive(cmd))
		assert.Empty(t, out.String())
	})
}
