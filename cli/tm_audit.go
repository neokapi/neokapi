package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/spf13/cobra"
)

// newTMAuditCmd returns `kapi tm audit`, which traces TM impact by a
// specific kapi merge batch id (AD-017, issue #418).
func (a *App) newTMAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Trace TM entries by merge batch id",
		Long: `Show every TM entry written (or updated) by a specific kapi merge batch,
so you can see what a particular return from a translator contributed to
the project TM.

Origin provenance is stamped by kapi merge (source="merge",
reference=<batch-id>, key=<source-rel>). Audit iterates the project TM,
surfaces only TUs with at least one matching Origin, and prints
source file, block hash, timestamp, and the originating XLIFF filename.

Examples:

  kapi tm audit --batch <batch-id>             # full listing
  kapi tm audit --batch <batch-id> --limit 50  # cap rows

Use "kapi tm stats" for global TM metrics (entry counts, per-locale
breakdown). Audit is narrow by design — it answers "what did this
merge do?".
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			batch, _ := cmd.Flags().GetString("batch")
			if batch == "" {
				return errors.New("audit: --batch <merge-batch-id> is required")
			}
			limit, _ := cmd.Flags().GetInt("limit")

			tm, dbPath, err := a.openTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			rows, err := collectAuditRows(cmd.Context(), tm, batch, limit)
			if err != nil {
				return err
			}

			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Timestamp.Equal(rows[j].Timestamp) {
					return rows[i].SourceFile < rows[j].SourceFile
				}
				return rows[i].Timestamp.After(rows[j].Timestamp)
			})

			out := output.TMAuditOutput{Batch: batch, DBPath: dbPath, Total: len(rows)}
			for _, r := range rows {
				out.Entries = append(out.Entries, output.TMAuditRow{
					Timestamp:     r.Timestamp.Format(time.RFC3339),
					SourceFile:    r.SourceFile,
					BlockHash:     r.BlockHash,
					XLIFFOriginal: r.XLIFFOriginal,
				})
			}
			return output.Print(cmd, out)
		},
	}

	cmd.Flags().String("batch", "", "kapi merge batch id to audit (required)")
	cmd.Flags().Int("limit", 0, "maximum rows to print (0 = all)")
	AddResourceFlags(cmd)
	return cmd
}

// auditRow represents one TM entry touched by a given merge batch.
type auditRow struct {
	EntryID       string
	SourceFile    string
	BlockHash     string
	XLIFFOriginal string
	Timestamp     time.Time
}

// collectAuditRows iterates the TM, keeping only entries with an Origin
// whose Source="merge" and Reference matches the given batch id. Results
// are capped at `limit` when > 0.
func collectAuditRows(ctx context.Context, tm sievepen.TMStore, batch string, limit int) ([]auditRow, error) {
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return strings.TrimRight(s[:max-1], " ") + "…"
}
