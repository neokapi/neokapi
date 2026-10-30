package cli

import (
	"github.com/spf13/cobra"
)

// NewFixCmd creates `kapi fix`: it runs the check `kapi check` runs and
// applies every fix the check computed through the change service `kapi
// apply` uses, so each format keeps its formatting. The assembly is
// host.FixFiles.
func NewFixCmd(a *App) *cobra.Command {
	var opts FixOptions
	cmd := &cobra.Command{
		Use:     "fix [files...]",
		Short:   "Apply the fixes kapi can make on its own",
		GroupID: "work",
		Args:    cobra.ArbitraryArgs,
		Long: `Run the checks 'kapi check' runs and apply the fixes they computed: the
replacement that each failing finding of a name, spelling or word rule names.
Each file's fixes are one change set, applied through the change service 'kapi
apply' uses, so the file keeps its formatting and every byte outside the
replaced words stays as it is. kapi fix previews each change set first, and a
file the change service cannot write without changing other bytes is not
written; its fixes are listed as not fixed.

Some findings have no automatic fix: a finding about tone or style, a rule
with no replacement, words inside a code span, and words that hold a link, a
formatting span or a placeholder, which the replacement would delete. kapi
fix lists them as needing a rewrite. A finding that only reports stays as it
is. A fix the change service refuses, such as one over words another fix also
changes, is listed with the reason, and the file's other fixes land.

With no file arguments inside a project, kapi fix checks the project's declared
content; naming files narrows it to those. After writing, it checks the files
it wrote again and prints how many failing findings remain. The voice,
profile and terms flags select what the check holds the content to, as they
do for 'kapi check'. kapi fix takes no --ship: the project gate mode of 'kapi
check' computes no fixes, so run 'kapi check --ship' after kapi fix.

--dry-run prints the change each file would get, as 'kapi apply --dry-run'
prints it, and writes nothing. --json prints the result (kapi.fix-result/v1):
the change set result of each file, the findings not fixed and the findings
that need a rewrite.

Exit codes: 0 when every fix landed, 1 when a fix did not, 4 when the check
did not run.`,
		Example: `  kapi fix
  kapi fix docs/guide.md
  kapi fix 'docs/**/*.md' --dry-run
  kapi fix --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Paths = args
			return a.RunFix(cmd, opts)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&opts.DryRun, "dry-run", false, "print the change each file would get and write nothing")
	f.String("profile", "", "voice profile name from the local store")
	f.String("profile-file", "", "path to a voice profile YAML")
	f.String("pack", "", "built-in voice starter pack")
	f.Bool("voice", false, "also run the voice/style-similarity check (needs the kapi-check plugin and a profile with examples); its findings have no automatic fix")
	f.Float64("voice-min", DefaultVoiceSimilarity, "voice-similarity cutoff (cosine, 0-1) below which a block is flagged off-voice")
	f.String("termstore", "", "named terms or terms-store path the checks read (defaults to the project terms store)")
	a.AddSourceLangFlag(f)
	AddProjectFlag(cmd)
	return cmd
}
