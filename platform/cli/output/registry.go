package output

import (
	"fmt"
	"io"
)

// RegistryInfo represents a single registry entry.
type RegistryInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// RegistryListOutput represents the list of registries.
type RegistryListOutput struct {
	Registries []RegistryInfo `json:"registries"`
	Total      int            `json:"total"`
}

func (o RegistryListOutput) FormatText(w io.Writer) error {
	if len(o.Registries) == 0 {
		fmt.Fprintln(w, "No registries configured.")
		return nil
	}

	fmt.Fprintf(w, "  %-20s %s\n", "NAME", "URL")
	fmt.Fprintf(w, "  %-20s %s\n", "----", "---")
	for _, r := range o.Registries {
		fmt.Fprintf(w, "  %-20s %s\n", r.Name, r.URL)
	}
	fmt.Fprintf(w, "\nTotal: %d registry(ies)\n", o.Total)
	return nil
}

// RegistryAddOutput represents the result of adding a registry.
type RegistryAddOutput struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (o RegistryAddOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Added registry %q (%s)\n", o.Name, o.URL)
	return nil
}

// RegistryRemoveOutput represents the result of removing a registry.
type RegistryRemoveOutput struct {
	Name string `json:"name"`
}

func (o RegistryRemoveOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Removed registry %q\n", o.Name)
	return nil
}
