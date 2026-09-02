package output

import (
	"fmt"
	"io"
	"strconv"
)

// AddEntry is a single pattern processed by `kapi add`.
type AddEntry struct {
	Pattern string `json:"pattern"`
	Format  string `json:"format,omitempty"`
	Target  string `json:"target,omitempty"`
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
		target := ""
		if e.Target != "" {
			target = " → " + e.Target
		}
		switch {
		case e.Skipped:
			fmt.Fprintf(w, "Already tracked: %s\n", e.Pattern)
		case e.Format != "":
			fmt.Fprintf(w, "Added %s (%s)%s: %d file(s)\n", e.Pattern, e.Format, target, e.Files)
		default:
			fmt.Fprintf(w, "Added %s%s: %d file(s)\n", e.Pattern, target, e.Files)
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
	// Dirty is the count of blocks changed locally since the last sync,
	// merged from the connected server's plumbing. nil when the project
	// binds no convergence venue (or the plumbing is unavailable).
	Dirty *int `json:"dirty,omitempty"`
}

// syncCell renders one entry's sync column: pending push count or "synced".
func syncCell(s *Styles, d *int) string {
	switch {
	case d == nil:
		return s.Dim("")
	case *d == 0:
		return s.Success.Render("synced")
	default:
		return s.Warn.Render(fmt.Sprintf("%d to push", *d))
	}
}

// LsOutput is the result of `kapi ls`.
type LsOutput struct {
	Files    []LsEntry `json:"files"`
	Total    int       `json:"total"`
	Blocks   int       `json:"blocks,omitempty"`
	Words    int       `json:"words,omitempty"`
	HasStats bool      `json:"-"`
	// HasSync marks that per-file sync standing was merged in from the
	// connected server's plumbing (kapi ls's sync column).
	HasSync bool `json:"-"`
	// Untracked inverts the listing: the files are the ones NO collection
	// tracks (`kapi ls --untracked`). It travels in the JSON document because
	// the two listings carry the same entry shape and a reader holding one of
	// them cannot otherwise tell which question it answers.
	Untracked bool `json:"untracked,omitempty"`
}

// FormatText renders the file list, with block/word columns when --stats is set.
func (o LsOutput) FormatText(w io.Writer) error {
	if len(o.Files) == 0 {
		if o.Untracked {
			fmt.Fprintln(w, "Every readable file is tracked by a collection.")
			return nil
		}
		fmt.Fprintln(w, "No tracked files.")
		return nil
	}

	headers := []string{"PATH", "FORMAT"}
	if o.HasStats {
		headers = append(headers, "BLOCKS", "WORDS")
	}
	if o.HasSync {
		headers = append(headers, "SYNC")
	}

	t := NewTable(w).Accent(0).Headers(headers...)
	s := t.Styles()
	for _, f := range o.Files {
		cells := []string{f.Path, s.Muted.Render(f.Format)}
		if o.HasStats {
			cells = append(cells, strconv.Itoa(f.Blocks), strconv.Itoa(f.Words))
		}
		if o.HasSync {
			cells = append(cells, syncCell(s, f.Dirty))
		}
		t.Row(cells...)
	}
	t.Render()

	fmt.Fprintln(w)
	switch {
	case o.Untracked:
		Note(w, "%d file(s) tracked by no collection", o.Total)
	case o.HasStats:
		Note(w, "%d file(s), %d blocks, %d words", o.Total, o.Blocks, o.Words)
	default:
		Note(w, "%d file(s)", o.Total)
	}
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
			fmt.Fprintf(w, "Excluded %s: %d file(s) now excluded\n", e.Pattern, e.Files)
		case "already_excluded":
			fmt.Fprintf(w, "Already excluded: %s\n", e.Pattern)
		}
	}
	return nil
}
