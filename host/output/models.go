package output

import (
	"fmt"
	"io"
)

// Model sources, used as ModelRow.Source so the unified `kapi models` view can
// group rows by where the model comes from.
const (
	ModelSourceDetected = "detected" // a keyless provider found on this machine (e.g. Claude Code)
	ModelSourceOllama   = "ollama"   // local models served by an Ollama runtime
	ModelSourcePlugin   = "plugin"   // host-owned assets a plugin declares
	ModelSourceCloud    = "cloud"    // a remote provider's model (needs an API key)
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
	section := func(title string, build func(*Table)) {
		if !first {
			fmt.Fprintln(w)
		}
		first = false
		Title(w, title)
		t := NewTable(w).Accent(0)
		build(t)
		t.Render()
	}

	if rows := o.rows(ModelSourceDetected); len(rows) > 0 {
		section("Detected · no API key needed", func(t *Table) {
			s := t.Styles()
			t.Headers("PROVIDER", "DEFAULT MODEL", "NOTE")
			for _, m := range rows {
				t.Row(m.Provider, m.Model, s.Dim(m.Note))
			}
		})
	}

	if rows := o.rows(ModelSourceOllama); len(rows) > 0 {
		section("Local · Ollama", func(t *Table) {
			s := t.Styles()
			t.Headers("MODEL", "STATUS", "SIZE", "NOTE")
			for _, m := range rows {
				name := m.Model
				if m.Default {
					name += " *"
				}
				t.Row(name, modelStatusCell(s, m.Status), s.Dim(m.Size), s.Dim(m.Note))
			}
		})
	}

	if rows := o.rows(ModelSourcePlugin); len(rows) > 0 {
		section("Plugin models", func(t *Table) {
			s := t.Styles()
			t.Headers("PLUGIN", "MODEL", "VERSION", "STATUS", "SIZE")
			for _, m := range rows {
				name := m.Model
				if m.Default {
					name += " (default)"
				}
				t.Row(m.Provider, name, s.Dim(m.Version), modelStatusCell(s, m.Status), s.Dim(m.Size))
			}
		})
	}

	if rows := o.rows(ModelSourceCloud); len(rows) > 0 {
		section("Cloud providers · require an API key", func(t *Table) {
			t.Headers("PROVIDER", "DEFAULT MODEL", "")
			for _, m := range rows {
				t.Row(m.Provider, m.Model, m.Note)
			}
		})
	}

	return nil
}

// modelStatusCell colors a ModelRow.Status by whether the model is usable right
// now. The vocabulary comes from host/modelcmds.go.
func modelStatusCell(s *Styles, status string) string {
	switch status {
	case "detected", "installed", "cached", "bundled":
		return s.Success.Render(status)
	case "unknown":
		return s.Warn.Render(status)
	default:
		return s.Muted.Render(status)
	}
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
