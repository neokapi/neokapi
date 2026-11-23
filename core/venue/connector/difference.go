package connector

import (
	"fmt"
	"io"
)

// FormatDifferences writes each record difference for a reader: whose
// version differs, who changed the server's and when, each differing field
// on both sides, and what the pull did or what to run next.
func FormatDifferences(w io.Writer, ds []RecordDifference) {
	for _, d := range ds {
		fmt.Fprintf(w, "The %s %q on the server differs from this project's", d.Kind, d.Name)
		switch {
		case d.ChangedBy != "" && !d.ChangedAt.IsZero():
			fmt.Fprintf(w, " (changed by %s at %s)", d.ChangedBy, d.ChangedAt.UTC().Format("2006-01-02 15:04 UTC"))
		case d.ChangedBy != "":
			fmt.Fprintf(w, " (changed by %s)", d.ChangedBy)
		}
		fmt.Fprintln(w, ":")
		for _, f := range d.Fields {
			fmt.Fprintf(w, "  %s\n    server: %s\n    yours:  %s\n", f.Field, f.Server, f.Local)
		}
		switch d.Outcome {
		case DifferenceTaken:
			fmt.Fprintf(w, "Took the server's version of the %s into this project, recorded on its context record.\n", d.Kind)
		case DifferenceKept:
			fmt.Fprintf(w, "Kept yours: the next `kapi push` replaces the server's version of the %s.\n", d.Kind)
		default:
			fmt.Fprintf(w, "The next `kapi push` that changes the %s is refused until you choose: "+
				"`kapi pull --keep-local` to replace the server's version with yours on that push, "+
				"or `kapi pull --take-server` to take the server's version.\n", d.Kind)
		}
	}
}
