package output

import (
	"fmt"
	"io"
)

// AddEntry is a single pattern processed by `kapi add`.
type AddEntry struct {
	Pattern string `json:"pattern"`
	Format  string `json:"format,omitempty"`
	Files   int    `json:"files"`
	Skipped bool   `json:"skipped,omitempty"`
}

// AddOutput is the result of `kapi add`.
type AddOutput struct {
	Added []AddEntry `json:"added"`
}

// FormatText renders the add result as human-readable lines.
func (o AddOutput) FormatText(w io.Writer) error {
	for _, e := range o.Added {
		switch {
		case e.Skipped:
			fmt.Fprintf(w, "Already tracked: %s\n", e.Pattern)
		case e.Format != "":
			fmt.Fprintf(w, "Added %s (%s) — %d file(s)\n", e.Pattern, e.Format, e.Files)
		default:
			fmt.Fprintf(w, "Added %s — %d file(s)\n", e.Pattern, e.Files)
		}
	}
	return nil
}

// RmEntry is a single pattern processed by `kapi rm`.
type RmEntry struct {
	Pattern string `json:"pattern"`
	Action  string `json:"action"`           // "removed", "excluded", "already_excluded"
	Format  string `json:"format,omitempty"` // only for "removed"
	Files   int    `json:"files,omitempty"`  // only for "excluded"
}

// RmOutput is the result of `kapi rm`.
type RmOutput struct {
	Entries []RmEntry `json:"entries"`
}

// FormatText renders the rm result as human-readable lines.
func (o RmOutput) FormatText(w io.Writer) error {
	for _, e := range o.Entries {
		switch e.Action {
		case "removed":
			if e.Format != "" {
				fmt.Fprintf(w, "Removed %s (was: %s)\n", e.Pattern, e.Format)
			} else {
				fmt.Fprintf(w, "Removed %s\n", e.Pattern)
			}
		case "excluded":
			fmt.Fprintf(w, "Excluded %s — %d file(s) now excluded\n", e.Pattern, e.Files)
		case "already_excluded":
			fmt.Fprintf(w, "Already excluded: %s\n", e.Pattern)
		}
	}
	return nil
}
