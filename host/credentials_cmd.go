package host

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/host/credentials"
	"github.com/neokapi/neokapi/host/output"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// KnownProviderTypes returns the canonical set of credential provider types
// accepted by `credentials add`, derived from the registered AI providers
// (aiprovider.Providers). Plugins that register additional AI providers are
// reflected automatically. The result is a deduplicated, sorted slice suitable
// for both membership checks and the help text shown on rejection.
func KnownProviderTypes() []string {
	set := map[string]struct{}{}
	for _, p := range aiprovider.Providers() {
		set[strings.ToLower(p.Name.String())] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ValidateProviderType reports whether providerType is a known credential
// provider type (case-insensitive). It returns a descriptive error listing the
// valid values when the provider is unknown, so a typo is rejected before it is
// persisted to the store.
func ValidateProviderType(providerType string) error {
	known := KnownProviderTypes()
	want := strings.ToLower(strings.TrimSpace(providerType))
	if slices.Contains(known, want) {
		return nil
	}
	return fmt.Errorf("unknown provider %q; valid providers are: %s", providerType, strings.Join(known, ", "))
}

// SaveCredential validates the provider type, upserts the config, and stores
// the API key when one is given. It is the single save path behind both the
// CLI's `credentials add` and the desktop's SaveProvider, so a typo'd provider
// type is rejected before it reaches the store on either surface.
func SaveCredential(store *credentials.Store, cfg credentials.ProviderConfig, apiKey string) (credentials.ProviderConfig, error) {
	if err := ValidateProviderType(cfg.ProviderType); err != nil {
		return credentials.ProviderConfig{}, err
	}
	saved, err := store.Upsert(cfg)
	if err != nil {
		return credentials.ProviderConfig{}, fmt.Errorf("save provider config: %w", err)
	}
	if apiKey != "" {
		if err := store.SetAPIKey(saved.ID, apiKey); err != nil {
			return credentials.ProviderConfig{}, fmt.Errorf("store API key: %w", err)
		}
	}
	return saved, nil
}

// RemoveCredential deletes a credential's API key (best-effort, ignoring
// keychain errors) and its stored config.
func RemoveCredential(store *credentials.Store, id string) error {
	_ = store.DeleteAPIKey(id)
	return store.Remove(id)
}

// TestCredential reports whether a credential is usable: keyless providers
// (on-device plus subscription-backed claude-code) pass once the record
// exists; every other provider needs a non-empty key in the keychain. Shared
// so the CLI's `credentials test` and the desktop's TestProvider agree on the
// keyless case rather than one reporting "empty" where the other passes.
func TestCredential(store *credentials.Store, cfg credentials.ProviderConfig) (bool, error) {
	if credentials.Keyless(cfg.ProviderType) {
		return true, nil
	}
	key, err := store.GetAPIKey(cfg.ID)
	if err != nil {
		return false, fmt.Errorf("API key not found in keychain: %w", err)
	}
	return len(key) > 0, nil
}

type CredentialSavedOutput struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

func (o CredentialSavedOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Credential %q saved (provider: %s, id: %s)\n", o.Name, o.Provider, o.ID)
	return nil
}

type CredentialRow struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	ID       string `json:"id"`
	HasKey   bool   `json:"has_key"`
}

type CredentialListOutput struct {
	Credentials []CredentialRow `json:"credentials"`
	Total       int             `json:"total"`
}

func (o CredentialListOutput) FormatText(w io.Writer) error {
	if o.Total == 0 {
		fmt.Fprintln(w, "No saved credentials. Use 'kapi credentials add' to save one.")
		return nil
	}
	t := output.NewTable(w).Accent(0).Headers("NAME", "PROVIDER", "MODEL", "ID", "KEY")
	s := t.Styles()
	for _, r := range o.Credentials {
		key := s.Error.Render("missing")
		if r.HasKey {
			key = s.Success.Render("ok")
		}
		t.Row(r.Name, r.Provider, s.Dim(r.Model), r.ID, key)
	}
	t.Render()
	fmt.Fprintln(w)
	output.Note(w, "%d credential(s)", o.Total)
	return nil
}
