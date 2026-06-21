package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/spf13/cobra"
)

// checkReport wraps the canonical, platform-agnostic core/check.Report so the
// CLI can render it as a human table while --json emits the Report verbatim (the
// embedded struct's fields are promoted, so the JSON IS the Report). It is the
// unit an AI assistant or CI reads, fixes, and re-runs against — like a test
// runner's report.
type checkReport struct {
	check.Report
}

// FormatText renders the report as a human-readable summary.
func (r checkReport) FormatText(w io.Writer) error {
	for _, d := range r.Findings {
		loc := d.Location.Block
		if d.Location.File != "" {
			loc = d.Location.File + ":" + loc
		}
		fmt.Fprintf(w, "  %-8s %-28s %s  %s\n", strings.ToUpper(string(d.Severity)), d.Rule, loc, d.Message)
		if d.Suggestion != "" {
			fmt.Fprintf(w, "           ↳ %s\n", d.Suggestion)
		}
	}
	if len(r.Findings) == 0 {
		fmt.Fprintln(w, "  No findings.")
	}
	fmt.Fprintln(w)
	verdict := "PASS"
	if !r.Pass {
		verdict = "FAIL"
	}
	fmt.Fprintf(w, "%s — score %d/100 · %d finding(s) (%d critical, %d major, %d minor)\n",
		verdict, r.Summary.Score, r.Summary.Findings, r.Summary.Critical, r.Summary.Major, r.Summary.Minor)
	for _, reason := range r.Gate.Failed {
		fmt.Fprintf(w, "  gate: %s\n", reason)
	}
	return nil
}

// NewCheckCmd creates `kapi check`: a content-first verifier. It runs a bundle
// of source-side content checks over any file — no translation needed — and
// returns one stable, machine-consumable Report (pass, score, gate, and a
// located finding per rule) the way a test runner reports, so an AI assistant or
// CI can read the findings, fix the exact block, and re-run until it passes.
//
//	kapi check guide.md                              # default content checkset
//	kapi check api.json --max-chars 60 --forbid TODO # length + forbidden-pattern
//	kapi check post.md --pack marketing-blog         # + brand vocabulary
//	kapi check api.json --target api.de.json --target-lang de  # + bilingual (l10n) checks
//
// The checks are content-level (the translatable units). Document-level
// structure and encoding validity is a format-reader concern, surfaced on
// demand with --validate (Reader Validation-Mode): off by default, the readers
// extract leniently; report folds located structure.*/encoding.* findings into
// the Report; strict also gates on them.
func (a *App) NewCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "check [files...]",
		Short:   "Verify content against a checkset and gate on severity, like tests over code",
		GroupID: "quality",
		Args:    cobra.ArbitraryArgs,
		Long: `Run content checks over one or more files and return structured findings
plus a pass/fail, gating on severity — the content-first counterpart to a test
runner.

The default checkset is source-side and needs no translation: text hygiene
(empty, doubled spaces/words, stray whitespace), length limits (--max-chars/
--max-words), forbidden/required patterns (--forbid/--require), and brand
vocabulary when a profile is bound (--profile/--pack/--profile-file).

Bilingual localization checks (do-not-translate, placeholder integrity) are an
opt-in: pass --target <file> --target-lang <lang> to check a translated target
against its source.

Each finding carries a stable rule id (<check>.<category>) and a block location,
so an assistant can fix the exact block and track rules across iterations. Output
is a human table by default; --json emits the kapi.check/v1 Report.

Exit codes: 0 pass, 3 when the gate fails, 1 operational. --no-fail always exits
0 (report mode) for a fix-loop.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runCheck(cmd, args)
		},
	}
	f := cmd.Flags()
	f.String("target", "", "translated target file to check against the (single) source — enables bilingual l10n checks")
	f.String("target-lang", "", "locale of the --target file (e.g. de)")
	f.StringSlice("dnt", nil, "do-not-translate terms that must survive verbatim into the target (with --target)")
	f.Int("max-chars", 0, "flag content longer than this many characters (0 = off)")
	f.Int("max-words", 0, "flag content with more than this many words (0 = off)")
	f.StringSlice("forbid", nil, "regex that must NOT appear in the content (repeatable)")
	f.StringSlice("require", nil, "regex that MUST appear in the content (repeatable)")
	f.String("profile", "", "brand profile name from the local store")
	f.String("profile-file", "", "path to a brand profile YAML")
	f.String("pack", "", "built-in brand starter pack")
	f.Int("max-critical", 0, "fail if critical findings exceed this count")
	f.Int("max-major", -1, "fail if major findings exceed this count (-1 = no limit)")
	f.Int("max-minor", -1, "fail if minor findings exceed this count (-1 = no limit)")
	f.Int("min-score", 0, "fail if the roll-up score is below this (0 = no score gate)")
	f.Bool("strict", false, "strict gate: fail on any critical or major finding")
	f.Bool("lenient", false, "report only: never fail the gate (still prints findings)")
	f.Bool("no-fail", false, "exit 0 even when the gate fails (fix-loop mode)")
	f.Bool("voice", false, "also run the voice/style-similarity check (needs the kapi-check plugin and a profile with examples)")
	f.Float64("voice-min", DefaultVoiceSimilarity, "voice-similarity cutoff (cosine, 0-1) below which a block is flagged off-voice")
	f.String("validate", "off", "reader structure/encoding validation: off|report|strict (report folds structure.*/encoding.* findings into the Report; strict also fails the gate on a Major+ structure/encoding problem)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	cmd.MarkFlagsMutuallyExclusive("strict", "lenient")
	return cmd
}

func (a *App) runCheck(cmd *cobra.Command, args []string) error {
	report, err := a.computeCheck(cmd, args)
	if err != nil {
		return err
	}
	if err := output.Print(cmd, checkReport{report}); err != nil {
		return err
	}
	if !report.Pass {
		if noFail, _ := cmd.Flags().GetBool("no-fail"); noFail {
			return nil
		}
		return ErrQualityGate
	}
	return nil
}

// computeCheck runs the configured checkset over the input file(s) and assembles
// the canonical Report. It is shared by the CLI and the MCP check tools so a CI
// gate and an assistant loop read byte-identical reports.
func (a *App) computeCheck(cmd *cobra.Command, args []string) (check.Report, error) {
	a.InitRegistries()
	ctx := cmdContext(cmd)

	targetFile, _ := cmd.Flags().GetString("target")
	if len(args) == 0 {
		return check.Report{}, errors.New("at least one file is required")
	}

	profile, err := a.resolveCheckProfile(cmd)
	if err != nil {
		return check.Report{}, err
	}

	validateMode, err := validateModeFromFlag(cmd)
	if err != nil {
		return check.Report{}, err
	}

	opts := checkRunOptions{profile: profile}
	opts.maxChars, _ = cmd.Flags().GetInt("max-chars")
	opts.maxWords, _ = cmd.Flags().GetInt("max-words")
	opts.forbid, _ = cmd.Flags().GetStringSlice("forbid")
	opts.require, _ = cmd.Flags().GetStringSlice("require")
	opts.voice, _ = cmd.Flags().GetBool("voice")
	opts.voiceMin, _ = cmd.Flags().GetFloat64("voice-min")

	var diags []check.Diagnostic
	totalBlocks := 0
	target := check.Target{Kind: "file"}

	if targetFile != "" {
		// Bilingual l10n mode (opt-in): a single source + its translated target.
		if len(args) != 1 {
			return check.Report{}, errors.New("--target checks one source file; pass exactly one positional file")
		}
		targetLang, _ := cmd.Flags().GetString("target-lang")
		if targetLang == "" {
			targetLang = "und"
		}
		dnt, _ := cmd.Flags().GetStringSlice("dnt")
		sourcePath := args[0]
		unit := verifyUnit{sourcePath: sourcePath, targetPath: targetFile, locale: targetLang, displayPath: targetFile}
		blocks, missing, berr := a.bilingualBlocks(ctx, unit)
		if berr != nil {
			return check.Report{}, berr
		}
		if missing {
			return check.Report{}, fmt.Errorf("target file %q does not exist", targetFile)
		}
		totalBlocks = len(blocks)
		target.File = sourcePath
		fileDiags, ferr := a.collectFileDiagnostics(ctx, blocks, sourcePath, opts)
		if ferr != nil {
			return check.Report{}, ferr
		}
		diags = append(diags, fileDiags...)
		diags = append(diags, a.collectBilingualDiagnostics(ctx, blocks, sourcePath, model.LocaleID(targetLang), dnt)...)
	} else {
		// Content-first generic mode: each positional file is a source, checked
		// independently. With --validate (Reader Validation-Mode) the format
		// reader's structure/encoding diagnostics fold into the same Report; off
		// (the default) keeps the lenient read where a malformed file is an
		// operational error.
		for _, file := range args {
			blocks, fileDiags, ferr := a.checkFileBlocks(ctx, file, validateMode, opts)
			if ferr != nil {
				return check.Report{}, ferr
			}
			totalBlocks += len(blocks)
			diags = append(diags, fileDiags...)
		}
		if len(args) == 1 {
			target.File = args[0]
		} else {
			target.File = fmt.Sprintf("%d files", len(args))
		}
	}
	target.Blocks = totalBlocks

	gate := gateFromFlags(cmd)
	report := check.BuildReport(target, diags, gate)
	if validateMode == format.ValidationStrict {
		applyStrictValidationGate(&report)
	}
	return report, nil
}

// checkFileBlocks reads one file's blocks and the content checkset diagnostics,
// folding in the reader's structure/encoding diagnostics when validateMode is on.
func (a *App) checkFileBlocks(ctx context.Context, file string, validateMode format.ValidationMode, opts checkRunOptions) ([]*model.Block, []check.Diagnostic, error) {
	var blocks []*model.Block
	var diags []check.Diagnostic

	if validateMode != format.ValidationOff {
		bl, fdiags, rerr := a.readBlocksValidated(ctx, file, a.SourceLang, validateMode)
		if rerr != nil {
			return nil, nil, rerr
		}
		blocks = bl
		for _, fd := range fdiags {
			diags = append(diags, check.DiagnosticFromReader(fd, displayName(file)))
		}
	} else {
		bl, rerr := a.readBlocks(ctx, file, a.SourceLang)
		if rerr != nil {
			// A read failure is operational in off mode: the lenient readers
			// extract from imperfect inputs, so a hard error means the file
			// could not be parsed at all. Pass --validate report to fold the
			// structure problem into the Report instead.
			return nil, nil, rerr
		}
		blocks = bl
	}

	fileDiags, ferr := a.collectFileDiagnostics(ctx, blocks, file, opts)
	if ferr != nil {
		return nil, nil, ferr
	}
	diags = append(diags, fileDiags...)
	return blocks, diags, nil
}

// validateModeFromFlag parses the --validate flag into a ValidationMode.
func validateModeFromFlag(cmd *cobra.Command) (format.ValidationMode, error) {
	v, _ := cmd.Flags().GetString("validate")
	return parseValidationMode(v)
}

// parseValidationMode maps an off|report|strict string (empty = off) to a
// ValidationMode. Shared by the CLI flag and the MCP check_file tool.
func parseValidationMode(v string) (format.ValidationMode, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "off":
		return format.ValidationOff, nil
	case "report":
		return format.ValidationReport, nil
	case "strict":
		return format.ValidationStrict, nil
	default:
		return format.ValidationOff, fmt.Errorf("invalid validate mode %q: want off, report, or strict", v)
	}
}

// applyStrictValidationGate tightens the report for --validate strict: any
// structure or encoding diagnostic of Major severity or worse fails the gate
// regardless of the severity-count thresholds — a structurally broken or
// mis-encoded document can't pass. A relabeled-charset mismatch is Minor and
// does not trip it. This is a check-layer gate policy, not a reader concern.
func applyStrictValidationGate(report *check.Report) {
	for _, f := range report.Findings {
		if f.Check != "structure" && f.Check != "encoding" {
			continue
		}
		if f.Severity != check.SeverityMajor && f.Severity != check.SeverityCritical {
			continue
		}
		report.Pass = false
		report.Gate.Failed = append(report.Gate.Failed,
			fmt.Sprintf("validation: %s is a blocking %s problem", f.Rule, f.Check))
	}
}

// checkRunOptions carries the resolved generic-check configuration.
type checkRunOptions struct {
	profile  *brand.VoiceProfile
	maxChars int
	maxWords int
	forbid   []string
	require  []string
	voice    bool
	voiceMin float64
}

// collectFileDiagnostics runs the source-side content checkset over one file's
// blocks and returns family-attributed, located diagnostics. Each checker family
// runs in turn; the new findings it adds to the unified annotation are tagged
// with the family and the block location.
func (a *App) collectFileDiagnostics(ctx context.Context, blocks []*model.Block, file string, opts checkRunOptions) ([]check.Diagnostic, error) {
	var diags []check.Diagnostic
	seen := make([]int, len(blocks)) // per-block count of findings already mapped

	// Hygiene — always on, no configuration needed.
	a.runFamily(ctx, blocks, coretools.NewContentLintTool(&coretools.ContentLintConfig{}))
	diags = append(diags, mapBlockDeltas(blocks, seen, "hygiene", file)...)

	// Length — only when a limit is set.
	if opts.maxChars > 0 || opts.maxWords > 0 {
		cfg := &coretools.LengthCheckConfig{CheckSource: true, MaxChars: opts.maxChars, MaxWords: opts.maxWords}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		a.runFamily(ctx, blocks, coretools.NewLengthCheckTool(cfg))
		diags = append(diags, mapBlockDeltas(blocks, seen, "length", file)...)
	}

	// Pattern — forbidden (must-not-match) and required (must-match).
	if rules := patternRules(opts.forbid, opts.require); len(rules) > 0 {
		cfg := &coretools.PatternCheckConfig{CheckSource: true, Patterns: rules}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		a.runFamily(ctx, blocks, coretools.NewPatternCheckTool(cfg))
		diags = append(diags, mapBlockDeltas(blocks, seen, "pattern", file)...)
	}

	// Brand vocabulary — separate annotation; runs when a profile is bound.
	if opts.profile != nil {
		vocab := coretools.NewBrandVocabCheckTool(opts.profile, nil)
		for _, b := range blocks {
			runCheckTool(ctx, vocab, b)
			if ann, ok := model.AnnoAs[*brand.BrandVoiceAnnotation](b, "brand-voice"); ok {
				loc := check.Location{File: displayName(file), Block: blockKey(b)}
				for _, f := range ann.Findings {
					diags = append(diags, check.DiagnosticFrom(f, "brand", loc))
				}
			}
		}
	}

	// Voice/style similarity (opt-in, --voice): drives the kapi-check plugin.
	if opts.voice {
		refs := voiceExamples(opts.profile)
		if len(refs) == 0 {
			return nil, errors.New("--voice needs a brand profile with examples (--profile/--pack/--profile-file)")
		}
		t, closeT, derr := dialVoicePlugin(ctx)
		if derr != nil {
			return nil, derr
		}
		defer closeT()
		vf, verr := voiceSimilarityFindings(blocks, refs, t, opts.voiceMin)
		if verr != nil {
			return nil, fmt.Errorf("voice check: %w", verr)
		}
		for _, f := range vf {
			diags = append(diags, check.DiagnosticFrom(f, "voice", check.Location{File: displayName(file)}))
		}
	}

	return diags, nil
}

// collectBilingualDiagnostics runs the target-gated localization checks
// (placeholder integrity, do-not-translate) over a source/target block set.
func (a *App) collectBilingualDiagnostics(ctx context.Context, blocks []*model.Block, file string, loc model.LocaleID, dntTerms []string) []check.Diagnostic {
	var diags []check.Diagnostic
	// Seed the per-block delta counts from the findings already on each block:
	// in bilingual mode the source checks (collectFileDiagnostics) ran first, so
	// starting at zero would re-attribute their findings to the placeholder
	// family.
	seen := make([]int, len(blocks))
	for i, b := range blocks {
		seen[i] = len(findingsFromBlock(b))
	}

	a.runFamily(ctx, blocks, coretools.NewPlaceholderCheckTool(coretools.NewPlaceholderCheckConfig(loc)))
	diags = append(diags, mapBlockDeltas(blocks, seen, "placeholder", file)...)

	if len(dntTerms) > 0 {
		dntCfg := coretools.NewDNTCheckConfig(loc)
		dntCfg.Terms = dntTerms
		a.runFamily(ctx, blocks, coretools.NewDNTCheckTool(dntCfg))
		diags = append(diags, mapBlockDeltas(blocks, seen, "dnt", file)...)
	}
	return diags
}

// runFamily runs one checker family's tool(s) over every block. Findings
// accumulate on each block's unified annotation; the caller reads the delta.
func (a *App) runFamily(ctx context.Context, blocks []*model.Block, tools ...blockProcessor) {
	for _, b := range blocks {
		for _, t := range tools {
			runCheckTool(ctx, t, b)
		}
	}
}

// blockProcessor is the minimal interface a check tool satisfies to run over a
// single block via runCheckTool.
type blockProcessor interface {
	Process(context.Context, <-chan *model.Part, chan<- *model.Part) error
}

// mapBlockDeltas maps the findings each block gained since the last family (the
// slice past seen[i]) into located, family-tagged diagnostics, then advances the
// per-block seen count.
func mapBlockDeltas(blocks []*model.Block, seen []int, family, file string) []check.Diagnostic {
	var out []check.Diagnostic
	for i, b := range blocks {
		all := findingsFromBlock(b)
		for _, f := range all[seen[i]:] {
			out = append(out, check.DiagnosticFrom(f, family, check.Location{File: displayName(file), Block: blockKey(b)}))
		}
		seen[i] = len(all)
	}
	return out
}

// patternRules builds forbidden (must-not-match) and required (must-match)
// pattern rules from the --forbid / --require flag values.
func patternRules(forbid, require []string) []coretools.PatternRule {
	var rules []coretools.PatternRule
	for i, p := range forbid {
		rules = append(rules, coretools.PatternRule{Name: fmt.Sprintf("forbidden-%d", i+1), Pattern: p, MustNotMatch: true})
	}
	for i, p := range require {
		rules = append(rules, coretools.PatternRule{Name: fmt.Sprintf("required-%d", i+1), Pattern: p, MustMatch: true})
	}
	return rules
}

// gateFromFlags builds the severity/score gate from the command flags, applying
// the --strict / --lenient presets.
func gateFromFlags(cmd *cobra.Command) check.Gate {
	g := check.Gate{}
	g.MaxCritical, _ = cmd.Flags().GetInt("max-critical")
	g.MaxMajor, _ = cmd.Flags().GetInt("max-major")
	g.MaxMinor, _ = cmd.Flags().GetInt("max-minor")
	g.MinScore, _ = cmd.Flags().GetInt("min-score")
	if strict, _ := cmd.Flags().GetBool("strict"); strict {
		g.MaxCritical = 0
		g.MaxMajor = 0
	}
	if lenient, _ := cmd.Flags().GetBool("lenient"); lenient {
		// All limits off: report only, the gate never trips.
		g = check.Gate{MaxCritical: -1, MaxMajor: -1, MaxMinor: -1, MinScore: 0}
	}
	return g
}

// findingsFromBlock reads the unified check annotation off a block.
func findingsFromBlock(b *model.Block) []check.Finding {
	if ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey); ok {
		return ann.Findings
	}
	return nil
}

func (a *App) resolveCheckProfile(cmd *cobra.Command) (*brand.VoiceProfile, error) {
	name, _ := cmd.Flags().GetString("profile")
	file, _ := cmd.Flags().GetString("profile-file")
	pack, _ := cmd.Flags().GetString("pack")
	if name == "" && file == "" && pack == "" {
		return nil, nil // a brand profile is optional for `kapi check`
	}
	p, _, err := a.resolveBrandProfile(cmd)
	return p, err
}
