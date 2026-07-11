package cli

import (
	"errors"

	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

// newSedCmd builds the sed command. Used as the standalone `ksed` root and,
// via newToolboxProxies, behind the detached `kapi ksed` subcommand.
func newSedCmd(a *App) *cobra.Command {
	var (
		scripts   []string
		targetLoc string
	)

	cmd := &cobra.Command{
		Use:     "sed [flags] SCRIPT [FILE...]",
		Short:   "Stream-edit the text/content inside files (s/regexp/replacement/)",
		GroupID: "advanced",
		Long: `Apply sed-style substitutions to the human-readable text inside any supported
format, then write the document back in the same format. Only the editable text
changes — a .docx keeps its styles, a JSON catalog keeps its keys and shape.

SCRIPT is a substitution command: s/regexp/replacement/flags. Backreferences
(\1, &), and the g (global) and i (ignore-case) flags are supported. Any
single-byte delimiter works (s|a|b|). Pass several with repeated -e.

By default the edited document is written to standard output (like sed); use -i
to edit files in place, optionally keeping a backup (-i.bak). Edits apply to the
source text unless --target LOCALE selects a translation.

With no FILE, or when FILE is "-", standard input is read.`,
		Example: `  ksed 's/colour/color/g' guide.md
  ksed -i 's/Inc\./LLC/' *.docx
  ksed -i.bak -e 's/v1/v2/g' -e 's/beta//' locales/en.JSON
  ksed --target fr 's/Bonjour/Salut/g' messages.xliff`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scriptStrs := scripts
			files := args
			if len(scriptStrs) == 0 {
				if len(args) == 0 {
					return errors.New("no script given")
				}
				scriptStrs = []string{args[0]}
				files = args[1:]
			}
			prog, err := ParseSedProgram(scriptStrs)
			if err != nil {
				return err
			}

			inPlace := cmd.Flags().Changed("in-place")
			backupSuffix := ""
			if inPlace {
				if v, _ := cmd.Flags().GetString("in-place"); v != SentinelInPlace {
					backupSuffix = v
				}
			}

			loc := model.LocaleID(targetLoc)
			scopeSource := loc == ""
			t := NewSedTool(prog, loc, scopeSource)
			writeLocale := loc // "" for source round-trip

			return a.RunSed(cmd.Context(), files, t, writeLocale, inPlace, backupSuffix)
		},
	}

	f := cmd.Flags()
	f.StringArrayVarP(&scripts, "expression", "e", nil, "add a substitution script (repeatable; SCRIPT positional not needed)")
	f.StringVar(&targetLoc, "target", "", "edit the target translation for LOCALE instead of the source")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input/output format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input/output encoding")

	// -i takes an OPTIONAL backup suffix: `-i` (no backup) or `-i.bak`.
	f.StringP("in-place", "i", "", "edit files in place; append a backup SUFFIX if given (e.g. -i.bak)")
	f.Lookup("in-place").NoOptDefVal = SentinelInPlace

	return cmd
}
