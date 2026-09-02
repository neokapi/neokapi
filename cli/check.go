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
plus a pass/fail, gating on severity — the content-first counterpart to a test
runner.

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
Report.

Positional paths accept glob patterns and directories, expanded by kapi itself —
quote the pattern and ` + "`**`" + ` recurses identically in every shell. Inside a .kapi
project, check with no file arguments checks the project's declared content;
naming files narrows it to those.

Project gate mode (--ship): it runs the project's bound quality gates (voice,
terminology, rule-based checks) plus its ship/source coverage gates over the
project's content, and exits non-zero when any gate is unmet — the pre-release
bar. Target drift never blocks an ordinary build (see 'kapi status'); --ship is
the explicit, opt-in enforcement point. With no file arguments it inspects the
project's content x target languages; pass files to gate just those.

Exit codes: 0 pass, 3 when the gate fails, 1 operational. --no-fail always exits
0 (report mode) for a fix-loop.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.RunCheck(cmd, args)
		},
	}
	f := cmd.Flags()
	f.String("target", "", "translated target file to check against the (single) source — enables bilingual checks")
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
	f.String("validate", "off", "reader structure/encoding validation: off|report|strict (report folds structure.*/encoding.* findings into the Report; strict also fails the gate on a Major+ structure/encoding problem)")
	a.AddSourceLangFlag(f)

	// Project gate mode (--ship): the project gates surface.
	AddProjectFlag(cmd)
	f.Bool("ship", false, "project gate mode: run the project's bound gates (voice, terminology, rule-based checks) plus its ship/source coverage gates; exit non-zero when unmet — the pre-release bar")
	AddGateFlag(cmd)
	f.String("locale", "", "with --ship: scope the target-side gates to a single target locale (e.g. fr)")
	f.String("termstore", "", "with --ship: named terms or terms-store path for the terminology gate (defaults to the project terms store)")

	cmd.MarkFlagsMutuallyExclusive("strict", "lenient")
	cmd.MarkFlagsMutuallyExclusive("ship", "target")
	return cmd
}
