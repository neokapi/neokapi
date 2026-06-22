package output

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// OllamaStatusOutput reports whether a usable Ollama runtime is present, used by
// `kapi models ollama status`. Ollama is the on-device GPU runtime kapi drives for
// local translation; this command tells the user exactly what (if anything) they
// still need to do.
type OllamaStatusOutput struct {
	BaseURL    string   `json:"base_url"`
	Installed  bool     `json:"installed"` // ollama binary found on PATH
	BinaryPath string   `json:"binary_path,omitempty"`
	Running    bool     `json:"running"`             // server responded
	Version    string   `json:"version,omitempty"`   // server version when running
	ModelCount int      `json:"model_count"`         // models already pulled
	Models     []string `json:"models,omitempty"`    // their names
	NextStep   string   `json:"next_step,omitempty"` // human guidance when something is missing
}

func (o OllamaStatusOutput) FormatText(w io.Writer) error {
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	fmt.Fprintf(w, "Ollama runtime (%s)\n", o.BaseURL)
	fmt.Fprintf(w, "  installed: %s", yn(o.Installed))
	if o.BinaryPath != "" {
		fmt.Fprintf(w, " (%s)", o.BinaryPath)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  running:   %s", yn(o.Running))
	if o.Version != "" {
		fmt.Fprintf(w, " (v%s)", o.Version)
	}
	fmt.Fprintln(w)
	if o.Running {
		fmt.Fprintf(w, "  models:    %d pulled\n", o.ModelCount)
	}
	if o.NextStep != "" {
		fmt.Fprintf(w, "\n%s\n", o.NextStep)
	}
	return nil
}

// OllamaModelRow is one installed model in an OllamaModelsOutput.
type OllamaModelRow struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Size      string `json:"size"`
	Modified  string `json:"modified,omitempty"`
}

// OllamaModelsOutput lists the models installed on the Ollama server.
type OllamaModelsOutput struct {
	Models []OllamaModelRow `json:"models"`
	Total  int              `json:"total"`
}

func (o OllamaModelsOutput) FormatText(w io.Writer) error {
	if o.Total == 0 {
		fmt.Fprintln(w, "No models installed. Pull one with `kapi models ollama pull <model>`.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MODEL\tSIZE\tMODIFIED")
	for _, m := range o.Models {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", m.Name, m.Size, m.Modified)
	}
	return tw.Flush()
}

// OllamaPullOutput reports the result of `kapi models ollama pull`.
type OllamaPullOutput struct {
	Model  string `json:"model"`
	Action string `json:"action"` // "pulled" | "present"
}

func (o OllamaPullOutput) FormatText(w io.Writer) error {
	if o.Action == "present" {
		fmt.Fprintf(w, "✓ %s is already installed.\n", o.Model)
		return nil
	}
	fmt.Fprintf(w, "✓ pulled %s\n", o.Model)
	return nil
}
