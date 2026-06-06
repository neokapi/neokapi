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

// LsEntry is one file tracked by the project's content.
type LsEntry struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Blocks int    `json:"blocks,omitempty"` // only with --stats
	Words  int    `json:"words,omitempty"`  // only with --stats
}

// LsOutput is the result of `kapi ls`.
type LsOutput struct {
	Files    []LsEntry `json:"files"`
	Total    int       `json:"total"`
	Blocks   int       `json:"blocks,omitempty"`
	Words    int       `json:"words,omitempty"`
	HasStats bool      `json:"-"`
}

// FormatText renders the file list, with block/word columns when --stats is set.
func (o LsOutput) FormatText(w io.Writer) error {
	if len(o.Files) == 0 {
		fmt.Fprintln(w, "No tracked files.")
		return nil
	}
	pathW := 4 // "PATH"
	for _, f := range o.Files {
		if len(f.Path) > pathW {
			pathW = len(f.Path)
		}
	}
	pathW += 2

	if !o.HasStats {
		for _, f := range o.Files {
			fmt.Fprintf(w, "%-*s %s\n", pathW, f.Path, f.Format)
		}
		fmt.Fprintf(w, "\n%d file(s)\n", o.Total)
		return nil
	}

	fmtW := 6 // "FORMAT"
	for _, f := range o.Files {
		if len(f.Format) > fmtW {
			fmtW = len(f.Format)
		}
	}
	fmtW += 2
	fmt.Fprintf(w, "  %-*s %-*s %8s %8s\n", pathW, "PATH", fmtW, "FORMAT", "BLOCKS", "WORDS")
	fmt.Fprintf(w, "  %-*s %-*s %8s %8s\n", pathW, "----", fmtW, "------", "------", "-----")
	for _, f := range o.Files {
		fmt.Fprintf(w, "  %-*s %-*s %8d %8d\n", pathW, f.Path, fmtW, f.Format, f.Blocks, f.Words)
	}
	fmt.Fprintf(w, "\n%d file(s), %d blocks, %d words\n", o.Total, o.Blocks, o.Words)
	return nil
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
