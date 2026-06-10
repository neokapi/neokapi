package output

import (
	"fmt"
	"io"
	"strings"
)

// RegistryInfo represents a single registry entry.
type RegistryInfo struct {
	Name     string   `json:"name"`
	URL      string   `json:"url"`
	Channels []string `json:"channels,omitempty"`
}

// RegistryListOutput represents the list of registries.
type RegistryListOutput struct {
	Registries []RegistryInfo `json:"registries"`
	Total      int            `json:"total"`
}

func (o RegistryListOutput) FormatText(w io.Writer) error {
	if len(o.Registries) == 0 {
		fmt.Fprintln(w, T("registries.none"))
		return nil
	}

	fmt.Fprintf(w, "  %-20s %-50s %s\n",
		T("registries.header.name"), T("registries.header.url"), T("registries.header.channels"))
	fmt.Fprintf(w, "  %-20s %-50s %s\n", "----", "---", "--------")
	for _, r := range o.Registries {
		channels := "-"
		if len(r.Channels) > 0 {
			channels = strings.Join(r.Channels, ", ")
		}
		fmt.Fprintf(w, "  %-20s %-50s %s\n", r.Name, r.URL, channels)
	}
	fmt.Fprintf(w, "\n"+T("registries.total")+"\n", o.Total)
	return nil
}

// RegistryAddOutput represents the result of adding a registry.
type RegistryAddOutput struct {
	Name     string   `json:"name"`
	URL      string   `json:"url"`
	Channels []string `json:"channels,omitempty"`
}

func (o RegistryAddOutput) FormatText(w io.Writer) error {
	if len(o.Channels) > 0 {
		fmt.Fprintf(w, "Added registry %q (%s) channels: %s\n", o.Name, o.URL, strings.Join(o.Channels, ", "))
	} else {
		fmt.Fprintf(w, "Added registry %q (%s)\n", o.Name, o.URL)
	}
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
