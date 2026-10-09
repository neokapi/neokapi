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
		printOps    bool
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

A match is found in a block's text with its inline codes left out, so it may
span a link or bold text, and the codes around a change are kept. In a plural
or select, each branch is matched on its own, and a match never swallows one.
Each substitution that changes a block becomes a replace_text operation on it,
guarded by the revision ksed read. --print-ops prints that change set
(kapi.change/v1) instead of applying it, so 'ksed ... --print-ops | kapi apply'
applies exactly what was printed.

By default the edited document is written to standard output (like sed) and the
file stays as it is; use -i to edit files in place, optionally keeping a backup
(-i.bak). Edits apply to the source text unless --target LOCALE selects the
translation a bilingual file holds. An archive is edited all or nothing: when
the edit of one member is refused, no member is written.

Inside a kapi project (the one found from the working directory, or the one
KAPI_PROJECT names), a file of the project is read and written as kapi apply
reads it: named by its project-relative path, with the format the recipe binds,
and a translation's file as that translation of its source. Any other file is
read with the format detection finds, and a file no format claims is read as
plain text.

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
  ksed --target fr 's/Bonjour/Salut/g' messages.xliff
  ksed 's/shop/store/g' docs/guide.html --print-ops > change.json`,
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
			if inPlace && printOps {
				return errors.New("--print-ops prints the change set and edits nothing; it cannot be combined with -i/--in-place")
			}
			backupSuffix := ""
			if inPlace {
				backupSuffix = inPlaceFlag.Suffix
			}

			return a.RunSed(cmd.Context(), cmd, files, prog, SedOptions{
				Target:       model.LocaleID(targetLoc),
				InPlace:      inPlace,
				BackupSuffix: backupSuffix,
				Recursive:    recursive,
				Force:        force,
				PrintOps:     printOps,
			})
		},
	}

	f := cmd.Flags()
	f.StringArrayVarP(&scripts, "expression", "e", nil, "add a substitution script (repeatable; SCRIPT positional not needed)")
	f.BoolVarP(&recursive, "recursive", "R", false, "recurse into directory arguments (-R, not -r: sed's -r is --regexp-extended)")
	f.StringVar(&targetLoc, "target", "", "edit the translation for LOCALE a bilingual file holds instead of the source")
	f.BoolVar(&force, "force", false, "write an edited binary document (.docx, .idml, …) to the terminal anyway")
	f.BoolVar(&printOps, "print-ops", false, "print the change set the substitutions compile to (kapi.change/v1) and change nothing")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input/output format (default: what the recipe binds, else auto-detect by extension/content)")
	a.AddEngineFlag(f)
	a.AddSourceLangFlag(f)
	a.AddEncodingFlag(f, "", "input/output encoding")

	// -i takes an OPTIONAL backup suffix: `-i` (no backup) or `-i.bak`.
	inPlaceFlag = RegisterInPlace(f, "edit files in place; append a backup SUFFIX if given (e.g. -i.bak)")

	return cmd
}
