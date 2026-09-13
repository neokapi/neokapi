package cli

import (
	"strconv"

	"github.com/spf13/cobra"
)

// NewCheckCmd creates `kapi check`: a content-first verifier. It runs a bundle
// of source-side content checks over any file — no translation needed — and
// returns one stable, machine-consumable Report (pass, score, gate, and a
// located finding per rule) the way a test runner reports, so an AI assistant or
// CI can read the findings, fix the exact block, and re-run until it passes.
//
//	kapi check guide.md                              # default content checkset
//	kapi check 'web/**/*.md'                         # glob, expanded in-process
//	kapi check api.json --max-chars 60 --forbid TODO # length + forbidden-pattern
//	kapi check post.md --pack marketing-blog         # + voice vocabulary
//	kapi check api.json --target api.de.json --target-lang de  # + bilingual checks
//
// The checks are content-level (the translatable units). Document-level
// structure and encoding validity is a format-reader concern, surfaced on
// demand with --validate (Reader Validation-Mode): off by default, the readers
// extract leniently; report folds located structure.*/encoding.* findings into
// the Report; strict also gates on them.
func NewCheckCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "check [files...]",
		Short:   "Verify content against a checkset and gate on severity, like tests over code",
		GroupID: "work",
		Args:    cobra.ArbitraryArgs,
		Long: `Run content checks over one or more files and return structured findings
plus a verdict (passed, failed, or did not run), gating on severity: the
content-first counterpart to a test runner.

The default checkset is source-side and needs no translation: text hygiene
(empty, doubled spaces/words, stray whitespace), length limits (--max-chars/
--max-words), forbidden/required patterns (--forbid/--require), and brand
vocabulary when a profile is bound (--profile/--pack/--profile-file).

Bilingual checks (do-not-translate, placeholder integrity) are an
opt-in: pass --target <file> --target-lang <lang> to check a translated target
against its source.

Each finding carries a stable rule id (<check>.<category>) and a block location,
so an assistant can fix the exact block and track rules across iterations. Output
is a human table by default; --output-format json|yaml emits the kapi.check/v1
Report. Configuration warnings, such as an unknown key in a voice profile, follow
the verdict and fill the report's warnings array. They name configuration to fix
and never change the score, the gate or the exit code.

Positional paths accept glob patterns and directories, expanded by kapi itself.
Quote the pattern and ` + "`**`" + ` recurses identically in every shell. Inside a .kapi
project, check with no file arguments checks the project's declared content;
naming files narrows it to those.

Project gate mode (--ship): it runs the project's bound quality gates (voice,
terminology, rule-based checks) plus its ship/source coverage gates over the
project's content, and exits non-zero when any gate is unmet: the pre-release
bar. Target drift never blocks an ordinary build (see 'kapi status'); --ship is
the explicit, opt-in enforcement point. With no file arguments it checks the
project's source-only content and declared source/target pairs. Named source
files receive content checks; named targets retain their source pairing.

Diff scope (--diff-file, --diff-against): check only the content blocks a
unified diff touches, each block whole, and report the lines each spans. Every
file the diff names is listed with what became of it: checked, untouched, out of
scope (inside a project, content the recipe does not declare), deleted, or did
not run (a changed file whose blocks cannot be located). --diff-file reads a
diff from a file, or from standard input with -. --diff-against runs git diff
against a revision, read-only, and treats untracked files as added. Named files
narrow the scope. The loop for an agent: edit, run
'kapi check --diff-against HEAD', repair, run it again.

Exit codes: 0 pass, 3 when the gate fails, 4 when the check did not run, 1
operational. A check did not run when it examined no content, or when one of its
checks reported nothing on the known-bad sample it is given beside the content.
--no-fail exits 0 when the gate fails (report mode for a fix loop); neither it
nor --lenient turns a check that did not run into a pass.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.RunCheck(cmd, args)
		},
	}
	f := cmd.Flags()
	f.String("target", "", "translated target file to check against the (single) source, enabling bilingual checks")
	f.String("target-lang", "", "locale of the --target file (e.g. de)")
	f.StringSlice("dnt", nil, "do-not-translate terms that must survive verbatim into the target (with --target)")
	f.Int("max-chars", 0, "flag content longer than this many characters (0 = off)")
	f.Int("max-words", 0, "flag content with more than this many words (0 = off)")
	f.StringSlice("forbid", nil, "regex that must NOT appear in the content (repeatable)")
	f.StringSlice("require", nil, "regex that MUST appear in the content (repeatable)")
	f.String("profile", "", "voice profile name from the local store")
	f.String("profile-file", "", "path to a voice profile YAML")
	f.String("pack", "", "built-in brand starter pack")
	f.Int("max-critical", 0, "fail if critical findings exceed this count")
	f.Int("max-major", -1, "fail if major findings exceed this count (-1 = no limit)")
	f.Int("max-minor", -1, "fail if minor findings exceed this count (-1 = no limit)")
	f.Int("min-score", 0, "fail if the roll-up score is below this (0 = no score gate); with --ship: the brand-gate compliance threshold (default "+strconv.Itoa(DefaultVoiceMinScore)+")")
	f.Bool("strict", false, "strict gate: fail on any critical or major finding")
	f.Bool("lenient", false, "report only: never fail the gate (still prints findings)")
	f.Bool("no-fail", false, "exit 0 even when the gate fails (fix-loop mode)")
	f.Bool("voice", false, "also run the voice/style-similarity check (needs the kapi-check plugin and a profile with examples)")
	f.Float64("voice-min", DefaultVoiceSimilarity, "voice-similarity cutoff (cosine, 0-1) below which a block is flagged off-voice")
	f.String("diff-file", "", "check only the content blocks this unified diff touches (a file, or - for standard input)")
	f.String("diff-against", "", "check only the content blocks changed since this git revision, including untracked files (runs git diff read-only)")
	f.String("validate", "off", "reader structure/encoding validation: off|report|strict (report folds structure.*/encoding.* findings into the Report; strict also fails the gate on a Major+ structure/encoding problem)")
	a.AddSourceLangFlag(f)

	// Project gate mode (--ship): the project gates surface.
	AddProjectFlag(cmd)
	f.Bool("ship", false, "project gate mode: run the project's bound gates (voice, terminology, rule-based checks) plus its ship/source coverage gates; exit non-zero when unmet: the pre-release bar")
	AddGateFlag(cmd)
	f.String("locale", "", "with --ship: scope the target-side gates to a single target locale (e.g. fr)")
	f.String("termstore", "", "with --ship: named terms or terms-store path for the terminology gate (defaults to the project terms store)")

	cmd.MarkFlagsMutuallyExclusive("strict", "lenient")
	cmd.MarkFlagsMutuallyExclusive("ship", "target")
	cmd.MarkFlagsMutuallyExclusive("diff-file", "diff-against")
	cmd.MarkFlagsMutuallyExclusive("diff-file", "ship")
	cmd.MarkFlagsMutuallyExclusive("diff-against", "ship")
	cmd.MarkFlagsMutuallyExclusive("diff-file", "target")
	cmd.MarkFlagsMutuallyExclusive("diff-against", "target")
	return cmd
}
