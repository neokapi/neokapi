package credentials

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
)

// providerEnvVars maps a provider id to the ordered list of environment
// variables that may carry its API key. The first non-empty variable wins.
// These follow the conventional names used by each provider's own SDKs and
// CLIs (and reuse GEMINI_API_KEY, the name the harness tooling already reads).
//
// API keys must never be sourced from the committable .kapi/kapi.yaml recipe;
// only provider/model defaults belong there. The standard per-provider env var
// is the supported fallback when no inline --api-key and no --credential are
// given, slotting in just above store auto-detect in the resolution order.
//
// Provider ids match the constants in providers/ai (anthropic, openai, gemini,
// azureopenai, ollama, demo) and providers/mt (deepl, google, microsoft,
// modernmt, mymemory). The map is the single source of truth for the fallback;
// keep it self-contained here to avoid import cycles with the provider
// packages.
var providerEnvVars = map[string][]string{
	// AI providers.
	"anthropic":   {"ANTHROPIC_API_KEY"},
	"openai":      {"OPENAI_API_KEY"},
	"gemini":      {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
	"azureopenai": {"AZURE_OPENAI_API_KEY"},
	// ollama and demo never require a key; they have no entry on purpose.

	// MT providers.
	"deepl":     {"DEEPL_API_KEY"},
	"google":    {"GOOGLE_TRANSLATE_API_KEY", "GOOGLE_API_KEY"},
	"microsoft": {"MICROSOFT_TRANSLATOR_KEY", "AZURE_TRANSLATOR_KEY"},
	"modernmt":  {"MODERNMT_API_KEY"},
	"mymemory":  {"MYMEMORY_API_KEY"},
}

// keylessProviders are the local, on-device providers that never require an API
// key. Mirrors aiprovider.IsLocalProvider (kept here to avoid an import cycle).
var keylessProviders = map[string]bool{
	"ollama": true,
	"demo":   true,
}

// isKeylessProvider reports whether a provider id runs locally with no key.
func isKeylessProvider(providerType string) bool {
	return keylessProviders[strings.ToLower(providerType)]
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
// The config map's explicit "provider" wins when set — the unified `translate`
// and `qa` tools (and every other AI tool) carry the chosen LLM/MT provider id
// there. With no explicit provider the id defaults to "anthropic" — the same
// default the schema applies — so the common `kapi translate` (no --provider)
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
// toolName is the registered tool id (e.g. "translate", "qa"); it is no longer
// used to infer the provider — the provider id is read from config["provider"]
// (defaulting to anthropic) for the env-var fallback.
//
// Resolution order (highest precedence first):
//  1. If the tool doesn't require "credentials" → return config unchanged
//  2. If config has "apiKey" already set → return unchanged (explicit inline key wins)
//  3. If config has "credential" → resolve by name or ID from store
//  4. If a standard per-provider env var is set → inject it as "apiKey"
//  5. Otherwise → auto-detect from store (single match required)
func ResolveCredentials(store *Store, toolName string, toolRequires []string, config map[string]any) (map[string]any, error) {
	if !slices.Contains(toolRequires, "credentials") {
		return config, nil
	}

	// Explicit inline API key takes priority.
	if apiKey, ok := config["apiKey"].(string); ok && apiKey != "" {
		return config, nil
	}

	// Resolve by credential name/ID.
	if credRef, ok := config["credential"].(string); ok && credRef != "" {
		resolved, err := resolveByRef(store, credRef)
		if err != nil {
			return nil, err
		}
		return mergeCredentials(config, resolved), nil
	}

	// Keyless local providers (Ollama, Gemma, Demo) run on-device and need no
	// API key, so credential resolution is a no-op for them — never fail a local
	// run by demanding a saved credential or env var. Kept in sync with
	// aiprovider.IsLocalProvider; duplicated here (not imported) to avoid an
	// import cycle with the provider packages, like providerEnvVars above.
	if isKeylessProvider(inferProviderID(toolName, config)) {
		return config, nil
	}

	// Environment-variable fallback: when no inline key and no --credential,
	// inject the standard per-provider env var for the resolved provider id.
	// This keeps the existing provider (we only fill in the missing apiKey).
	providerType := inferProviderID(toolName, config)
	if key, ok := apiKeyFromEnv(providerType); ok {
		return mergeCredentials(config, &ProviderConfigWithKey{
			ProviderConfig: ProviderConfig{ProviderType: providerType},
			APIKey:         key,
		}), nil
	}

	// Auto-detect: find matching credentials from store. Use the config's
	// explicit provider (not the inferred default) so behaviour around store
	// matching is unchanged from before the env fallback was added.
	storeProvider, _ := config["provider"].(string)
	resolved, err := autoDetect(store, storeProvider)
	if err != nil {
		return nil, err
	}
	return mergeCredentials(config, resolved), nil
}

// resolveByRef looks up a credential by name first, then by ID.
func resolveByRef(store *Store, ref string) (*ProviderConfigWithKey, error) {
	// Try name first.
	cfg, err := store.GetByName(ref)
	if err != nil {
		// Try ID.
		cfg, err = store.Get(ref)
		if err != nil {
			return nil, fmt.Errorf("credential %q not found (looked up by name and ID)", ref)
		}
	}

	apiKey, err := store.GetAPIKey(cfg.ID)
	if err != nil {
		return nil, fmt.Errorf("API key for credential %q not found in keychain: %w", cfg.Name, err)
	}

	return &ProviderConfigWithKey{
		ProviderConfig: cfg,
		APIKey:         apiKey,
	}, nil
}

// autoDetect finds a single matching credential from the store.
func autoDetect(store *Store, providerType string) (*ProviderConfigWithKey, error) {
	matches := store.FindByType(providerType)
	switch len(matches) {
	case 0:
		if providerType != "" {
			return nil, fmt.Errorf("no saved credentials found for provider %q; use 'kapi credentials add' to save one or pass --api-key directly", providerType)
		}
		return nil, errors.New("no saved credentials found; use 'kapi credentials add' to save one or pass --api-key directly")
	case 1:
		cfg := matches[0]
		apiKey, err := store.GetAPIKey(cfg.ID)
		if err != nil {
			return nil, fmt.Errorf("API key for credential %q not found in keychain: %w", cfg.Name, err)
		}
		return &ProviderConfigWithKey{
			ProviderConfig: cfg,
			APIKey:         apiKey,
		}, nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.Name
		}
		return nil, &AmbiguousCredentialError{Provider: providerType, Candidates: names}
	}
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

	// Remove the credential reference key — tools don't know about it.
	delete(result, "credential")

	// Inject resolved values (only if not already set).
	if _, ok := result["provider"]; !ok || result["provider"] == "" {
		result["provider"] = cred.ProviderType
	}
	result["apiKey"] = cred.APIKey
	if cred.Model != "" {
		if _, ok := result["model"]; !ok || result["model"] == "" {
			result["model"] = cred.Model
		}
	}

	return result
}
