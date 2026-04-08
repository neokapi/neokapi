package credentials

import (
	"fmt"
	"slices"
	"strings"
)

// ResolveCredentials resolves credential references in a tool config map.
// It looks for a "credential" key in the config, resolves it from the store,
// and injects provider/apiKey/model into the config map.
//
// Resolution order:
//  1. If the tool doesn't require "credentials" → return config unchanged
//  2. If config has "apiKey" already set → return unchanged (explicit inline key wins)
//  3. If config has "credential" → resolve by name or ID from store
//  4. If neither → auto-detect from store (single match required)
func ResolveCredentials(store *Store, toolRequires []string, config map[string]any) (map[string]any, error) {
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

	// Auto-detect: find matching credentials from store.
	providerType, _ := config["provider"].(string)
	resolved, err := autoDetect(store, providerType)
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
		return nil, fmt.Errorf("no saved credentials found; use 'kapi credentials add' to save one or pass --api-key directly")
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
		return nil, fmt.Errorf("multiple credentials found (%s); specify one with --credential <name>", strings.Join(names, ", "))
	}
}

// mergeCredentials injects resolved credential fields into the config map.
// Only sets fields that are not already explicitly set by the user.
func mergeCredentials(config map[string]any, cred *ProviderConfigWithKey) map[string]any {
	result := make(map[string]any, len(config))
	for k, v := range config {
		result[k] = v
	}

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
