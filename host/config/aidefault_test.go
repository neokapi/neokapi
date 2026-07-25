package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reported bug in one sentence: `kapi models default --provider ollama
// gemma4:e2b` reported success, and the very next `kapi up` said no AI provider
// was configured. These tests hold the write→resolve round trip, since that is
// the contract that was broken — not the writing, and not the reading, but the
// agreement between them.

// TestSetAndResolveRoundTrip is the founder's exact sequence.
func TestSetAndResolveRoundTrip(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(EnvAIProvider, "")
	t.Setenv(EnvAIModel, "")

	require.NoError(t, SetAIDefault(nil, "ollama", "gemma4:e2b"))

	// A *fresh* reader, as a separate `kapi up` process would build.
	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	def := ResolveAIDefault(cfg)
	assert.True(t, def.Configured())
	assert.Equal(t, "ollama", def.Provider)
	assert.Equal(t, "gemma4:e2b", def.Model)
	assert.Equal(t, AIDefaultConfig, def.Source)
	assert.Equal(t, GlobalConfigFilePath(), def.File)
}

// isolateRealConfigHome points the *unoverridden* config path at a throwaway
// HOME: no KAPI_CONFIG_DIR, so GlobalConfigFilePath resolves through
// os.UserConfigDir exactly as it does for a user, and the reader has to find that
// directory on its own. This is the shape the bug needed — KAPI_CONFIG_DIR was on
// the old reader's search path *first*, so any test that set it agreed with the
// writer by accident and could not see the defect.
func isolateRealConfigHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", "")
	t.Setenv("HOME", home)
	// os.UserConfigDir consults XDG_CONFIG_HOME on Linux; clearing it keeps the
	// resolved path derived from HOME on every platform.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv(EnvAIProvider, "")
	t.Setenv(EnvAIModel, "")
	return home
}

// TestSetAndResolveRoundTripInsideAProject is the same round trip from inside a
// project directory — the case that failed, because the recipe's filename
// (kapi.yaml) collided with the app config's and the working directory was
// searched first, so the recipe was loaded *as* the app config.
//
// It deliberately does not set KAPI_CONFIG_DIR: with the override in play the old
// reader looked in the right directory anyway and the shadowing never showed.
// Reverting NewAppConfig fails this test.
func TestSetAndResolveRoundTripInsideAProject(t *testing.T) {
	isolateRealConfigHome(t)

	require.NoError(t, SetAIDefault(nil, "ollama", "gemma4:e2b"))

	proj := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(proj, "kapi.yaml"), []byte(
		"version: v1\nname: shadowing-recipe\ndefaults:\n  source_language: en\n"), 0o644))
	t.Chdir(proj)

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "ollama", ResolveAIDefault(cfg).Provider,
		"a project recipe must not shadow the app config")
	assert.Equal(t, "gemma4:e2b", ResolveAIDefault(cfg).Model)
}

// TestReaderFindsTheWritersDirectory is the other half of the divergence: with no
// KAPI_CONFIG_DIR the writer resolves os.UserConfigDir(), which on macOS is
// ~/Library/Application Support — a directory the old reader never searched (it
// hardcoded $HOME/.config/kapi). The assertion is not "look in path X" but the
// property that matters: whatever GlobalConfigFilePath names, Load must read.
func TestReaderFindsTheWritersDirectory(t *testing.T) {
	isolateRealConfigHome(t)

	require.NoError(t, SetAIDefault(nil, "ollama", "gemma4:e2b"))
	written := GlobalConfigFilePath()
	require.FileExists(t, written, "the writer must have created the file it names")

	// Read from a directory that holds no kapi.yaml, so only the config-path
	// resolution is under test.
	t.Chdir(t.TempDir())
	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	def := ResolveAIDefault(cfg)
	assert.Equal(t, "ollama", def.Provider, "the reader must read the file the writer wrote")
	assert.Equal(t, written, def.File)
}

// TestLegacyConfigLocationStillRead: `$HOME/.config/kapi/kapi.yaml` is what the
// docs name and what the old reader searched. On macOS it is not the pinned path,
// so a hand-written config there must keep working as a lower-precedence layer
// rather than silently stop being read.
func TestLegacyConfigLocationStillRead(t *testing.T) {
	home := isolateRealConfigHome(t)
	t.Chdir(t.TempDir())

	legacy := filepath.Join(home, ".config", "kapi", "kapi.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o755))
	require.NoError(t, os.WriteFile(legacy,
		[]byte("ai:\n    provider: ollama\n    model: gemma4:e2b\n"), 0o644))

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "ollama", ResolveAIDefault(cfg).Provider,
		"the documented legacy location must still be read")
}

// TestPinnedConfigBeatsLegacyLocation: the file kapi writes wins key by key over
// a legacy layer, so `kapi models default` is never overruled by a stale file.
func TestPinnedConfigBeatsLegacyLocation(t *testing.T) {
	home := isolateRealConfigHome(t)
	t.Chdir(t.TempDir())

	legacy := filepath.Join(home, ".config", "kapi", "kapi.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o755))
	require.NoError(t, os.WriteFile(legacy,
		[]byte("ai:\n    provider: anthropic\n    model: claude-sonnet-4-6\n"), 0o644))
	require.NoError(t, SetAIDefault(nil, "ollama", "gemma4:e2b"))

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	def := ResolveAIDefault(cfg)
	if GlobalConfigFilePath() == legacy {
		t.Skip("this platform pins the legacy path itself (Linux/XDG); nothing to layer")
	}
	assert.Equal(t, "ollama", def.Provider, "what kapi writes must win over a legacy layer")
	assert.Equal(t, "gemma4:e2b", def.Model)
}

// TestLegacyMergeGranularityIsTheBlock pins the merge rule as a contract rather
// than an accident: the pinned file wins per top-level block, so a legacy `ai:`
// block is ignored entirely once kapi has written one — not merged leaf by leaf,
// which would leave a provider and a model from two different files.
func TestLegacyMergeGranularityIsTheBlock(t *testing.T) {
	home := isolateRealConfigHome(t)
	t.Chdir(t.TempDir())

	legacy := filepath.Join(home, ".config", "kapi", "kapi.yaml")
	if GlobalConfigFilePath() == legacy {
		t.Skip("this platform pins the legacy path itself (Linux/XDG); nothing to layer")
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o755))
	require.NoError(t, os.WriteFile(legacy,
		[]byte("ai:\n    model: from-legacy\nplugins:\n    directory: /legacy/plugins\n"), 0o644))

	// kapi writes a provider and no model.
	require.NoError(t, SetGlobalConfig(KeyAIProvider, "ollama"))

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	def := ResolveAIDefault(cfg)
	assert.Equal(t, "ollama", def.Provider)
	assert.Empty(t, def.Model,
		"the legacy ai: block is skipped whole, so its model must not attach to the pinned provider")

	// A block the pinned file says nothing about still comes through.
	assert.Equal(t, "/legacy/plugins", cfg.GetString(KeyPluginsDirectory),
		"an untouched legacy block must still be read")
}

// TestProjectRecipeIsNeverAppConfig states the rule directly: a recipe in the
// working directory contributes nothing to app configuration, whatever it says.
func TestProjectRecipeIsNeverAppConfig(t *testing.T) {
	isolateRealConfigHome(t)

	proj := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(proj, "kapi.yaml"), []byte(
		"version: v1\nname: not-app-config\nai:\n    provider: anthropic\n"), 0o644))
	t.Chdir(proj)

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.False(t, ResolveAIDefault(cfg).Any(),
		"a recipe must never be read as app configuration")
}

// TestResolvedDefaultReachesToolConfig closes the loop to the consumer that
// actually matters: the tool-config preprocessor every AI tool goes through. If
// this resolves, `kapi up` / `kapi translate` use the configured provider. It
// runs in an unoverridden config home and from inside a project, so it fails on
// the pre-fix reader.
func TestResolvedDefaultReachesToolConfig(t *testing.T) {
	isolateRealConfigHome(t)
	require.NoError(t, SetAIDefault(nil, "ollama", "gemma4:e2b"))

	proj := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(proj, "kapi.yaml"),
		[]byte("version: v1\nname: p\n"), 0o644))
	t.Chdir(proj)

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	got := ApplyAIToolDefaults(cfg, "ai-translate", []string{"credentials"}, nil)
	assert.Equal(t, "ollama", got["provider"])
	assert.Equal(t, "gemma4:e2b", got["model"])
}

// TestToolConfigPrecedenceUnchanged: an explicit provider always wins, and a
// stored model never attaches to a different, explicitly chosen provider.
func TestToolConfigPrecedenceUnchanged(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(EnvAIProvider, "")
	t.Setenv(EnvAIModel, "")
	require.NoError(t, SetAIDefault(nil, "ollama", "gemma4:e2b"))

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	got := ApplyAIToolDefaults(cfg, "ai-translate", []string{"credentials"},
		map[string]any{"provider": "anthropic"})
	assert.Equal(t, "anthropic", got["provider"])
	assert.NotContains(t, got, "model",
		"a stored model must not attach to an explicitly chosen different provider")
}

// TestEnvOverridesStoredDefault keeps env ahead of the file, and says so.
func TestEnvOverridesStoredDefault(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	require.NoError(t, SetAIDefault(nil, "ollama", "gemma4:e2b"))
	t.Setenv(EnvAIProvider, "anthropic")
	t.Setenv(EnvAIModel, "claude-sonnet-4")

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	def := ResolveAIDefault(cfg)
	assert.Equal(t, "anthropic", def.Provider)
	assert.Equal(t, "claude-sonnet-4", def.Model)
	assert.Equal(t, AIDefaultEnv, def.Source)
	assert.Contains(t, def.Describe(), EnvAIProvider,
		"the description must name the env var so a user knows what to change")
}

// TestSetAIDefaultClearsAStaleModel: choosing a provider with no model must not
// leave the previous provider's model attached to it.
func TestSetAIDefaultClearsAStaleModel(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(EnvAIProvider, "")
	t.Setenv(EnvAIModel, "")

	require.NoError(t, SetAIDefault(nil, "ollama", "gemma4:e2b"))
	require.NoError(t, SetAIDefault(nil, "claude-code", ""))

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	def := ResolveAIDefault(cfg)
	assert.Equal(t, "claude-code", def.Provider)
	assert.Empty(t, def.Model, "the previous provider's model must not carry over")
}

// TestClearAIDefault restores "nothing configured", and reports whether it had
// anything to clear.
func TestClearAIDefault(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(EnvAIProvider, "")
	t.Setenv(EnvAIModel, "")

	cleared, err := ClearAIDefault(nil)
	require.NoError(t, err)
	assert.False(t, cleared, "nothing stored yet")

	require.NoError(t, SetAIDefault(nil, "ollama", "gemma4:e2b"))
	cleared, err = ClearAIDefault(nil)
	require.NoError(t, err)
	assert.True(t, cleared)

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.False(t, ResolveAIDefault(cfg).Configured())
}

// TestSetAIDefaultMirrorsIntoTheRunningConfig: an inline wizard mid-`kapi up`
// must take effect for the rest of that process without re-reading the file.
func TestSetAIDefaultMirrorsIntoTheRunningConfig(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(EnvAIProvider, "")
	t.Setenv(EnvAIModel, "")

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	require.False(t, ResolveAIDefault(cfg).Configured())

	require.NoError(t, SetAIDefault(cfg, "ollama", "gemma4:e2b"))
	assert.Equal(t, "ollama", ResolveAIDefault(cfg).Provider,
		"the same in-memory config must see the change")
}

// TestModelOnlyConfigStillApplies: a hand-edited config with ai.model and no
// ai.provider is unusual but legal, and the model must reach the tool rather than
// being dropped. Configured() stays "a provider is set" so callers that must
// print a provider are unaffected; Any() is what the merge asks.
func TestModelOnlyConfigStillApplies(t *testing.T) {
	isolateRealConfigHome(t)
	require.NoError(t, SetGlobalConfig(KeyAIModel, "claude-sonnet-4-6"))
	t.Chdir(t.TempDir())

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	def := ResolveAIDefault(cfg)
	assert.False(t, def.Configured(), "no provider is set")
	assert.True(t, def.Any(), "but something is")
	assert.Equal(t, "claude-sonnet-4-6", def.Model)

	got := ApplyAIToolDefaults(cfg, "ai-translate", []string{"credentials"}, nil)
	assert.Equal(t, "claude-sonnet-4-6", got["model"],
		"the stored model must still reach the tool")
	assert.NotContains(t, got, "provider",
		"and no empty provider may be injected — the tool's own default applies")
}

// TestLoneEnvModelIsAttributedToTheEnvironment: KAPI_AI_MODEL alone must not be
// reported as coming from a file that does not contain it.
func TestLoneEnvModelIsAttributedToTheEnvironment(t *testing.T) {
	isolateRealConfigHome(t)
	t.Setenv(EnvAIModel, "gemma4:e2b")
	t.Chdir(t.TempDir())

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	def := ResolveAIDefault(cfg)
	assert.Equal(t, AIDefaultEnv, def.Source)
	assert.Equal(t, "gemma4:e2b", def.Model)
	assert.Empty(t, def.File, "no file backs an env-only default")
}

// TestDescribeNeverClaimsNothingWhenSomethingIsSet is the message-accuracy guard
// the founder asked for.
func TestDescribeNeverClaimsNothingWhenSomethingIsSet(t *testing.T) {
	assert.Equal(t, "no default AI provider configured", AIDefault{}.Describe())

	d := AIDefault{Provider: "ollama", Model: "gemma4:e2b", Source: AIDefaultConfig, File: "/tmp/kapi.yaml"}
	desc := d.Describe()
	assert.Contains(t, desc, "ollama")
	assert.Contains(t, desc, "gemma4:e2b")
	assert.Contains(t, desc, "/tmp/kapi.yaml", "say which file to change")
	assert.NotContains(t, desc, "no default")
}
