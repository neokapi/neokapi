package output

import (
	"fmt"
	"io"
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

	// The table narrows the long columns (source path, block hash) to whatever
	// the terminal can show; --json carries the full values.
	t := NewTable(w).Accent(1).Headers("TIMESTAMP", "SOURCE FILE", "BLOCK HASH", "XLIFF ORIGINAL")
	s := t.Styles()
	for _, r := range o.Entries {
		t.Row(s.Muted.Render(r.Timestamp), r.SourceFile, s.Dim(r.BlockHash), s.Dim(r.XLIFFOriginal))
	}
	t.Render()
	return nil
}
