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

	t := NewTable(w).Accent(0).Headers(
		T("registries.header.name"), T("registries.header.url"), T("registries.header.channels"))
	s := t.Styles()
	for _, r := range o.Registries {
		t.Row(r.Name, r.URL, s.Dim(strings.Join(r.Channels, ", ")))
	}
	t.Render()

	fmt.Fprintln(w)
	Note(w, T("registries.total"), o.Total)
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
