package cli

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

// newCatCmd builds the cat command. Used as the standalone `kcat` root and,
// via newToolboxProxies, behind the detached `kapi kcat` subcommand.
func newCatCmd(a *App) *cobra.Command {
	var (
		number    bool
		showIDs   bool
		targetLoc string
		recursive bool
	)

	cmd := &cobra.Command{
		Use:     "cat [flags] [FILE...]",
		Short:   "Print the text/content inside files, block by block",
		GroupID: "advanced",
		Long: `Print the human-readable text extracted from each file, one block per line,
regardless of the underlying format. A Word .docx, a JSON catalog and an XLIFF
file all print as their plain prose, with the markup and structure stripped.

With no FILE, or when FILE is "-", standard input is read.`,
		Example: `  kcat report.docx
  kcat -n locales/en.JSON
  kcat --target fr messages.xliff
  cat raw.txt | kcat -f plaintext`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.RunCat(cmd.Context(), cmd, args, CatOptions{
				Number:       number,
				ShowIDs:      showIDs,
				TargetLocale: model.LocaleID(targetLoc),
				Recursive:    recursive,
			})
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&number, "number", "n", false, "number the output blocks")
	f.BoolVar(&showIDs, "id", false, "prefix each block with its source ID")
	f.BoolVarP(&recursive, "recursive", "r", false, "recurse into directory arguments")
	f.StringVar(&targetLoc, "target", "", "print the target translation for LOCALE instead of the source")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input encoding")
	f.Bool("json", false, "emit blocks as JSON instead of plain text")
	return cmd
}
