package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// NewExtractCmd returns the `kapi extract` command (AD-017, issue #415).
// Emits one XLIFF 2.x (or PO) file per source → target-locale pair with
// TM pre-fill, under .kapi/cache/extractions/<batch-id>/ bookkeeping.
func NewExtractCmd(a *App, _ ExtractCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "extract",
		Short:   "Emit a bilingual file for a translator — native .klz or XLIFF/PO",
		GroupID: "advanced",
		Long: `Emit bilingual XLIFF 2.x (default) or PO files for each target locale
declared in a .kapi project, pre-filled from the project's translation
memory.

Each invocation writes one batch of outputs under .kapi/cache/extractions/<batch-id>/
plus one bilingual file per source → target pair in --out-dir (default "out/").`,
		Example: `  kapi extract -p app.kapi --no-tm
  kapi extract -p app.kapi --target-lang fr
  kapi extract -p app.kapi --target-lang fr,de,es
  kapi extract src/*.json -o work.klz --target-lang fr,qps   # ad-hoc .klz workspace`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Ad-hoc workspace: `extract <sources> -o work.klz` ingests source
			// documents into a portable .klz workspace (no project needed).
			out, _ := cmd.Flags().GetString("output")
			format, _ := cmd.Flags().GetString("format")
			withSource, _ := cmd.Flags().GetBool("with-source")
			// Bilingual interchange .klz: `extract -i <src> --format klz` emits a
			// task-scoped (kind=kapi-interchange) package per source→target pair.
			if format == ExtractFormatKLZ {
				return a.RunExtractKlz(cmd)
			}
			if IsKlzPath(out) || len(args) > 0 {
				if !IsKlzPath(out) {
					return errors.New("extract: writing source files needs -o <work.klz>")
				}
				tl, _ := cmd.Flags().GetString("target-lang")
				layout, _ := cmd.Flags().GetString("out")
				return a.ExtractToKlz(cmd.Context(), args, out, tl, layout, withSource)
			}
			return a.RunExtract(cmd)
		},
	}
	AddProjectFlag(cmd)
	cmd.Flags().StringP("output", "o", "", "write an ad-hoc .klz workspace from the given source files")
	cmd.Flags().String("out", "", "merge-time output layout recorded in the .klz (e.g. 'l10n/{lang}/{name}.{ext}')")
	cmd.Flags().String("target-lang", "", "comma-separated target locales (default: all in recipe)")
	cmd.Flags().String("only", "", "restrict to a single content collection by name")
	cmd.Flags().String("pattern", "", "extra glob pattern restricting which source files to include")
	cmd.Flags().String("format", ExtractFormatXLIFF2, "bilingual output format (xliff2 | po | klz)")
	cmd.Flags().String("xliff-version", "", "XLIFF 2.x version to emit (2.0, 2.1, 2.2; default 2.2)")
	cmd.Flags().Bool("no-tm", false, "skip TM pre-fill on extract")
	cmd.Flags().Bool("force", false, "re-extract every file, ignoring the incremental reuse of unchanged sources")
	cmd.Flags().Bool("with-source", false, "embed raw source bytes in the .klz (default: identity + skeleton only)")
	cmd.Flags().String("out-dir", "out", "directory for emitted bilingual files (relative to project)")
	cmd.Flags().Bool("redact", false, "replace sensitive content with placeholders; originals stay in a local vault for merge")
	cmd.Flags().String("redact-rules", "", "path to a redaction rules YAML file (implies --redact)")
	return cmd
}
