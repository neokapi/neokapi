package cli

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

// newDiffCmd builds the diff command. Used as the standalone `kdiff` root and,
// via NewToolboxProxies, behind the detached `kapi diff` subcommand.
func newDiffCmd(a *App) *cobra.Command {
	var (
		by        string
		targetLoc string
		brief     bool
		stat      bool
	)

	cmd := &cobra.Command{
		Use:     "diff [flags] FILE_A [FILE_B]",
		Short:   "Compare the text/content of files block by block",
		GroupID: "advanced",
		Long: `Compare the human-readable text inside any supported format, block by block,
rather than byte by byte. A reflowed Word .docx, a re-zipped container or a
reordered JSON catalog do not register as a diff, and only the prose that actually
changed does.

Two modes:

  Revision diff (two files)     what translatable content changed between two
                                versions of a document.
  Coverage diff (one file +     compare a target translation against the source
  --target LOCALE)              within one file: which blocks are untranslated or
                                are still a verbatim copy of the source.

Alignment is chosen automatically: keyed formats (JSON, XLIFF, PO, … with stable
block IDs) align by ID, so added / removed / changed / reordered keys are
reported directly; prose formats align by the block text. Force either with
--by id or --by content.

Exit status is 0 when the inputs are equivalent, 1 when they differ, 2 on error.`,
		Example: `  kdiff old.json new.JSON
  kdiff report.docx report-v2.docx
  kdiff --target fr messages.xliff            # coverage: source vs French
  kdiff --target fr old.xliff new.xliff       # what changed in the French
  kdiff --by content draft.md final.md
  kdiff --json a.JSON b.JSON`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch by {
			case "", "auto", "id", "content":
			default:
				return fmt.Errorf("invalid --by %q: use auto, id, or content", by)
			}
			if by == "" {
				by = "auto"
			}
			colorMode, _ := cmd.Flags().GetString("color")
			jsonOut, _ := cmd.Flags().GetBool("json")
			return a.RunDiff(cmd.Context(), args, DiffOptions{
				By:           by,
				TargetLocale: model.LocaleID(targetLoc),
				Brief:        brief,
				Stat:         stat,
				JSON:         jsonOut,
				Color:        UseColor(colorMode),
			})
		},
	}

	f := cmd.Flags()
	f.StringVar(&by, "by", "auto", "alignment strategy: auto, id, or content")
	f.StringVar(&targetLoc, "target", "", "compare the target translation for LOCALE (one file: a coverage report)")
	f.BoolVarP(&brief, "brief", "q", false, "report only whether the inputs differ, not the changes")
	f.BoolVar(&stat, "stat", false, "print a one-line summary of the changes before the diff")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	a.AddSourceLangFlag(f)
	a.AddEncodingFlag(f, "", "input encoding")
	f.String("color", "auto", "colorize the diff: auto, always, never")
	f.Bool("json", false, "emit the diff as JSON")
	return cmd
}
