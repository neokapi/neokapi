package cli

import (
	"errors"

	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

// newGrepCmd builds the grep command with the full classic option surface.
// It is used as the standalone `kgrep` root and, via newToolboxProxies, behind
// the detached `kapi kgrep` subcommand — never as a plain child of the kapi root
// (which would shadow -v/-c with kapi's global flags).
func newGrepCmd(a *App) *cobra.Command {
	var (
		ignoreCase   bool
		invert       bool
		count        bool
		number       bool
		onlyMatching bool
		filesWith    bool
		filesWithout bool
		wordRegexp   bool
		fixedStrings bool
		recursive    bool
		withFilename bool
		noFilename   bool
		patterns     []string
		targetLoc    string
	)

	cmd := &cobra.Command{
		Use:     "grep [flags] PATTERN [FILE...]",
		Short:   "Search the text/content inside files for a pattern",
		GroupID: "advanced",
		Long: `Search the human-readable text inside any supported format for a regular
expression: the prose of a Word .docx, the values of a JSON catalog, the
segments of an XLIFF file, skipping markup and structure. Output mirrors grep:
one matching block per line, optionally prefixed with the file name and the
block number.

With no FILE, or when FILE is "-", standard input is read. Exit status is 0 if
any block matched, 1 if none did, 2 on error.`,
		Example: `  kgrep "Tervetuloa" report.docx
  kgrep -i todo locales/*.JSON
  kgrep -r --target fr "déconnexion" ./content
  kgrep -c "©" *.md`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Pattern comes from -e flags, or the first positional argument.
			pats := patterns
			files := args
			if len(pats) == 0 {
				if len(args) == 0 {
					return errors.New("no pattern given")
				}
				pats = []string{args[0]}
				files = args[1:]
			}
			m, err := NewMatcher(pats, MatcherOpts{
				IgnoreCase:   ignoreCase,
				WordRegexp:   wordRegexp,
				FixedStrings: fixedStrings,
				Invert:       invert,
			})
			if err != nil {
				return err
			}
			colorMode, _ := cmd.Flags().GetString("color")
			jsonOut, _ := cmd.Flags().GetBool("json")
			quiet, _ := cmd.Flags().GetBool("quiet")
			return a.RunGrep(cmd.Context(), files, m, GrepOptions{
				Count:        count,
				Number:       number,
				OnlyMatching: onlyMatching,
				FilesWith:    filesWith,
				FilesWithout: filesWithout,
				WithFilename: withFilename,
				NoFilename:   noFilename,
				Recursive:    recursive,
				Quiet:        quiet,
				JSON:         jsonOut,
				Color:        UseColor(colorMode),
				TargetLocale: model.LocaleID(targetLoc),
			})
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&ignoreCase, "ignore-case", "i", false, "case-insensitive matching")
	f.BoolVarP(&number, "line-number", "n", false, "prefix each match with its block number")
	f.BoolVarP(&onlyMatching, "only-matching", "o", false, "print only the matched text, not the whole block")
	f.BoolVarP(&filesWith, "files-with-matches", "l", false, "print only the names of files containing matches")
	f.BoolVarP(&filesWithout, "files-without-match", "L", false, "print only the names of files containing no match")
	f.BoolVarP(&wordRegexp, "word-regexp", "w", false, "match only whole words")
	f.BoolVarP(&fixedStrings, "fixed-strings", "F", false, "treat the pattern as a literal string, not a regexp")
	f.BoolVarP(&recursive, "recursive", "r", false, "recurse into directory arguments")
	f.BoolVarP(&withFilename, "with-filename", "H", false, "print the file name for each match")
	f.BoolVar(&noFilename, "no-filename", false, "suppress file-name prefixes on output")
	f.StringArrayVarP(&patterns, "regexp", "e", nil, "pattern to search for (repeatable; PATTERN positional not needed)")
	f.StringVar(&targetLoc, "target", "", "search the target translation for LOCALE instead of the source")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	a.AddSourceLangFlag(f)
	a.AddEncodingFlag(f, "", "input encoding")

	// Full classic shorthand surface — kapi's persistent flags are never inherited
	// (busybox root, or detached proxy), so -v/-c/-q are ours to define.
	f.BoolVarP(&invert, "invert-match", "v", false, "select blocks that do NOT match")
	f.BoolVarP(&count, "count", "c", false, "print a count of matching blocks per file")
	f.BoolP("quiet", "q", false, "suppress all output; exit status only")
	f.String("color", "auto", "highlight matches: auto, always, never")
	f.Bool("json", false, "emit matches as JSON")
	return cmd
}
