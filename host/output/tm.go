package output

import (
	"fmt"
	"io"
)

// MemoryAuditRow is one content-memory entry touched by a merge batch (kapi memory audit).
type MemoryAuditRow struct {
	Timestamp     string `json:"timestamp"`
	SourceFile    string `json:"source_file"`
	BlockHash     string `json:"block_hash,omitempty"`
	XLIFFOriginal string `json:"xliff_original,omitempty"`
}

// MemoryAuditOutput is the result of `kapi memory audit --batch <id>`.
type MemoryAuditOutput struct {
	Batch   string           `json:"batch"`
	DBPath  string           `json:"db_path"`
	Entries []MemoryAuditRow `json:"entries"`
	Total   int              `json:"total"`
}

func (o MemoryAuditOutput) FormatText(w io.Writer) error {
	if o.Total == 0 {
		fmt.Fprintf(w, "No content-memory entries found for batch %s (in %s)\n", o.Batch, o.DBPath)
		return nil
	}
	fmt.Fprintf(w, "Batch %s → %d content-memory entries (in %s)\n\n", o.Batch, o.Total, o.DBPath)

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
