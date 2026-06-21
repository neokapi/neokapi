package output

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// ModelAssetRow is one declared model asset in a ModelsListOutput.
type ModelAssetRow struct {
	Plugin    string `json:"plugin"`
	Model     string `json:"model"`
	Version   string `json:"version"`
	Default   bool   `json:"default"`
	Status    string `json:"status"` // "cached" | "not cached" | "unknown"
	SizeBytes int64  `json:"size_bytes"`
	Size      string `json:"size"` // human-readable form of SizeBytes
}

// ModelsListOutput lists the model assets declared by installed plugins.
type ModelsListOutput struct {
	Models []ModelAssetRow `json:"models"`
	Total  int             `json:"total"`
}

func (o ModelsListOutput) FormatText(w io.Writer) error {
	if o.Total == 0 {
		fmt.Fprintln(w, "No installed plugins declare model assets.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PLUGIN\tMODEL\tVERSION\tSTATUS\tSIZE")
	for _, m := range o.Models {
		name := m.Model
		if m.Default {
			name += " (default)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", m.Plugin, name, m.Version, m.Status, m.Size)
	}
	return tw.Flush()
}

// ModelActionOutput reports the result of `kapi models pull` / `prune`.
type ModelActionOutput struct {
	Plugin string `json:"plugin"`
	Model  string `json:"model"`
	Dir    string `json:"dir,omitempty"`
	Action string `json:"action"` // "ready" | "removed" | "absent"
}

func (o ModelActionOutput) FormatText(w io.Writer) error {
	switch o.Action {
	case "absent":
		fmt.Fprintf(w, "%s/%s is not cached.\n", o.Plugin, o.Model)
	case "removed":
		fmt.Fprintf(w, "✓ removed %s/%s (%s)\n", o.Plugin, o.Model, o.Dir)
	default: // "ready"
		fmt.Fprintf(w, "✓ %s/%s ready at %s\n", o.Plugin, o.Model, o.Dir)
	}
	return nil
}
