package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/channel"
	"github.com/spf13/viper"
)

// AppConfig holds application-level configuration loaded via Viper.
type AppConfig struct {
	v *viper.Viper
}

// NewAppConfig creates a reader for kapi's app configuration: the global config
// file (plugins, formats, flow, the default AI provider/model), plus env-var
// overrides and built-in defaults.
//
// # One path, shared with the writer
//
// The file is pinned to [GlobalConfigFilePath] — the same function every writer
// uses (SetGlobalConfig, UnsetGlobalConfig, AddGlobalRegistry) — rather than
// resolved through a search path. A search path made read and write disagree in
// two independent ways, and `kapi models default` silently lost its value to
// both:
//
//  1. `.` (the working directory) was searched *before* the user config dir, and
//     a kapi project's recipe is named `kapi.yaml` — exactly the name being
//     searched for. Inside any project, the recipe was loaded *as* the app
//     config, so `ai.provider` read as empty however it had been set.
//  2. The writer resolved `os.UserConfigDir()` while the reader searched a
//     hardcoded `$HOME/.config/kapi`. On macOS those are different directories,
//     so even outside a project the stored value was in a directory the reader
//     never looked in.
//
// A recipe is not app configuration, and app configuration is not per-directory:
// the two never had a reason to share a search path. The locations the old reader
// searched are still read as lower-precedence layers (see legacyConfigPaths), so
// an existing `$HOME/.config/kapi/kapi.yaml` or `/etc/kapi/kapi.yaml` keeps
// working — what is removed is only the working directory.
func NewAppConfig() *AppConfig {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(GlobalConfigFilePath())
	v.SetEnvPrefix("KAPI")
	// Translate dots to underscores so dotted keys like "plugins.directory"
	// resolve from env vars like KAPI_PLUGINS_DIRECTORY (not KAPI_PLUGINS.DIRECTORY).
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults
	v.SetDefault(KeyFlowChannelBuffer, 64)
	pluginDir := "./plugins"
	if configDir, err := os.UserConfigDir(); err == nil {
		pluginDir = filepath.Join(configDir, "kapi", "plugins")
	}
	v.SetDefault(KeyPluginsDirectory, pluginDir)
	v.SetDefault(KeyPluginsRegistry, DefaultRegistryURL)

	return &AppConfig{v: v}
}

// NewOverlayAppConfig creates a config reader that layers app-specific config
// on top of the shared kapi config. App-specific settings are read from
// ~/.config/<appName>/<appName>.yaml; shared settings (plugins, formats, flow)
// come from the kapi config.
func NewOverlayAppConfig(appName string, configure func(cfg *AppConfig)) *AppConfig {
	cfg := NewAppConfig()

	overlayPath := GlobalConfigFilePath(appName)
	if _, err := os.Stat(overlayPath); err == nil {
		overlay := viper.New()
		overlay.SetConfigFile(overlayPath)
		overlay.SetConfigType("yaml")
		if err := overlay.ReadInConfig(); err != nil {
			// A present-but-unreadable overlay config is a real error; only
			// a missing file (filtered by os.Stat above) is acceptable.
			fmt.Fprintf(os.Stderr, "Warning: overlay config %s: %v\n", overlayPath, err)
		} else {
			// Merge into the file-layer (not Set) so env vars still beat the
			// overlay file and flag > env > file precedence is preserved.
			if err := cfg.v.MergeConfigMap(overlay.AllSettings()); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: merge overlay config %s: %v\n", overlayPath, err)
			}
		}
	}

	if configure != nil {
		configure(cfg)
	}

	return cfg
}

// legacyConfigPaths are pre-existing app-config locations kept readable as
// lower-precedence layers beneath the pinned global file, so an installation
// that already had one keeps working:
//
//   - `$HOME/.config/kapi/kapi.yaml` — the location the old reader searched and
//     the docs name. On Linux it *is* the pinned path (os.UserConfigDir honours
//     XDG), so this entry is a no-op there; on macOS os.UserConfigDir resolves to
//     `~/Library/Application Support`, and a hand-written config at the
//     documented path must not stop being read just because the reader stopped
//     guessing.
//   - `/etc/kapi/kapi.yaml` — a system-wide install.
//
// The working directory is deliberately absent: `./kapi.yaml` is a project
// recipe, not app configuration, and searching it is what made `kapi models
// default` unreadable inside any project (see NewAppConfig). A path equal to the
// pinned file is skipped, so nothing is read twice.
func legacyConfigPaths() []string {
	paths := []string{filepath.Join("/etc", "kapi", "kapi.yaml")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append([]string{filepath.Join(home, ".config", "kapi", "kapi.yaml")}, paths...)
	}
	pinned := GlobalConfigFilePath()
	return slices.DeleteFunc(paths, func(p string) bool { return p == pinned })
}

// Load reads the configuration file, then merges any legacy location underneath
// it. A missing file is not an error — an unconfigured kapi runs on its built-in
// defaults.
//
// Precedence is per **top-level block**, not per leaf: if the pinned file
// mentions `ai:` at all, the legacy file's whole `ai:` block is ignored rather
// than merged leaf by leaf. Deliberate — a half-merged block is the harder thing
// to reason about, and the file kapi writes always writes a complete block.
func (c *AppConfig) Load() error {
	if err := c.v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || errors.Is(err, os.ErrNotExist) {
			// Nothing stored yet; fall through to the legacy layers.
		} else {
			return err
		}
	}
	c.mergeLegacyLayers()
	return nil
}

// mergeLegacyLayers folds each legacy config file into the defaults layer,
// skipping any top-level block the pinned file already declares (see Load on why
// the granularity is the block). Best-effort: a legacy file that cannot be parsed
// is reported and skipped rather than failing the run, since it is not the file
// kapi writes.
func (c *AppConfig) mergeLegacyLayers() {
	for _, path := range legacyConfigPaths() {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		layer := viper.New()
		layer.SetConfigFile(path)
		layer.SetConfigType("yaml")
		if err := layer.ReadInConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: config %s: %v\n", path, err)
			continue
		}
		for key, val := range layer.AllSettings() {
			if c.v.InConfig(key) {
				continue // the pinned file already decided this key
			}
			c.v.SetDefault(key, val)
		}
	}
}

// Viper returns the underlying Viper instance for advanced access.
func (c *AppConfig) Viper() *viper.Viper {
	return c.v
}

// GetString returns a string config value.
func (c *AppConfig) GetString(key string) string {
	return c.v.GetString(key)
}

// GetInt returns an integer config value.
func (c *AppConfig) GetInt(key string) int {
	return c.v.GetInt(key)
}

// GetBool returns a boolean config value.
func (c *AppConfig) GetBool(key string) bool {
	return c.v.GetBool(key)
}

// Set sets a config value.
func (c *AppConfig) Set(key string, value any) {
	c.v.Set(key, value)
}

// ChannelBuffer returns the configured channel buffer size.
func (c *AppConfig) ChannelBuffer() int {
	return c.v.GetInt(KeyFlowChannelBuffer)
}

// PluginDirectory returns the configured plugin directory.
func (c *AppConfig) PluginDirectory() string {
	return c.v.GetString(KeyPluginsDirectory)
}

// Language returns the configured target locale for CLI/UI output
// (BCP-47, e.g. "fr"). Empty when unset — the i18n.Resolve chain
// then falls back to KAPI_LANG / LC_ALL / LANG.
func (c *AppConfig) Language() string {
	return c.v.GetString(KeyLanguage)
}

// RegistryURL returns the URL of the remote plugin registry.
func (c *AppConfig) RegistryURL() string {
	return c.v.GetString(KeyPluginsRegistry)
}

// UpdateChannel returns the release channel `kapi update` and the background
// update notifier track. A persisted update.channel (or KAPI_UPDATE_CHANNEL env)
// is the sticky source of truth; when unset it falls back to the channel inferred
// from the running build (a prerelease build defaults to "beta"), so a fresh beta
// install tracks beta without configuration. The persisted preference outlives a
// later update to a final (non-prerelease) version — see EnsureChannelPinned.
func (c *AppConfig) UpdateChannel() string {
	if ch := c.v.GetString(KeyUpdateChannel); ch != "" {
		return ch // explicit KAPI_UPDATE_CHANNEL env or kapi.yaml update.channel
	}
	return channel.Resolve() // shared persisted preference, else inferred from build
}

// EnsureChannelPinned makes beta-channel membership sticky via the shared
// channel preference (core/channel), so the CLI and the desktop apps all honor
// it. The first time a prerelease build runs with nothing pinned and no
// KAPI_UPDATE_CHANNEL override, beta is persisted — so a beta user stays on the
// fast track after updating to a final release (the beta channel also serves
// finals, so its version no longer infers "beta"). A pre-existing kapi.yaml
// update.channel is treated as an explicit choice and left untouched. Best-effort.
func (c *AppConfig) EnsureChannelPinned(_ ...string) {
	if c.v.InConfig(KeyUpdateChannel) {
		return // an explicit kapi.yaml choice already exists
	}
	channel.EnsurePinned()
}

// RegistryEntry represents a named plugin registry.
type RegistryEntry struct {
	Name     string   `yaml:"name"               json:"name"`
	URL      string   `yaml:"url"                json:"url"`
	Channels []string `yaml:"channels,omitempty" json:"channels,omitempty"`
}

// DefaultRegistryURL is the official neokapi plugin registry.
const DefaultRegistryURL = "https://neokapi.github.io/registry/manifest-plugins.json"

// DefaultUpdateChannel is the stable release channel. It is the ultimate
// fallback; the effective default for an unconfigured build is version.Channel()
// (a prerelease build defaults to "beta") — see UpdateChannel.
const DefaultUpdateChannel = "stable"

// EnvUpdateChannel is the environment variable that overrides the update channel
// for both the CLI and the desktop apps.
const EnvUpdateChannel = "KAPI_UPDATE_CHANNEL"

// Config key constants for use with Get/Set/BindEnv to avoid scattered magic strings.
const (
	KeyFlowChannelBuffer = "flow.channelBuffer"
	KeyPluginsDirectory  = "plugins.directory"
	KeyPluginsRegistry   = "plugins.registry"
	KeyFormatsPriorities = "formats.priorities"
	KeyLanguage          = "language"
	KeyUpdateChannel     = "update.channel"
	// KeyAIProvider / KeyAIModel are the default AI provider and model used by AI
	// tools (ai-translate, qa, voice-check, …) and flows when no
	// --provider/--model flag or recipe default is given. Set e.g. to "ollama"
	// (with ai.model "llama3.2:3b") to run locally by default. An explicit flag,
	// inline config, or project recipe default still wins.
	KeyAIProvider = "ai.provider"
	KeyAIModel    = "ai.model"
)

// Registries returns the configured list of plugin registries.
// If the "registries" key is set, those entries are returned.
// Otherwise, falls back to "plugins.registry" wrapped as a single entry named "default".
func (c *AppConfig) Registries() []RegistryEntry {
	raw := c.v.Get("registries")
	if raw != nil {
		if entries := parseRegistryEntries(raw); len(entries) > 0 {
			return entries
		}
	}
	// Fallback to single registry URL.
	url := c.v.GetString(KeyPluginsRegistry)
	if url == "" {
		url = DefaultRegistryURL
	}
	return []RegistryEntry{{Name: "default", URL: url}}
}

// parseRegistryEntries converts Viper's raw any into []RegistryEntry.
// Handles both []any (from YAML) and []map[string]any (from Set).
func parseRegistryEntries(raw any) []RegistryEntry {
	switch v := raw.(type) {
	case []any:
		var entries []RegistryEntry
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if e, ok := registryEntryFromMap(m); ok {
					entries = append(entries, e)
				}
			}
		}
		return entries
	case []map[string]any:
		var entries []RegistryEntry
		for _, m := range v {
			if e, ok := registryEntryFromMap(m); ok {
				entries = append(entries, e)
			}
		}
		return entries
	default:
		return nil
	}
}

// registryEntryFromMap extracts a RegistryEntry from a map.
func registryEntryFromMap(m map[string]any) (RegistryEntry, bool) {
	name, _ := m["name"].(string)
	url, _ := m["url"].(string)
	if name == "" || url == "" {
		return RegistryEntry{}, false
	}
	e := RegistryEntry{Name: name, URL: url}
	if raw, ok := m["channels"]; ok {
		e.Channels = parseStringSlice(raw)
	}
	return e, true
}

// parseStringSlice converts an any to []string.
func parseStringSlice(raw any) []string {
	switch v := raw.(type) {
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

// GlobalConfigFilePath returns the path to the global config file
// for the given app name (e.g. ~/.config/kapi/kapi.yaml).
// If no app name is provided, defaults to "kapi".
//
// Resolution order:
//
//  1. The per-app env override <NAME>_CONFIG_DIR, with hyphens mapped to
//     underscores (e.g. "kapi-desktop" → KAPI_DESKTOP_CONFIG_DIR), so
//     hyphenated app names get a legal environment variable name.
//  2. KAPI_CONFIG_DIR, which nests a sibling app under kapi's config root
//     ($KAPI_CONFIG_DIR/<name>/<name>.yaml). This is what makes the isolation
//     contract whole: `kapi config set bowrain.…` writes a *plugin's* config
//     file, so pinning KAPI_CONFIG_DIR must contain that write too — otherwise
//     an isolated run reaches into the developer's real home.
//  3. The OS user config dir.
func GlobalConfigFilePath(appName ...string) string {
	name := "kapi"
	if len(appName) > 0 && appName[0] != "" {
		name = appName[0]
	}
	envKey := strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_CONFIG_DIR"
	if dir := os.Getenv(envKey); dir != "" {
		return filepath.Join(dir, name+".yaml")
	}
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		if name == "kapi" {
			return filepath.Join(dir, "kapi.yaml")
		}
		return filepath.Join(dir, name, name+".yaml")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, name, name+".yaml")
}

// SetGlobalConfig sets a key-value pair in the global config file.
// The file is loaded, updated, and written back as YAML.
// If an app name is provided, it determines the config path;
// otherwise defaults to "kapi".
func SetGlobalConfig(key, value string, appName ...string) error {
	path := GlobalConfigFilePath(appName...)

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Try to read existing config (ignore not-found).
	_ = v.ReadInConfig()

	v.Set(key, value)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return v.WriteConfigAs(path)
}

// UnsetGlobalConfig removes a key from the global config file, restoring the
// built-in default. Viper has no delete, so the file is read, the key pruned
// from the nested settings map, and the result written back. Removing the last
// leaf of a nested block prunes the now-empty parent too, so an unset leaves no
// hollow `ai: {}` behind. It reports whether the key was present.
func UnsetGlobalConfig(key string, appName ...string) (bool, error) {
	path := GlobalConfigFilePath(appName...)
	if _, err := os.Stat(path); err != nil {
		return false, nil // nothing stored: unsetting is a no-op, not an error
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	settings := v.AllSettings()
	if !pruneKey(settings, strings.Split(strings.ToLower(key), ".")) {
		return false, nil
	}

	out := viper.New()
	out.SetConfigFile(path)
	out.SetConfigType("yaml")
	for k, val := range settings {
		out.Set(k, val)
	}
	if err := out.WriteConfigAs(path); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// pruneKey deletes the dotted path from a nested settings map, dropping any
// parent left empty. It reports whether something was removed.
func pruneKey(m map[string]any, path []string) bool {
	if len(path) == 0 {
		return false
	}
	head := path[0]
	if len(path) == 1 {
		if _, ok := m[head]; !ok {
			return false
		}
		delete(m, head)
		return true
	}
	child, ok := m[head].(map[string]any)
	if !ok {
		return false
	}
	if !pruneKey(child, path[1:]) {
		return false
	}
	if len(child) == 0 {
		delete(m, head)
	}
	return true
}

// GlobalConfigValues reads the flat, dotted key → value map stored in an app's
// global config file. Unlike AppConfig it reads only that one file — no env
// vars, no defaults, no search path — so a listing can say what is actually
// persisted and where, which is what `kapi config list` reports.
func GlobalConfigValues(appName ...string) (map[string]string, error) {
	path := GlobalConfigFilePath(appName...)
	if _, err := os.Stat(path); err != nil {
		return map[string]string{}, nil
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	out := make(map[string]string, len(v.AllKeys()))
	for _, k := range v.AllKeys() {
		out[k] = v.GetString(k)
	}
	return out, nil
}

// FormatPriorities returns the configured format priority overrides.
// The map keys are format names and values are priority integers.
// Higher values are preferred when multiple formats match the same
// MIME type or file extension.
func (c *AppConfig) FormatPriorities() map[string]int {
	result := make(map[string]int)
	sub := c.v.GetStringMap("formats.priorities")
	for name, val := range sub {
		switch v := val.(type) {
		case int:
			result[name] = v
		case int64:
			result[name] = int(v)
		case float64:
			result[name] = int(v)
		}
	}
	return result
}
