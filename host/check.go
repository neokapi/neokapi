package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/host/output"
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
	renderFindingsTable(w, r.Findings)
	s := output.NewTable(w).Styles()
	verdict := s.Success.Render("PASS")
	if !r.Pass {
		verdict = s.Error.Render("FAIL")
	}
	fmt.Fprintf(w, "%s — ", verdict)
	writeFindingsCounts(w, r.Summary)
	for _, reason := range r.Gate.Failed {
		fmt.Fprintf(w, "  gate: %s\n", reason)
	}
	return nil
}

// writeFindingsCounts writes the score + severity roll-up line shared by
// `kapi check` (after its PASS/FAIL verdict) and a `kapi exec <check>` run.
func writeFindingsCounts(w io.Writer, s check.Summary) {
	fmt.Fprintf(w, "score %d/100 · %d finding(s) (%d critical, %d major, %d minor)\n",
		s.Score, s.Findings, s.Critical, s.Major, s.Minor)
}

// renderFindingsTable writes the severity/rule/location/message table shared by
// `kapi check` and the findings a `kapi exec <check>` run reports, so the two
// read identically — an assistant that learned one has learned the other.
func renderFindingsTable(w io.Writer, diags []check.Diagnostic) {
	t := output.NewTable(w).Accent(1).Headers("severity", "rule", "location", "message")
	s := t.Styles()
	for _, d := range diags {
		loc := d.Location.Block
		if d.Location.File != "" {
			loc = d.Location.File + ":" + loc
		}
		t.Row(severityCell(s, string(d.Severity)), d.Rule, s.Dim(loc), d.Message)
		if d.Suggestion != "" {
			t.Row("", "", "", s.Muted.Render("↳ "+d.Suggestion))
		}
	}
	t.Render()
	if len(diags) == 0 {
		fmt.Fprintln(w, "  No findings.")
	}
	fmt.Fprintln(w)
}

func (a *App) RunCheck(cmd Command, args []string) error {
	if ship, _ := cmd.Flags().GetBool("ship"); ship {
		return a.runShipCheck(cmd, args)
	}
	report, err := a.ComputeCheck(cmd, args)
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

// checkProjectSources resolves what a bare `kapi check` checks: every source
// file the project declares as content. It is the same resolution the ship gate
// and the voice gate use (projectSourceFiles), so "check my content" means the
// same set of files whichever bar you hold it to.
//
// Outside a project there is nothing to expand to, so the file requirement
// stands — and says how to satisfy it.
func (a *App) checkProjectSources(cmd Command) ([]string, error) {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil {
		return nil, err
	}
	if projectPath == "" {
		return nil, errors.New("at least one file is required — or run inside a kapi project to check its declared content")
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	files, err := a.projectSourceFiles(proj, filepath.Dir(projectPath))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s declares no content to check — add some with `kapi add <glob>`", DisplayName(projectPath))
	}
	// The content resolver returns absolute paths; findings should read the way
	// the user would type them, like the ones from a named file do.
	if cwd, cerr := os.Getwd(); cerr == nil {
		for i, f := range files {
			if rel, rerr := filepath.Rel(cwd, f); rerr == nil && !strings.HasPrefix(rel, "..") {
				files[i] = rel
			}
		}
	}
	return files, nil
}

// runShipCheck is `kapi check --ship`: the project gate mode that absorbed the
// retired `kapi verify` (#1078 C1) — kapi retires spellings outright rather than
// carrying aliases, so this is now the only way in. It routes through the shared
// verify engine (RunVerify/computeVerify), which the Stop hook also drives, so a
// release gate and the hook evaluate a project identically. Flag defaults that
// differ between the file checkset and the project gates are mapped here: an
// untouched --min-score means the voice-gate threshold (DefaultVoiceMinScore),
// and an untouched --source-lang defers to the project's source_language.
func (a *App) runShipCheck(cmd Command, args []string) error {
	if !cmd.Flags().Changed("min-score") {
		_ = cmd.Flags().Set("min-score", strconv.Itoa(DefaultVoiceMinScore))
	}
	if !cmd.Flags().Changed("source-lang") {
		// computeVerify treats "" as "use the project's source_language"; the
		// check flag's static default ("en") would otherwise override it.
		_ = cmd.Flags().Set("source-lang", "")
	}
	return a.RunVerify(cmd, args)
}

// ComputeCheck runs the configured checkset over the input file(s) and assembles
// the canonical Report. It is shared by the CLI and the MCP check tools so a CI
// gate and an assistant loop read byte-identical reports.
func (a *App) ComputeCheck(cmd Command, args []string) (check.Report, error) {
	a.InitRegistries()
	ctx := CmdContext(cmd)

	targetFile, _ := cmd.Flags().GetString("target")
	if len(args) == 0 {
		// Bare `kapi check` inside a project checks the project: a project-aware
		// command given nothing to narrow it to works on the whole project (as
		// `up`, `status`, and `check --ship` do). Named files still win — they
		// narrow the check to exactly what you named.
		//
		// --target is bilingual mode: it pairs ONE source with ONE translated
		// target, so there is nothing to expand to and the file requirement
		// stands.
		if targetFile != "" {
			return check.Report{}, errors.New("--target checks one source file; pass exactly one positional file")
		}
		files, perr := a.checkProjectSources(cmd)
		if perr != nil {
			return check.Report{}, perr
		}
		args = files
	} else if targetFile == "" {
		// Named inputs expand like every other content verb: globs resolve
		// in-process (so `kapi check 'web/**/*.md'` works in any shell) and a
		// directory means the files beneath it. --target is excluded: it pairs
		// exactly one source with one target, so there is nothing to expand.
		expanded, eerr := a.ResolveInputs(cmd, args, InputOptions{
			Command:  "kapi check",
			Fallback: FallbackNone,
			OnSkip: func(path string, serr error) {
				fmt.Fprintf(cmd.ErrOrStderr(), "kapi check: %s: %v\n", path, serr)
			},
		})
		if eerr != nil {
			return check.Report{}, eerr
		}
		if len(expanded) == 0 {
			return check.Report{}, fmt.Errorf("no files matched %s", strings.Join(args, ", "))
		}
		args = expanded
	}

	voice, err := a.newCheckVoice(cmd)
	if err != nil {
		return check.Report{}, err
	}

	validateMode, err := validateModeFromFlag(cmd)
	if err != nil {
		return check.Report{}, err
	}

	opts := checkRunOptions{}
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
		if opts.profile, err = voice.forFile(ctx, sourcePath); err != nil {
			return check.Report{}, err
		}
		unit := VerifyUnit{SourcePath: sourcePath, TargetPath: targetFile, Locale: targetLang, DisplayPath: targetFile}
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
		biDiags, berr := a.collectBilingualDiagnostics(ctx, blocks, sourcePath, model.LocaleID(targetLang), dnt)
		if berr != nil {
			return check.Report{}, berr
		}
		diags = append(diags, biDiags...)
	} else {
		// Content-first generic mode: each positional file is a source, checked
		// independently. With --validate (Reader Validation-Mode) the format
		// reader's structure/encoding diagnostics fold into the same Report; off
		// (the default) keeps the lenient read where a malformed file is an
		// operational error.
		prog := a.NewProgress(cmd, "checking", len(args))
		defer prog.Done()
		for _, file := range args {
			prog.Step(DisplayName(file))
			// The voice is resolved per file: a run over a governed project
			// checks each file against the vocabulary in force where that file
			// sits, not against one voice picked for the whole invocation.
			if opts.profile, err = voice.forFile(ctx, file); err != nil {
				return check.Report{}, err
			}
			blocks, fileDiags, ferr := a.checkFileBlocks(ctx, file, validateMode, opts)
			prog.Advance()
			if ferr != nil {
				return check.Report{}, ferr
			}
			totalBlocks += len(blocks)
			diags = append(diags, fileDiags...)
		}
		prog.Done()
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
		bl, fdiags, rerr := a.readBlocksValidated(ctx, file, a.FormatFlag, a.SourceLang, validateMode)
		if rerr != nil {
			return nil, nil, rerr
		}
		blocks = bl
		for _, fd := range fdiags {
			diags = append(diags, check.DiagnosticFromReader(fd, DisplayName(file)))
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
func validateModeFromFlag(cmd Command) (format.ValidationMode, error) {
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
	profile  *profile.VoiceProfile
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
	if err := a.runFamily(ctx, blocks, check.NewContentLintTool()); err != nil {
		return nil, fmt.Errorf("hygiene check %s: %w", DisplayName(file), err)
	}
	diags = append(diags, mapBlockDeltas(blocks, seen, "hygiene", file)...)

	// Length — only when a limit is set.
	if opts.maxChars > 0 || opts.maxWords > 0 {
		lengthTool, err := check.NewSourceLengthTool(opts.maxChars, opts.maxWords)
		if err != nil {
			return nil, err
		}
		if err := a.runFamily(ctx, blocks, lengthTool); err != nil {
			return nil, fmt.Errorf("length check %s: %w", DisplayName(file), err)
		}
		diags = append(diags, mapBlockDeltas(blocks, seen, "length", file)...)
	}

	// Pattern — forbidden (must-not-match) and required (must-match).
	if rules := patternRules(opts.forbid, opts.require); len(rules) > 0 {
		patternTool, err := check.NewSourcePatternTool(rules)
		if err != nil {
			return nil, err
		}
		if err := a.runFamily(ctx, blocks, patternTool); err != nil {
			return nil, fmt.Errorf("pattern check %s: %w", DisplayName(file), err)
		}
		diags = append(diags, mapBlockDeltas(blocks, seen, "pattern", file)...)
	}

	// Brand vocabulary — separate annotation; runs when a profile is bound.
	if opts.profile != nil {
		vocab := coretools.NewVoiceVocabCheckTool(opts.profile, nil)
		for _, b := range blocks {
			if err := RunCheckTool(ctx, vocab, b); err != nil {
				return nil, fmt.Errorf("voice vocabulary check %s: %w", DisplayName(file), err)
			}
			if ann, ok := model.AnnoAs[*profile.VoiceAnnotation](b, "voice"); ok {
				loc := check.Location{File: DisplayName(file), Block: blockKey(b)}
				for _, f := range ann.Findings {
					diags = append(diags, check.DiagnosticFrom(f, "voice", loc))
				}
			}
		}
	}

	// Voice/style similarity (opt-in, --voice): drives the kapi-check plugin.
	if opts.voice {
		refs := voiceExamples(opts.profile)
		if len(refs) == 0 {
			return nil, errors.New("--voice needs a voice profile with examples — bind one in the recipe, or name it with --profile/--pack/--profile-file")
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
			diags = append(diags, check.DiagnosticFrom(f, "voice", check.Location{File: DisplayName(file)}))
		}
	}

	return diags, nil
}

// collectBilingualDiagnostics runs the target-gated localization checks
// (placeholder integrity, do-not-translate) over a source/target block set.
// A checker that could not run is an error, not an empty finding set: a silent
// skip would report the file as passing placeholder integrity it was never
// measured against.
func (a *App) collectBilingualDiagnostics(ctx context.Context, blocks []*model.Block, file string, loc model.LocaleID, dntTerms []string) ([]check.Diagnostic, error) {
	var diags []check.Diagnostic
	// Seed the per-block delta counts from the findings already on each block:
	// in bilingual mode the source checks (collectFileDiagnostics) ran first, so
	// starting at zero would re-attribute their findings to the placeholder
	// family.
	seen := make([]int, len(blocks))
	for i, b := range blocks {
		seen[i] = len(FindingsFromBlock(b, false))
	}

	if err := a.runFamily(ctx, blocks, coretools.NewPlaceholderCheckTool(coretools.NewPlaceholderCheckConfig(loc))); err != nil {
		return nil, fmt.Errorf("placeholder check %s (%s): %w", DisplayName(file), loc, err)
	}
	diags = append(diags, mapBlockDeltas(blocks, seen, "placeholder", file)...)

	if len(dntTerms) > 0 {
		dntCfg := coretools.NewDNTCheckConfig(loc)
		dntCfg.Terms = dntTerms
		if err := a.runFamily(ctx, blocks, coretools.NewDNTCheckTool(dntCfg)); err != nil {
			return nil, fmt.Errorf("do-not-translate check %s (%s): %w", DisplayName(file), loc, err)
		}
		diags = append(diags, mapBlockDeltas(blocks, seen, "dnt", file)...)
	}
	return diags, nil
}

// runFamily runs one checker family's tool(s) over every block. Findings
// accumulate on each block's unified annotation; the caller reads the delta.
// A checker that fails aborts the family: its blocks carry no annotation, so
// continuing would report the remainder as a complete, clean result.
func (a *App) runFamily(ctx context.Context, blocks []*model.Block, tools ...BlockProcessor) error {
	for _, b := range blocks {
		for _, t := range tools {
			if err := RunCheckTool(ctx, t, b); err != nil {
				return err
			}
		}
	}
	return nil
}

// mapBlockDeltas maps the findings each block gained since the last family (the
// slice past seen[i]) into located, family-tagged diagnostics, then advances the
// per-block seen count.
func mapBlockDeltas(blocks []*model.Block, seen []int, family, file string) []check.Diagnostic {
	var out []check.Diagnostic
	for i, b := range blocks {
		all := FindingsFromBlock(b, false)
		for _, f := range all[seen[i]:] {
			out = append(out, check.DiagnosticFrom(f, family, check.Location{File: DisplayName(file), Block: blockKey(b)}))
		}
		seen[i] = len(all)
	}
	return out
}

// patternRules builds forbidden (must-not-match) and required (must-match)
// pattern rules from the --forbid / --require flag values.
func patternRules(forbid, require []string) []check.PatternRule {
	var rules []check.PatternRule
	for i, p := range forbid {
		rules = append(rules, check.PatternRule{Name: fmt.Sprintf("forbidden-%d", i+1), Pattern: p, MustNotMatch: true})
	}
	for i, p := range require {
		rules = append(rules, check.PatternRule{Name: fmt.Sprintf("required-%d", i+1), Pattern: p, MustMatch: true})
	}
	return rules
}

// gateFromFlags builds the severity/score gate from the command flags, applying
// the --strict / --lenient presets.
func gateFromFlags(cmd Command) check.Gate {
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

// checkVoice answers "which voice governs this file" for one `kapi check` run.
//
// An explicit --profile / --profile-file / --pack names one voice for every
// file, and outranks the recipe. With none, the project's recipe governs, and it
// governs PER FILE: the point a file sits at — its content item's own
// `channel:`, else its collection's — selects the profile, and an expired
// profile selects none. Two files of one project are therefore checked against
// two vocabularies when the recipe says so, which is the whole reason a recipe
// can bind governance to a point.
//
// Resolved profiles are cached by point, so a check over a thousand files loads
// each voice once.
type checkVoice struct {
	app   *App
	cmd   Command
	fixed *profile.VoiceProfile
	proj  *project.KapiProject
	root  string
	store string
	cache map[string]*profile.VoiceProfile
}

// newCheckVoice builds the resolver for one run. A project that will not load
// leaves it with nothing to resolve, which is the ad-hoc case: `kapi check` on
// a file outside any project checks the content-only families.
func (a *App) newCheckVoice(cmd Command) (*checkVoice, error) {
	v := &checkVoice{app: a, cmd: cmd, cache: map[string]*profile.VoiceProfile{}}

	name, _ := cmd.Flags().GetString("profile")
	file, _ := cmd.Flags().GetString("profile-file")
	pack, _ := cmd.Flags().GetString("pack")
	if name != "" || file != "" || pack != "" {
		p, _, err := a.ResolveVoiceProfileCmd(cmd)
		if err != nil {
			return nil, err
		}
		v.fixed = p
		return v, nil
	}

	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return v, nil
	}
	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return nil, fmt.Errorf("load project for voice: %w", lerr)
	}
	storePath, serr := resolveResourcePath(cmd, "brands", "brand.db")
	if serr != nil {
		return nil, serr
	}
	v.proj, v.root, v.store = proj, filepath.Dir(projectPath), storePath
	return v, nil
}

// forFile returns the voice profile governing one file, or nil when nothing
// binds one there.
func (v *checkVoice) forFile(ctx context.Context, file string) (*profile.VoiceProfile, error) {
	if v.fixed != nil || v.proj == nil {
		return v.fixed, nil
	}

	point := v.app.GovernancePointFor("", "")
	abs := file
	if !filepath.IsAbs(abs) {
		if r, aerr := filepath.Abs(abs); aerr == nil {
			abs = r
		}
	}
	if rel, ok := projectRelPath(v.root, abs); ok {
		point = v.app.GovernancePointFor("", rel)
	}

	rc, err := v.app.ResolveGovernanceAtPoint(v.cmd, v.proj, point)
	if err != nil {
		return nil, err
	}
	key := rc.Profile + "\x00" + rc.Channel
	if p, ok := v.cache[key]; ok {
		return p, nil
	}
	p, _, found, err := v.app.ResolveVoiceProfile(ctx, v.proj, v.root, VoiceResolveOptions{
		StorePath: v.store,
		Point:     point,
	})
	if err != nil {
		return nil, err
	}
	if !found {
		p = nil
	}
	v.cache[key] = p
	return p, nil
}
