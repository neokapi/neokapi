package host

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/sievepen"
)

// auditRow represents one TM entry touched by a given merge batch.
type auditRow struct {
	EntryID       string
	SourceFile    string
	BlockHash     string
	XLIFFOriginal string
	Timestamp     time.Time
}

// CollectAuditRows iterates the TM, keeping only entries with an Origin
// whose Source="merge" and Reference matches the given batch id. Results
// are capped at `limit` when > 0.
func CollectAuditRows(ctx context.Context, tm sievepen.TMStore, batch string, limit int) ([]auditRow, error) {
	var rows []auditRow
	entries, err := tm.Entries(ctx)
	if err != nil {
		return nil, fmt.Errorf("read TM entries: %w", err)
	}
	for _, entry := range entries {
		matched := false
		var origin sievepen.Origin
		for _, o := range entry.Origins {
			if o.Source == "merge" && o.Reference == batch {
				matched = true
				origin = o
				break
			}
		}
		if !matched {
			continue
		}
		row := auditRow{
			EntryID:    entry.ID,
			SourceFile: origin.Key,
			Timestamp:  origin.AddedAt,
		}
		if entry.Properties != nil {
			row.BlockHash = entry.Properties["kapi-merge:block-content-hash"]
			row.XLIFFOriginal = entry.Properties["kapi-merge:xliff-original"]
		}
		rows = append(rows, row)
		if limit > 0 && len(rows) >= limit {
			break
		}
	}
	return rows, nil
}

func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return strings.TrimRight(s[:max-1], " ") + "…"
}
