package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// NewExtractCmd returns the `kapi extract` command (AD-017, issue #415).
// Emits one XLIFF 2.x (or PO) file per source → target-locale pair with
// content memory pre-fill, under .kapi/work/cache/extractions/<batch-id>/ bookkeeping.
func NewExtractCmd(a *App, _ ExtractCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "extract",
		Short:   "Emit a bilingual file for a translator: native .kpz or XLIFF/PO",
		GroupID: "advanced",
		Long: `Emit bilingual XLIFF 2.x (default) or PO files for each target locale
declared in a kapi project, pre-filled from the project's content memory.

Each invocation writes one batch of outputs under .kapi/work/cache/extractions/<batch-id>/
plus one bilingual file per source → target pair in --out-dir (default "out/").`,
		Example: `  kapi extract -p kapi.yaml --no-memory
  kapi extract -p kapi.yaml --target-lang fr
  kapi extract -p kapi.yaml --target-lang fr,de,es
  kapi extract src/*.json -o work.kpz --target-lang fr,qps   # ad-hoc .kpz workspace`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Ad-hoc workspace: `extract <sources> -o work.kpz` ingests source
			// documents into a portable .kpz workspace (no project needed).
			out, _ := cmd.Flags().GetString("output")
			format, _ := cmd.Flags().GetString("format")
			withSource, _ := cmd.Flags().GetBool("with-source")
			// Bilingual interchange .kpz: `extract --format kpz` emits a
			// task-scoped (kind=kapi-interchange) package per source→target
			// pair, over the project's content (extract has no -i flag; its
			// sources are the recipe's, or positional paths for the ad-hoc
			// workspace form below).
			if format == ExtractFormatKPZ {
				return a.RunExtractKpz(cmd)
			}
			if IsKpzPath(out) || len(args) > 0 {
				if !IsKpzPath(out) {
					return errors.New("extract: writing source files needs -o <work.kpz>")
				}
				tl, _ := cmd.Flags().GetString("target-lang")
				layout, _ := cmd.Flags().GetString("out")
				return a.ExtractToKpz(cmd.Context(), args, out, tl, layout, withSource)
			}
			return a.RunExtract(cmd)
		},
	}
	AddProjectFlag(cmd)
	cmd.Flags().StringP("output", "o", "", "write an ad-hoc .kpz workspace from the given source files")
	cmd.Flags().String("out", "", "merge-time output layout recorded in the .kpz (e.g. 'translated/{lang}/{name}.{ext}')")
	cmd.Flags().String("target-lang", "", "comma-separated target locales (default: all in recipe)")
	cmd.Flags().String("only", "", "restrict to a single content collection by name")
	cmd.Flags().String("pattern", "", "extra glob pattern restricting which source files to include")
	cmd.Flags().String("format", ExtractFormatXLIFF2, "bilingual output format (xliff2 | po | kpz)")
	cmd.Flags().String("xliff-version", "", "XLIFF 2.x version to emit (2.0, 2.1, 2.2; default 2.2)")
	cmd.Flags().Bool("no-memory", false, "skip content-memory pre-fill on extract")
	// Retired spelling: accepted so existing scripts keep working, hidden so
	// --help teaches only the current name.
	cmd.Flags().Bool("no-tm", false, "skip content-memory pre-fill on extract")
	_ = cmd.Flags().MarkHidden("no-tm")
	cmd.Flags().Bool("force", false, "re-extract every file, ignoring the incremental reuse of unchanged sources")
	cmd.Flags().Bool("with-source", false, "embed raw source bytes in the .kpz (default: identity + skeleton only)")
	cmd.Flags().String("out-dir", "out", "directory for emitted bilingual files (relative to project)")
	cmd.Flags().Bool("redact", false, "replace sensitive content with placeholders; originals stay in a local vault for merge")
	cmd.Flags().String("redact-rules", "", "path to a redaction rules YAML file (implies --redact)")
	AddProgressFlag(cmd)
	return cmd
}
