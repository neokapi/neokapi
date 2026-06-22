package output

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// Model sources, used as ModelRow.Source so the unified `kapi models` view can
// group rows by where the model comes from.
const (
	ModelSourceOllama = "ollama" // local models served by an Ollama runtime
	ModelSourcePlugin = "plugin" // host-owned assets a plugin declares
	ModelSourceCloud  = "cloud"  // a remote provider's model (needs an API key)
)

// ModelRow is one row in the unified model view — a model kapi can use, from any
// source. Source determines which group it renders under and which columns are
// meaningful (Provider is the Ollama runtime, the plugin name, or the cloud
// provider id).
type ModelRow struct {
	Source    string `json:"source"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Version   string `json:"version,omitempty"`
	Default   bool   `json:"default,omitempty"`
	Status    string `json:"status"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Size      string `json:"size,omitempty"`
	Note      string `json:"note,omitempty"`
}

// ModelsListOutput is the unified `kapi models list` view across Ollama, plugin
// assets, and cloud providers.
type ModelsListOutput struct {
	Models []ModelRow `json:"models"`
	Total  int        `json:"total"`
}

func (o ModelsListOutput) rows(source string) []ModelRow {
	var out []ModelRow
	for _, m := range o.Models {
		if m.Source == source {
			out = append(out, m)
		}
	}
	return out
}

func (o ModelsListOutput) FormatText(w io.Writer) error {
	if o.Total == 0 {
		fmt.Fprintln(w, "No models available. Install the Ollama runtime (`kapi models ollama install`)")
		fmt.Fprintln(w, "or configure a cloud provider to translate with an LLM.")
		return nil
	}

	first := true
	section := func(title string, render func(*tabwriter.Writer)) {
		if !first {
			fmt.Fprintln(w)
		}
		first = false
		fmt.Fprintln(w, title)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		render(tw)
		_ = tw.Flush()
	}

	if rows := o.rows(ModelSourceOllama); len(rows) > 0 {
		section("Local · Ollama", func(tw *tabwriter.Writer) {
			fmt.Fprintln(tw, "  MODEL\tSTATUS\tSIZE\tNOTE")
			for _, m := range rows {
				name := m.Model
				if m.Default {
					name += " *"
				}
				fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", name, m.Status, dash(m.Size), m.Note)
			}
		})
	}

	if rows := o.rows(ModelSourcePlugin); len(rows) > 0 {
		section("Plugin models", func(tw *tabwriter.Writer) {
			fmt.Fprintln(tw, "  PLUGIN\tMODEL\tVERSION\tSTATUS\tSIZE")
			for _, m := range rows {
				name := m.Model
				if m.Default {
					name += " (default)"
				}
				fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", m.Provider, name, m.Version, m.Status, dash(m.Size))
			}
		})
	}

	if rows := o.rows(ModelSourceCloud); len(rows) > 0 {
		section("Cloud providers · require an API key", func(tw *tabwriter.Writer) {
			fmt.Fprintln(tw, "  PROVIDER\tDEFAULT MODEL")
			for _, m := range rows {
				fmt.Fprintf(tw, "  %s\t%s\n", m.Provider, m.Model)
			}
		})
	}

	return nil
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// ModelActionOutput reports the result of a `kapi models pull` / `prune`. Plugin
// assets report a cache Dir; Ollama models do not.
type ModelActionOutput struct {
	Source string `json:"source,omitempty"` // "ollama" | "plugin"
	Plugin string `json:"plugin,omitempty"`
	Model  string `json:"model"`
	Dir    string `json:"dir,omitempty"`
	Action string `json:"action"` // "ready" | "removed" | "absent" | "present"
}

func (o ModelActionOutput) FormatText(w io.Writer) error {
	name := o.Model
	if o.Plugin != "" {
		name = o.Plugin + "/" + o.Model
	}
	switch o.Action {
	case "absent":
		fmt.Fprintf(w, "%s is not cached.\n", name)
	case "present":
		fmt.Fprintf(w, "✓ %s is already installed.\n", name)
	case "removed":
		if o.Dir != "" {
			fmt.Fprintf(w, "✓ removed %s (%s)\n", name, o.Dir)
		} else {
			fmt.Fprintf(w, "✓ removed %s\n", name)
		}
	default: // "ready"
		if o.Dir != "" {
			fmt.Fprintf(w, "✓ %s ready at %s\n", name, o.Dir)
		} else {
			fmt.Fprintf(w, "✓ %s ready\n", name)
		}
	}
	return nil
}
