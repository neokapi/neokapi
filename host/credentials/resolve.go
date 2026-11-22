package credentials

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/credentials/providerenv"
)

// providerEnvVars maps a provider id to the ordered list of environment
// variables that may carry its API key. The first non-empty variable wins.
//
// API keys must never be sourced from the committable kapi.yaml recipe; only
// provider/model defaults belong there. The standard per-provider env var is
// the supported fallback when no inline --api-key and no --credential are
// given, slotting in just above store auto-detect in the resolution order.
//
// The list itself lives in core/credentials/providerenv, because the plugin
// conformance harness under core/ has to scrub the same variables the host
// does and cannot import host. This is an alias, not a copy.
var providerEnvVars = providerenv.Vars

// ProviderKeyEnvNames returns every environment variable that may carry a
// provider API key, sorted. It is the denylist for anything that hands an
// environment to a subprocess kapi did not write. See
// pluginhost.WithoutProviderKeys.
func ProviderKeyEnvNames() []string { return providerenv.Names() }

// keylessProviders are the providers that never require an API key: the local,
// on-device ones (ollama and demo, as aiprovider.IsLocalProvider lists) plus
// claude-code, which authenticates through the local Claude Code CLI login and
// bills the user's Claude subscription. Kept here (not imported) to avoid an
// import cycle with the provider packages.
var keylessProviders = map[string]bool{
	"ollama":      true,
	"demo":        true,
	"claude-code": true,
}

// isKeylessProvider reports whether a provider id needs no API key.
func isKeylessProvider(providerType string) bool {
	return keylessProviders[strings.ToLower(providerType)]
}

// Keyless reports whether a provider id needs no API key (local providers and
// the subscription-backed claude-code). Exported for the CLI command layer and
// the desktop bindings, so every surface shares one keyless list.
func Keyless(providerType string) bool { return isKeylessProvider(providerType) }

// ProvidersWithEnvKey returns the provider ids whose conventional API-key
// environment variable is currently set (e.g. ANTHROPIC_API_KEY → anthropic).
// Used by first-run provider detection.
func ProvidersWithEnvKey() []string {
	var out []string
	for provider := range providerEnvVars {
		if _, ok := apiKeyFromEnv(provider); ok {
			out = append(out, provider)
		}
	}
	slices.Sort(out)
	return out
}

// apiKeyFromEnv returns the API key for the given provider id from the
// environment, trying each mapped variable in order and returning the first
// non-empty value. It returns ("", false) when the provider has no mapped
// variable or none of them are set.
func apiKeyFromEnv(providerType string) (string, bool) {
	for _, name := range providerEnvVars[strings.ToLower(providerType)] {
		if v := os.Getenv(name); v != "" {
			return v, true
		}
	}
	return "", false
}

// inferProviderID determines the provider id to resolve credentials for.
//
// The config map's explicit "provider" wins when set. The unified `translate`
// and `qa` tools (and every other AI tool) carry the chosen LLM/MT provider id
// there. With no explicit provider the id defaults to "anthropic", the default
// the schema applies, so the common `kapi translate` (no --provider) path can
// still pick up ANTHROPIC_API_KEY from the environment.
func inferProviderID(_ string, config map[string]any) string {
	if p, ok := config["provider"].(string); ok && p != "" {
		return p
	}
	return "anthropic"
}

// ResolveCredentials resolves credential references in a tool config map.
// It looks for a "credential" key in the config, resolves it from the store,
// and injects provider/apiKey/model into the config map.
//
// toolName is the registered tool id (e.g. "translate", "qa"). The provider
// id comes from config["provider"] (defaulting to anthropic), never from the
// tool name.
//
// findCredential states the resolution order. Before it runs, the provider
// endpoint is taken out of the caller's hands: see stripRecipeEndpoint.
func ResolveCredentials(store *Store, toolName string, toolRequires []string, config map[string]any) (map[string]any, error) {
	// Unconditionally, and before every return below: the endpoint is host
	// configuration, so whatever the config arrived with is discarded here and
	// only a stored credential can put one back.
	config = stripRecipeEndpoint(config)

	src, err := findCredential(store, toolName, toolRequires, config)
	if err != nil {
		return nil, err
	}
	switch {
	case src.envKey != "":
		// Keeps the existing provider: only the missing api_key is filled in.
		return mergeCredentials(config, &ProviderConfigWithKey{
			ProviderType: src.envProvider,
			APIKey:       src.envKey,
		}), nil
	case src.stored != nil:
		resolved, err := loadCredentialKey(store, *src.stored)
		if err != nil {
			return nil, err
		}
		return mergeCredentials(config, resolved), nil
	default:
		return config, nil
	}
}

// MissingCredential returns the error ResolveCredentials would return for
// config when it finds no credential, or nil when it would find one. It
// applies the same order through findCredential and stops before reading a
// key from the keychain, so asking never prompts for keychain access. A nil
// store holds no credentials.
//
// Reading no keychain is the design, and it has two consequences. A saved
// credential whose keychain entry is gone counts as found here; the run then
// fails with `API key for credential "<name>" not found in keychain`. A
// keyless provider (ollama, claude-code) counts as found even when Ollama is
// not running or Claude Code is not logged in; the run reports that through
// the provider's own error.
func MissingCredential(store *Store, toolName string, toolRequires []string, config map[string]any) error {
	_, err := findCredential(store, toolName, toolRequires, config)
	return err
}

// credentialSource is where ResolveCredentials takes a config's key from. The
// zero value means the config needs nothing looked up: the tool requires no
// credentials, the config carries its own key, or the provider is keyless.
type credentialSource struct {
	envProvider string
	envKey      string
	stored      *ProviderConfig
}

// findCredential applies the resolution order of ResolveCredentials without
// reading any key from the keychain:
//
//  1. a tool that does not require "credentials" needs nothing;
//  2. an inline "api_key" wins;
//  3. a "credential" names a stored credential by name or ID;
//  4. a keyless provider (ollama, demo, claude-code) needs nothing;
//  5. the provider's standard environment variable;
//  6. otherwise auto-detect from the store, which needs a single match.
func findCredential(store *Store, toolName string, toolRequires []string, config map[string]any) (credentialSource, error) {
	if !slices.Contains(toolRequires, "credentials") {
		return credentialSource{}, nil
	}
	if apiKey, ok := config["api_key"].(string); ok && apiKey != "" {
		return credentialSource{}, nil
	}
	if credRef, ok := config["credential"].(string); ok && credRef != "" {
		cfg, err := lookupRef(store, credRef)
		if err != nil {
			return credentialSource{}, err
		}
		return credentialSource{stored: &cfg}, nil
	}
	// Keyless providers run on-device or on a local login, so a local run never
	// fails for want of a saved credential or env var. Kept in sync with
	// aiprovider.IsLocalProvider; duplicated here (not imported) to avoid an
	// import cycle with the provider packages, like providerEnvVars above.
	providerType := inferProviderID(toolName, config)
	if isKeylessProvider(providerType) {
		return credentialSource{}, nil
	}
	if key, ok := apiKeyFromEnv(providerType); ok {
		return credentialSource{envProvider: providerType, envKey: key}, nil
	}
	// Auto-detect matches on the config's explicit provider, not the inferred
	// default, so a config with no provider matches every saved credential.
	storeProvider, _ := config["provider"].(string)
	cfg, err := autoDetect(store, storeProvider)
	if err != nil {
		return credentialSource{}, err
	}
	return credentialSource{stored: &cfg}, nil
}

// configKeyBaseURL is the tool-config key naming the endpoint a provider is
// called at. The host owns it end to end: it is cleared on the way in and set
// only from a resolved credential.
const configKeyBaseURL = "base_url"

// stripRecipeEndpoint removes any endpoint the tool config arrived with.
//
// The endpoint and the secret sent to it are one decision, so they travel
// together or not at all. Whoever wrote the project directory authors the
// committable recipe; the person kapi runs as authors the per-machine
// credential. If the recipe could choose the endpoint, it could send a
// user's key to a host the user never picked. The policy for keys above
// (`providerEnvVars`) therefore covers the address as well as the secret.
//
// Self-hosted and private-cloud endpoints are configured next to the key they
// authenticate with, via `kapi credentials add --base-url`, and
// mergeCredentials puts them back.
//
// The map is copied rather than edited, because callers pass configs they
// still own. Only tool configs carrying the key pay for the copy.
func stripRecipeEndpoint(config map[string]any) map[string]any {
	if _, ok := config[configKeyBaseURL]; !ok {
		return config
	}
	out := make(map[string]any, len(config))
	maps.Copy(out, config)
	delete(out, configKeyBaseURL)
	return out
}

// MissingCredentialError is returned when no saved credential, inline key or
// environment variable gives a provider its key. Its text names what works
// for every command (MissingKeyMessage without the flag); a command that reads
// --api-key adds the flag with WithAPIKeyFlag.
type MissingCredentialError struct {
	// Provider is the provider the key is for, or "" when the config names
	// none (every saved credential was a candidate).
	Provider string
}

func (e *MissingCredentialError) Error() string { return MissingKeyMessage(e.Provider, false) }

// MissingKeyMessage says that provider has no key and what gives it one: a
// saved credential, the provider's environment variable, and, for a command
// that reads it, --api-key. It is the one statement of that advice: the
// runtime error, the pre-run refusal of `kapi up` and the desktop's
// missing-key dialog all print it.
func MissingKeyMessage(provider string, apiKeyFlag bool) string {
	var b strings.Builder
	if provider == "" {
		b.WriteString("no saved credentials found; save one with 'kapi credentials add'")
	} else {
		fmt.Fprintf(&b, "no saved credentials found for provider %q; save one with 'kapi credentials add'", provider)
	}
	if vars := providerEnvVars[strings.ToLower(provider)]; len(vars) > 0 {
		if apiKeyFlag {
			b.WriteString(", set " + vars[0])
		} else {
			b.WriteString(" or set " + vars[0])
		}
	}
	if apiKeyFlag {
		b.WriteString(" or pass --api-key")
	}
	return b.String()
}

// WithAPIKeyFlag returns err with a missing credential's advice naming
// --api-key, for a command that reads the flag. Any other error is returned
// as it is.
func WithAPIKeyFlag(err error) error {
	missing, ok := errors.AsType[*MissingCredentialError](err)
	if !ok {
		return err
	}
	return &flagHintError{err: err, msg: MissingKeyMessage(missing.Provider, true)}
}

// flagHintError keeps the wrapped chain (errors.As still finds the
// MissingCredentialError) and replaces only the missing-credential text.
type flagHintError struct {
	err error
	msg string
}

func (e *flagHintError) Error() string {
	if missing, ok := errors.AsType[*MissingCredentialError](e.err); ok {
		return strings.Replace(e.err.Error(), missing.Error(), e.msg, 1)
	}
	return e.err.Error()
}

func (e *flagHintError) Unwrap() error { return e.err }

// lookupRef looks up a credential by name first, then by ID.
func lookupRef(store *Store, ref string) (ProviderConfig, error) {
	if store == nil {
		return ProviderConfig{}, fmt.Errorf("credential %q not found (looked up by name and ID)", ref)
	}
	if cfg, err := store.GetByName(ref); err == nil {
		return cfg, nil
	}
	cfg, err := store.Get(ref)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("credential %q not found (looked up by name and ID)", ref)
	}
	return cfg, nil
}

// autoDetect finds the single matching credential in the store. Several
// matches resolve only when exactly one of them is marked default.
func autoDetect(store *Store, providerType string) (ProviderConfig, error) {
	var matches []ProviderConfig
	if store != nil {
		matches = store.FindByType(providerType)
	}
	switch len(matches) {
	case 0:
		return ProviderConfig{}, &MissingCredentialError{Provider: providerType}
	case 1:
		return matches[0], nil
	default:
		// Several match: a credential explicitly marked default for this provider
		// disambiguates (at most one, enforced by Store.SetDefault).
		var defaults []ProviderConfig
		for _, m := range matches {
			if m.Default {
				defaults = append(defaults, m)
			}
		}
		if len(defaults) == 1 {
			return defaults[0], nil
		}
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.Name
		}
		return ProviderConfig{}, &AmbiguousCredentialError{Provider: providerType, Candidates: names}
	}
}

// loadCredentialKey reads a config's API key from the keychain and pairs them.
func loadCredentialKey(store *Store, cfg ProviderConfig) (*ProviderConfigWithKey, error) {
	apiKey, err := store.GetAPIKey(cfg.ID)
	if err != nil {
		return nil, fmt.Errorf("API key for credential %q not found in keychain: %w", cfg.Name, err)
	}
	return &ProviderConfigWithKey{ProviderConfig: cfg, APIKey: apiKey}, nil
}

// AmbiguousCredentialError is returned when credential auto-detection finds
// more than one saved credential and cannot pick one on its own. It carries
// the candidate names (and the provider filter, if any) so each front-end can
// render its own guidance: Error() keeps the CLI's --credential hint, while a
// GUI catches it with errors.As and offers a picker or default-provider
// setting instead of echoing a CLI flag.
type AmbiguousCredentialError struct {
	// Provider is the provider type the auto-detect was filtered by, or ""
	// when no provider was specified (so every saved credential matched).
	Provider string
	// Candidates are the names of the credentials that matched.
	Candidates []string
}

func (e *AmbiguousCredentialError) Error() string {
	return fmt.Sprintf("multiple credentials found (%s); specify one with --credential <name>", strings.Join(e.Candidates, ", "))
}

// mergeCredentials injects resolved credential fields into the config map.
// Only sets fields that are not already explicitly set by the user.
func mergeCredentials(config map[string]any, cred *ProviderConfigWithKey) map[string]any {
	result := make(map[string]any, len(config))
	maps.Copy(result, config)

	// Remove the credential reference key: tools don't know about it.
	delete(result, "credential")

	// Inject resolved values (only if not already set).
	if _, ok := result["provider"]; !ok || result["provider"] == "" {
		result["provider"] = cred.ProviderType
	}
	result["api_key"] = cred.APIKey
	if cred.Model != "" {
		if _, ok := result["model"]; !ok || result["model"] == "" {
			result["model"] = cred.Model
		}
	}
	// The endpoint travels with the key that authenticates to it. A stored
	// credential may name a self-hosted or private-cloud host (`kapi
	// credentials add --base-url`), and that is the only way one is set:
	// stripRecipeEndpoint has already removed anything the config arrived
	// with, so this assignment is never overriding a user's intent.
	if cred.BaseURL != "" {
		result[configKeyBaseURL] = cred.BaseURL
	}

	return result
}
