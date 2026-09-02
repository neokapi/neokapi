package cli

import (
	"github.com/neokapi/neokapi/kpz"
	"github.com/spf13/cobra"
)

// NewMergeCmd returns the `kapi merge` command (AD-017, issue #416).
// Applies a translator-returned XLIFF back onto the project's source
// files using the captured skeleton, records stale segments, and
// absorbs accepted targets into the project content memory.
func NewMergeCmd(a *App, _ MergeCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "merge",
		Short:   "Apply a returned bilingual file (.kpz/XLIFF/PO) back onto the project source",
		GroupID: "advanced",
		Long: `Materialize target-language files for a project, or apply bilingual files
returned by a translator back onto the project's source locales.

With no -i in a project, merge writes the target-language files from the
project block store: a process-only "kapi run" (in a project, no -o)
commits its work as targets/<locale> overlays, and merge is the matching
sink: it reads each source, applies the stored overlays, and writes the
target-language file via the source format's skeleton round-trip.

With -i, merge applies one or more bilingual files returned by a
translator back onto the project's source locales, using the skeleton
captured by kapi extract. Each input carries the extraction
batch id in a file-level <note>, so merge finds the right extraction
manifest without guessing from the filename. Mixed target locales in one
batch are fine, and merge handles each input independently.`,
		Example: `  kapi merge                     # materialize target-language files from the project store
  kapi merge -i out/app.en-to-fr.xliff
  kapi merge -i file1.xliff -i file2.xliff
  kapi merge -i vendor-return/ --no-memory-update
  kapi merge work.kpz -o out/    # emit target-language files from a .kpz workspace`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// A single .kpz positional arg is either a bilingual interchange
			// file (kind=kapi-interchange — ingest the translator's targets) or
			// an ad-hoc workspace (emit its target-language files).
			if len(args) == 1 && IsKpzPath(args[0]) {
				out, _ := cmd.Flags().GetString("output")
				if pkg, err := LoadWorkspace(args[0]); err == nil && pkg.Kind == kpz.KindInterchange {
					return a.MergeOneKpz(cmd, args[0])
				}
				return a.MergeFromKpz(cmd.Context(), args[0], out)
			}
			// In a project with no -i input, materialize the target-language files
			// from the project block store: a process-only `kapi run` lands its
			// work as `targets/<locale>` overlays, and this is the matching sink
			// (AD-026 §3). Explicit -i keeps the XLIFF/PO/dir merge path below.
			if inputs, _ := cmd.Flags().GetStringArray("input"); len(inputs) == 0 {
				return a.MergeFromProjectStore(cmd)
			}
			return a.RunMerge(cmd)
		},
	}
	AddProjectFlag(cmd)
	cmd.Flags().StringArrayP("input", "i", nil, "input XLIFF file, glob, or directory (repeatable)")
	cmd.Flags().StringP("output", "o", "", "output directory or template when merging a .kpz workspace")
	cmd.Flags().Bool("no-memory-update", false, "skip content-memory write-back")
	// Retired spelling: accepted so existing scripts keep working, hidden so
	// --help teaches only the current name.
	cmd.Flags().Bool("no-tm-update", false, "skip content-memory write-back")
	_ = cmd.Flags().MarkHidden("no-tm-update")
	cmd.Flags().Bool("no-restore", false, "skip restoring redacted originals from the batch vault")
	AddProgressFlag(cmd)
	return cmd
}
