package cli

import (
	"errors"
	"sort"
	"time"

	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// newMemoryAuditCmd returns `kapi memory audit`, which traces content memory impact by a
// specific kapi merge batch id (AD-017, issue #418).
func newMemoryAuditCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Trace content-memory entries by merge batch id",
		Long: `Show every content-memory entry written (or updated) by a specific kapi merge batch,
so you can see what a particular return from a translator contributed to
the project content memory.

Origin provenance is stamped by kapi merge (source="merge",
reference=<batch-id>, key=<source-rel>). Audit iterates the project content memory,
surfaces only TUs with at least one matching Origin, and prints
source file, block hash, timestamp, and the originating XLIFF filename.

Examples:

  kapi memory audit --batch <batch-id>             # full listing
  kapi memory audit --batch <batch-id> --limit 50  # cap rows

Use "kapi memory stats" for global content-memory metrics (entry counts, per-locale
breakdown). Audit is narrow by design: it answers "what did this
merge do?".
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			batch, _ := cmd.Flags().GetString("batch")
			if batch == "" {
				return errors.New("audit: --batch <merge-batch-id> is required")
			}
			limit, _ := cmd.Flags().GetInt("limit")

			tm, dbPath, releaseMemory, err := a.OpenMemorySQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseMemory()

			rows, err := CollectAuditRows(cmd.Context(), tm, batch, limit)
			if err != nil {
				return err
			}

			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Timestamp.Equal(rows[j].Timestamp) {
					return rows[i].SourceFile < rows[j].SourceFile
				}
				return rows[i].Timestamp.After(rows[j].Timestamp)
			})

			out := output.MemoryAuditOutput{Batch: batch, DBPath: dbPath, Total: len(rows)}
			for _, r := range rows {
				out.Entries = append(out.Entries, output.MemoryAuditRow{
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
