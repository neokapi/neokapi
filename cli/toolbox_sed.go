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
		scripts     []string
		targetLoc   string
		recursive   bool
		force       bool
		inPlaceFlag *InPlaceFlag
	)

	cmd := &cobra.Command{
		Use:     "sed [flags] SCRIPT [FILE...]",
		Short:   "Stream-edit the text/content inside files (s/regexp/replacement/)",
		GroupID: "advanced",
		Long: `Apply sed-style substitutions to the human-readable text inside any supported
format, then write the document back in the same format. Only the editable text
changes: a .docx keeps its styles, a JSON catalog keeps its keys and shape.

SCRIPT is a substitution command: s/regexp/replacement/flags. Backreferences
(\1, &), and the g (global) and i (ignore-case) flags are supported. Any
single-byte delimiter works (s|a|b|). Pass several with repeated -e.

By default the edited document is written to standard output (like sed); use -i
to edit files in place, optionally keeping a backup (-i.bak). Edits apply to the
source text unless --target LOCALE selects a translation.

Editing a binary document (.docx, .idml, .epub, …) writes a binary document, so
when standard output is a terminal that is refused rather than streamed at it.
Use -i, redirect stdout, or pass --force. Redirected or piped output is never
touched.

Directory arguments are walked with -R. It is spelled -R rather than -r because
sed's own -r means --regexp-extended, and quietly repurposing it would rewrite a
whole tree for someone who only asked for a different regexp dialect.

With no FILE, or when FILE is "-", standard input is read.`,
		Example: `  ksed 's/colour/color/g' guide.md
  ksed -i 's/Inc\./LLC/' *.docx
  ksed -R -i 's/colour/color/g' docs
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
				backupSuffix = inPlaceFlag.Suffix
			}

			loc := model.LocaleID(targetLoc)
			scopeSource := loc == ""
			t := NewSedTool(prog, loc, scopeSource)
			writeLocale := loc // "" for source round-trip

			return a.RunSed(cmd.Context(), files, t, SedOptions{
				WriteLocale:  writeLocale,
				InPlace:      inPlace,
				BackupSuffix: backupSuffix,
				Recursive:    recursive,
				Force:        force,
			})
		},
	}

	f := cmd.Flags()
	f.StringArrayVarP(&scripts, "expression", "e", nil, "add a substitution script (repeatable; SCRIPT positional not needed)")
	f.BoolVarP(&recursive, "recursive", "R", false, "recurse into directory arguments (-R, not -r: sed's -r is --regexp-extended)")
	f.StringVar(&targetLoc, "target", "", "edit the target translation for LOCALE instead of the source")
	f.BoolVar(&force, "force", false, "write an edited binary document (.docx, .idml, …) to the terminal anyway")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input/output format (default: auto-detect by extension/content)")
	a.AddSourceLangFlag(f)
	a.AddEncodingFlag(f, "", "input/output encoding")

	// -i takes an OPTIONAL backup suffix: `-i` (no backup) or `-i.bak`.
	inPlaceFlag = RegisterInPlace(f, "edit files in place; append a backup SUFFIX if given (e.g. -i.bak)")

	return cmd
}
