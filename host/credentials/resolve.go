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
// environment to a subprocess kapi did not write: see
// pluginhost.WithoutProviderKeys.
func ProviderKeyEnvNames() []string { return providerenv.Names() }

// keylessProviders are the providers that never require an API key: the local,
// on-device ones (ollama, demo, mirrors aiprovider.IsLocalProvider) plus
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
// The config map's explicit "provider" wins when set: the unified `translate`
// and `qa` tools (and every other AI tool) carry the chosen LLM/MT provider id
// there. With no explicit provider the id defaults to "anthropic" (the same
// default the schema applies) so the common `kapi translate` (no --provider)
// path can still pick up ANTHROPIC_API_KEY from the environment.
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
// id is read from config["provider"] (defaulting to anthropic), not inferred
// from the tool.
//
// Resolution order (highest precedence first):
//  1. If the tool doesn't require "credentials" → return config unchanged
//  2. If config has "api_key" already set → return unchanged (explicit inline key wins)
//  3. If config has "credential" → resolve by name or ID from store
//  4. If a standard per-provider env var is set → inject it as "api_key"
//  5. Otherwise → auto-detect from store (single match required)
//
// Before any of that, the provider endpoint is taken out of the caller's hands:
// see stripRecipeEndpoint.
func ResolveCredentials(store *Store, toolName string, toolRequires []string, config map[string]any) (map[string]any, error) {
	// Unconditionally, and before every return below: the endpoint is host
	// configuration, so whatever the config arrived with is discarded here and
	// only a stored credential can put one back.
	config = stripRecipeEndpoint(config)

	d, err := decide(store, toolName, toolRequires, config)
	if err != nil {
		return nil, err
	}
	switch {
	case d.env != "":
		// The standard per-provider env var fills in the missing key and
		// keeps the provider.
		return mergeCredentials(config, &ProviderConfigWithKey{ProviderType: d.provider, APIKey: d.env}), nil
	case d.stored != nil:
		resolved, err := loadCredentialKey(store, *d.stored)
		if err != nil {
			return nil, err
		}
		return mergeCredentials(config, resolved), nil
	}
	return config, nil
}

// Available reports the provider a tool configured with config would call
// and whether its credential is available, as ResolveCredentials decides it,
// without reading a saved key from the keychain. The error is the one
// ResolveCredentials returns for the same config (a missing or ambiguous
// credential), so a surface that reports a run in advance says what the run
// would say. provider is "" for a tool that needs no credentials.
func Available(store *Store, toolName string, toolRequires []string, config map[string]any) (provider string, err error) {
	if store == nil {
		store = &Store{} // no store opened: nothing is saved
	}
	d, err := decide(store, toolName, toolRequires, stripRecipeEndpoint(config))
	return d.provider, err
}

// decision is where a tool's credential comes from. At most one of env and
// stored is set; neither is set for an inline key, a keyless provider, or a
// tool that needs no credentials.
type decision struct {
	provider string
	env      string          // the key from the provider's environment variable
	stored   *ProviderConfig // the saved credential whose key the run reads
}

// decide resolves where a tool's credential comes from, in the order
// ResolveCredentials documents, reading only the store's metadata.
func decide(store *Store, toolName string, toolRequires []string, config map[string]any) (decision, error) {
	if !slices.Contains(toolRequires, "credentials") {
		return decision{}, nil
	}
	providerType := inferProviderID(toolName, config)

	// Explicit inline API key takes priority.
	if apiKey, ok := config["api_key"].(string); ok && apiKey != "" {
		return decision{provider: providerType}, nil
	}

	// Resolve by credential name/ID.
	if credRef, ok := config["credential"].(string); ok && credRef != "" {
		cfg, err := lookupRef(store, credRef)
		if err != nil {
			return decision{provider: providerType}, err
		}
		return decision{provider: credentialProvider(config, cfg), stored: &cfg}, nil
	}

	// Keyless local providers (Ollama, Gemma, Demo) run on-device and need no
	// API key, so credential resolution is a no-op for them: a local run
	// never fails for want of a saved credential or env var. Kept in sync with
	// aiprovider.IsLocalProvider; duplicated here (not imported) to avoid an
	// import cycle with the provider packages, like providerEnvVars above.
	if isKeylessProvider(providerType) {
		return decision{provider: providerType}, nil
	}

	// Environment-variable fallback: with no inline key and no --credential,
	// the standard per-provider env var for the resolved provider id.
	if key, ok := apiKeyFromEnv(providerType); ok {
		return decision{provider: providerType, env: key}, nil
	}

	// Auto-detect: find matching credentials from store. Use the config's
	// explicit provider (not the inferred default), so a config that names
	// none matches a saved credential of any provider.
	storeProvider, _ := config["provider"].(string)
	cfg, err := autoDetect(store, storeProvider, providerType)
	if err != nil {
		return decision{provider: providerType}, err
	}
	return decision{provider: credentialProvider(config, cfg), stored: &cfg}, nil
}

// credentialProvider is the provider a config resolved through the saved
// credential cfg calls: the config's own, else the credential's
// (mergeCredentials).
func credentialProvider(config map[string]any, cfg ProviderConfig) string {
	if p, ok := config["provider"].(string); ok && p != "" {
		return p
	}
	return cfg.ProviderType
}

// MissingKeyMessage is what a run says when provider needs an API key and
// none is set: no inline key, no saved credential, and no key in the
// provider's environment variable.
func MissingKeyMessage(provider string) string {
	how := "save one with 'kapi credentials add'"
	if vars := providerEnvVars[strings.ToLower(provider)]; len(vars) > 0 {
		how = "set " + vars[0] + ", " + how
	}
	return fmt.Sprintf("no saved credentials found for provider %q; %s, or pass --api-key directly", provider, how)
}

// configKeyBaseURL is the tool-config key naming the endpoint a provider is
// called at. The host owns it end to end: it is cleared on the way in and set
// only from a resolved credential.
const configKeyBaseURL = "base_url"

// stripRecipeEndpoint removes any endpoint the tool config arrived with.
//
// The endpoint and the secret sent to it are one decision, so they travel
// together or not at all. A recipe is committable and is authored by whoever
// wrote the project directory; a credential is per-machine and is authored by
// the person kapi is running as. A recipe that chose the endpoint would send
// a user's key to a host the user never picked, so the policy for keys
// (providerEnvVars) covers the address as well.
//
// Self-hosted endpoints are configured next to the key they authenticate
// with (`kapi credentials add --base-url`) and re-injected by
// mergeCredentials.
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

// lookupRef looks up a credential's metadata by name first, then by ID.
func lookupRef(store *Store, ref string) (ProviderConfig, error) {
	cfg, err := store.GetByName(ref)
	if err != nil {
		cfg, err = store.Get(ref)
		if err != nil {
			return ProviderConfig{}, fmt.Errorf("credential %q not found (looked up by name and ID); 'kapi credentials list' shows the saved ones", ref)
		}
	}
	return cfg, nil
}

// autoDetect finds a single matching credential in the store, by its
// metadata. providerType filters the match ("" matches every provider);
// provider is the one the run would call, which a missing credential names.
func autoDetect(store *Store, providerType, provider string) (ProviderConfig, error) {
	matches := store.FindByType(providerType)
	switch len(matches) {
	case 0:
		return ProviderConfig{}, errors.New(MissingKeyMessage(provider))
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
