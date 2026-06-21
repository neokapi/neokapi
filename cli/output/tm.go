package output

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// TMAuditRow is one TM entry touched by a merge batch (kapi tm audit).
type TMAuditRow struct {
	Timestamp     string `json:"timestamp"`
	SourceFile    string `json:"source_file"`
	BlockHash     string `json:"block_hash,omitempty"`
	XLIFFOriginal string `json:"xliff_original,omitempty"`
}

// TMAuditOutput is the result of `kapi tm audit --batch <id>`.
type TMAuditOutput struct {
	Batch   string       `json:"batch"`
	DBPath  string       `json:"db_path"`
	Entries []TMAuditRow `json:"entries"`
	Total   int          `json:"total"`
}

func (o TMAuditOutput) FormatText(w io.Writer) error {
	if o.Total == 0 {
		fmt.Fprintf(w, "No TM entries found for batch %s (in %s)\n", o.Batch, o.DBPath)
		return nil
	}
	fmt.Fprintf(w, "Batch %s → %d TM entries (in %s)\n\n", o.Batch, o.Total, o.DBPath)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TIMESTAMP\tSOURCE FILE\tBLOCK HASH\tXLIFF ORIGINAL")
	for _, r := range o.Entries {
		// JSON carries full values; the text table truncates the long columns
		// for readability (alignment is handled by tabwriter).
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Timestamp, truncate(r.SourceFile, 40), truncate(r.BlockHash, 16), r.XLIFFOriginal)
	}
	return tw.Flush()
}
