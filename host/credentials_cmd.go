package host

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

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
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  NAME\tPROVIDER\tMODEL\tID\tKEY\n")
	for _, r := range o.Credentials {
		keyStatus := "missing"
		if r.HasKey {
			keyStatus = "ok"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", r.Name, r.Provider, r.Model, r.ID, keyStatus)
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%d credential(s)\n", o.Total)
	return nil
}
