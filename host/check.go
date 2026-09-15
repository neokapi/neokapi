package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/terms"
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
	switch r.Verdict {
	case check.VerdictFailed:
		verdict = s.Error.Render("FAIL")
	case check.VerdictDidNotRun:
		verdict = s.Warn.Render("DID NOT RUN")
	}
	fmt.Fprintf(w, "%s configured checks: ", verdict)
	writeFindingsCounts(w, r.Summary)
	if r.Verdict == check.VerdictDidNotRun {
		writeDidNotRun(w, r.DidNotRunCause)
	}
	if r.Execution != nil {
		completed, invalid, skipped := 0, 0, 0
		for _, run := range r.Execution.Analyzers {
			switch run.Status {
			case check.AnalyzerPassed, check.AnalyzerFindings:
				completed++
			case check.AnalyzerInvalid:
				invalid++
			default:
				skipped++
			}
		}
		fmt.Fprintf(w, "  Coverage: %d completed, %d not run or unsupported", completed, skipped)
		if invalid > 0 {
			fmt.Fprintf(w, ", %d missed a canary", invalid)
		}
		fmt.Fprintln(w, ". Score covers reported findings only.")
	}
	if r.Scope != nil {
		writeScope(w, r.Scope)
	}
	for _, reason := range r.DidNotRun {
		fmt.Fprintf(w, "  did not run: %s\n", reason)
	}
	if r.Execution != nil {
		for _, scope := range r.Execution.Contexts {
			writeCheckContext(w, scope)
		}
	}
	for _, reason := range r.Gate.Failed {
		fmt.Fprintf(w, "  gate: %s\n", reason)
	}
	writeWarnings(w, r.Warnings)
	return nil
}

// writeScope lists every file a diff named and what the check did with it.
func writeScope(w io.Writer, s *check.Scope) {
	checked, blocks := 0, 0
	for _, f := range s.Files {
		if f.Status == check.ScopeChecked {
			checked++
		}
		blocks += len(f.Blocks)
	}
	fmt.Fprintf(w, "  Diff scope (%s): %d file(s) changed, %d checked, %d block(s)\n", DisplayName(s.Diff), len(s.Files), checked, blocks)
	for _, f := range s.Files {
		status := strings.ReplaceAll(string(f.Status), "_", " ")
		switch {
		case f.Status == check.ScopeChecked:
			fmt.Fprintf(w, "    %s: %s, %d block(s)\n", f.Path, status, len(f.Blocks))
		case f.Reason != "" && len(f.Blocks) > 0:
			fmt.Fprintf(w, "    %s: %s, %d block(s) checked (%s)\n", f.Path, status, len(f.Blocks), f.Reason)
		case f.Reason != "":
			fmt.Fprintf(w, "    %s: %s (%s)\n", f.Path, status, f.Reason)
		default:
			fmt.Fprintf(w, "    %s: %s\n", f.Path, status)
		}
	}
}

// writeCheckContext displays effective selection independently of the verdict.
func writeCheckContext(w io.Writer, scope check.CheckContext) {
	input := scope.File
	if input == "" {
		input = "draft"
	}
	if scope.ContextPath != "" {
		input += " for " + scope.ContextPath
	}
	voice := "none"
	if scope.Voice.Applied {
		voice = scope.Voice.Name
	}
	fmt.Fprintf(w, "  Context: %s; voice %s (%s)", input, voice, scope.Voice.Selection)
	if scope.Voice.Profile != "" {
		fmt.Fprintf(w, "; profile %s", scope.Voice.Profile)
	}
	if scope.Voice.Channel != "" {
		fmt.Fprintf(w, "; channel %s", scope.Voice.Channel)
	}
	if scope.Voice.Source != "" {
		fmt.Fprintf(w, "; source %s", scope.Voice.Source)
	}
	terms := "none loaded"
	if scope.TermsApplied {
		terms = "loaded"
	}
	fmt.Fprintf(w, "; terms %s\n", terms)
}

// writeFindingsCounts writes the score + severity roll-up line shared by
// `kapi check` (after its PASS/FAIL verdict) and a `kapi exec <check>` run.
func writeFindingsCounts(w io.Writer, s check.Summary) {
	fmt.Fprintf(w, "score %d/100 · %d finding(s) (%d critical, %d major, %d minor)\n",
		s.Score, s.Findings, s.Critical, s.Major, s.Minor)
}

// writeDidNotRun writes the sentence that names why a check did not run. The
// causes share an exit code, so the sentence is what tells a broken checker
// from an empty scope.
func writeDidNotRun(w io.Writer, cause string) {
	s := output.NewTable(w).Styles()
	sentence := s.Warn.Render("Did not run: " + check.CauseSummary(cause) + ".")
	if cause == check.CauseCheckerInvalid {
		sentence = s.Error.Render("Did not run: " + check.CauseSummary(cause) + ".")
	}
	fmt.Fprintf(w, "  %s (%s)\n", sentence, cause)
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
		if l := d.Location.Lines; l != nil {
			loc += fmt.Sprintf(" L%d", l.First)
			if l.Last != l.First {
				loc += fmt.Sprintf("-%d", l.Last)
			}
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
	switch report.Verdict {
	case check.VerdictDidNotRun:
		// --no-fail and --lenient govern what the findings do to the exit code.
		// A check that did not run has no findings to read, so neither applies.
		return fmt.Errorf("%w: %s (%s): %s", ErrCheckNotRun, check.CauseSummary(report.DidNotRunCause), report.DidNotRunCause, strings.Join(report.DidNotRun, "; "))
	case check.VerdictFailed:
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
		return nil, errors.New("at least one file is required, or run inside a kapi project to check its declared content")
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
		return nil, fmt.Errorf("%s declares no content to check. Add some with `kapi add <glob>`", DisplayName(projectPath))
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

// applyProjectSourceLang adopts the project's declared source language when the
// caller did not name one. Best-effort: no project, or one that will not load,
// leaves the resolved language as it stands — that is the file-only shape, which
// has no project language to adopt.
func (a *App) applyProjectSourceLang(cmd Command) {
	if cmd == nil {
		return
	}
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return
	}
	proj, err := project.LoadWithOptions(path, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return
	}
	a.ResolveSourceLang(proj.Defaults.SourceLanguage)
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
// gate and an assistant loop read the same findings and gate; timings vary.
func (a *App) ComputeCheck(cmd Command, args []string) (check.Report, error) {
	execution := newCheckExecution()
	a.InitRegistries()
	ctx := CmdContext(cmd)

	targetFile, _ := cmd.Flags().GetString("target")
	diff, err := a.diffSourceFromFlags(cmd)
	if err != nil {
		return check.Report{}, err
	}
	if diff != nil && targetFile != "" {
		return check.Report{}, errors.New("--target checks a source against its translation and cannot be scoped to a diff")
	}
	// unread collects the declared content a check over the whole project cannot
	// open. A check over named files keeps it nil, so a file there with no
	// reader fails the check.
	var unread *UnreadSet
	if diff != nil {
		// A diff names the files; named files narrow it.
	} else if len(args) == 0 {
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
		unread = a.newUnreadSet()
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

	// The project's own source language, unless the caller named one. A term
	// lookup matches the locale exactly, so content read in the wrong language is
	// held to no vocabulary at all. `check --ship` resolves it the same way, off
	// the same precedence (host/sourcelang.go).
	a.applyProjectSourceLang(cmd)

	contextStart := time.Now()
	voice, err := a.newCheckVoice(cmd, execution.warningSink())
	if err != nil {
		return check.Report{}, err
	}
	defer voice.close()

	validateMode, err := validateModeFromFlag(cmd)
	if err != nil {
		return check.Report{}, err
	}
	if targetFile != "" && validateMode != format.ValidationOff {
		return check.Report{}, errors.New("reader validation is unavailable with --target; validate each file separately")
	}

	// The vocabulary the project decided travels with the profile: a term
	// retired in the project's terms is a finding here, not only in retrieval.
	// Resolved per file for the same reason the profile is — see checkTerms.
	vocab, err := a.newCheckTerms(cmd)
	if err != nil {
		return check.Report{}, err
	}

	// The formats the project declared for its content: what a file IS to this
	// project, resolved once for the run.
	formats, err := a.newCheckFormats(cmd)
	if err != nil {
		return check.Report{}, err
	}

	execution.Timings.ContextMS += elapsedMS(contextStart)
	opts := checkRunOptions{formats: formats, execution: execution}
	opts.maxChars, _ = cmd.Flags().GetInt("max-chars")
	opts.maxWords, _ = cmd.Flags().GetInt("max-words")
	opts.forbid, _ = cmd.Flags().GetStringSlice("forbid")
	opts.require, _ = cmd.Flags().GetStringSlice("require")
	opts.voice, _ = cmd.Flags().GetBool("voice")
	opts.voiceMin, _ = cmd.Flags().GetFloat64("voice-min")

	if diff != nil {
		if validateMode != format.ValidationOff {
			return check.Report{}, errors.New("reader validation is unavailable with a diff scope; validate the files separately")
		}
		return a.runDiffCheck(ctx, diffCheckRun{src: diff, named: args, cmd: cmd, opts: opts, voice: voice, vocab: vocab, gate: gateFromFlags(cmd)})
	}

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
		contextStart = time.Now()
		g, gerr := a.governFile(ctx, voice, vocab, sourcePath, atPoint{})
		if gerr != nil {
			return check.Report{}, gerr
		}
		// A translated rendering holds no comment layer, so its blocks sit at
		// the source file's own point.
		opts = opts.govern(g)
		opts.comments = nil
		execution.recordContext(sourcePath, "", opts)
		execution.Timings.ContextMS += elapsedMS(contextStart)
		// `--target` names the translated rendering of one source file, so both
		// files carry the source's reader binding.
		fmtName, fmtCfg := opts.formats.forFile(a, sourcePath)
		unit := VerifyUnit{
			SourcePath:   sourcePath,
			TargetPath:   targetFile,
			Locale:       targetLang,
			DisplayPath:  targetFile,
			SourceFormat: fmtName,
			SourceConfig: fmtCfg,
			TargetFormat: fmtName,
			TargetConfig: fmtCfg,
		}
		extractionStart := time.Now()
		blocks, missing, berr := a.bilingualBlocks(ctx, unit)
		execution.Timings.ExtractionMS += elapsedMS(extractionStart)
		execution.skipped("reader.validation", sourcePath, "Reader validation was not requested.")
		if berr != nil {
			return check.Report{}, NoReaderError(berr, DisplayName(sourcePath), fmtName)
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
		termRules, terr := vocab.rulesFor(sourcePath, targetLang)
		if terr != nil {
			return check.Report{}, terr
		}
		biDiags, berr := a.collectBilingualDiagnostics(ctx, blocks, sourcePath, model.LocaleID(targetLang), dnt, termRules, execution)
		if berr != nil {
			return check.Report{}, berr
		}
		opts.stampPoints(biDiags, blocks)
		diags = append(diags, biDiags...)
	} else {
		// Content-first generic mode: each positional file is a source, checked
		// independently. With --validate (Reader Validation-Mode) the format
		// reader's structure/encoding diagnostics fold into the same Report; off
		// (the default) keeps the lenient read where a malformed file is an
		// operational error.
		prog := a.NewProgress(cmd, "checking", len(args))
		defer prog.Done()
		var checked []string
		for _, file := range args {
			prog.Step(DisplayName(file))
			// The governance is resolved per file: a run over a governed
			// project checks each file against the voice and the vocabulary in
			// force where that file sits, not against one pair picked for the
			// whole invocation. A file's comments are held to the point they sit
			// at, which the project may place apart from the file's.
			contextStart = time.Now()
			g, gerr := a.governFile(ctx, voice, vocab, file, atPoint{})
			if gerr != nil {
				return check.Report{}, gerr
			}
			opts = opts.govern(g)
			execution.Timings.ContextMS += elapsedMS(contextStart)
			blocks, fileDiags, ferr := a.checkFileBlocks(ctx, file, validateMode, opts)
			prog.Advance()
			if ferr != nil {
				if name, _ := opts.formats.forFile(a, file); unread.Skip(ferr, file, name) {
					continue
				}
				return check.Report{}, ferr
			}
			// The context is recorded for a file the check read, and a skipped
			// file has none.
			execution.recordContexts(file, "", opts, blocks)
			checked = append(checked, file)
			totalBlocks += len(blocks)
			diags = append(diags, fileDiags...)
		}
		prog.Done()
		if len(checked) == 1 {
			target.File = checked[0]
		} else {
			target.File = fmt.Sprintf("%d files", len(checked))
		}
	}
	target.Blocks = totalBlocks

	gate := gateFromFlags(cmd)
	report := execution.report(target, diags, gate)
	if validateMode == format.ValidationStrict {
		applyStrictValidationGate(&report)
	}
	if lenient, _ := cmd.Flags().GetBool("lenient"); !lenient {
		ApplyFormatterGate(&report)
	}
	unread.Report(&report)
	unread.warn(a, cmd)
	return report, nil
}

// checkFileBlocks reads one file's blocks and the content checkset diagnostics,
// folding in the reader's structure/encoding diagnostics when validateMode is on.
func (a *App) checkFileBlocks(ctx context.Context, file string, validateMode format.ValidationMode, opts checkRunOptions) ([]*model.Block, []check.Diagnostic, error) {
	var blocks []*model.Block
	var diags []check.Diagnostic

	fmtName, fmtCfg := opts.formats.forFile(a, file)
	if p, ok := a.commentLayerFor(file, fmtName); ok {
		return a.checkCommentFile(ctx, file, p, validateMode, opts)
	}
	if opts.formats.commentsOnly(file) {
		return a.checkCommentsOnlyFile(ctx, file, fmtName, validateMode, opts)
	}
	extractionStart := time.Now()

	if validateMode != format.ValidationOff {
		bl, fdiags, rerr := a.readBlocksValidated(ctx, file, fmtName, fmtCfg, a.SourceLocale(), validateMode)
		if rerr != nil {
			return nil, nil, rerr
		}
		blocks = bl
		for _, fd := range fdiags {
			diags = append(diags, check.DiagnosticFromReader(fd, DisplayName(file)))
		}
	} else {
		bl, rerr := a.readBlocksAs(ctx, file, fmtName, fmtCfg, a.SourceLocale())
		if rerr != nil {
			// A read failure is operational in off mode: the lenient readers
			// extract from imperfect inputs, so a hard error means the file
			// could not be parsed at all. Pass --validate report to fold the
			// structure problem into the Report instead.
			return nil, nil, rerr
		}
		blocks = bl
	}

	if opts.execution != nil {
		opts.execution.Timings.ExtractionMS += elapsedMS(extractionStart)
	}
	if validateMode == format.ValidationOff {
		opts.execution.skipped("reader.validation", file, "Reader validation was not requested.")
	} else {
		canary, err := a.probeReaderValidation(validateMode)
		if err != nil {
			return nil, nil, err
		}
		opts.execution.completed("reader.validation", file, len(diags), extractionStart, canary, true)
	}

	// A recipe that declares the file's comments adds its comment layer to the
	// blocks the reader extracted. The reader's own blocks are what they were.
	layer, lerr := a.readDeclaredComments(ctx, file, fmtName, opts)
	if lerr != nil {
		return nil, nil, lerr
	}
	if layer != nil {
		layerDiags, err := recordProviderAnalyzers(ctx, layer.analyzers, layer.blocks, file, opts.execution)
		if err != nil {
			return nil, nil, err
		}
		blocks = append(blocks, layer.blocks...)
		diags = append(diags, layerDiags...)
	}

	fileDiags, ferr := a.collectFileDiagnostics(ctx, blocks, file, opts)
	if ferr != nil {
		return nil, nil, ferr
	}
	diags = append(diags, fileDiags...)
	opts.stampPoints(diags, blocks)
	if layer != nil {
		layer.locate(diags)
	}
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
		report.Gate.Failed = append(report.Gate.Failed,
			fmt.Sprintf("validation: %s is a blocking %s problem", f.Rule, f.Check))
	}
	report.Decide()
}

// commentLimitsOnly reports whether p holds comment limits and no rule for
// voice.rules, over blocks holding a comment for those limits to check.
func commentLimitsOnly(p *profile.VoiceProfile, blocks []*model.Block) bool {
	return p != nil && p.Style.Comments != nil && !profile.HasDeterministicRules(p) && slices.ContainsFunc(blocks, comment.IsBlock)
}

// recordGuidance records voice.guidance as unsupported when p holds applicable
// guidance, which deterministic rules do not assess.
func recordGuidance(execution *checkExecution, p *profile.VoiceProfile, file string) {
	for _, resolution := range profile.ConstraintResolutions(p) {
		if resolution.Status == "applicable" && resolution.Constraint.Kind == profile.ConstraintGuidance {
			execution.unsupported("voice.guidance", file,
				"Applicable guidance requires semantic analysis; deterministic rules do not assess it.")
			return
		}
	}
}

// checkRunOptions carries the resolved generic-check configuration.
type checkRunOptions struct {
	execution    *checkExecution
	profile      *profile.VoiceProfile
	voiceContext check.VoiceContext
	// terms is the project's terms store, when it binds one: the vocabulary the
	// project decided, enforced beside the profile's own lists. nil for a run
	// with no project or no terminology.
	terms    terms.Terminology
	maxChars int
	maxWords int
	forbid   []string
	require  []string
	voice    bool
	voiceMin float64
	// formats binds each file to the format and reader config the project
	// declared for it; nil outside a project.
	formats *checkFormats
	// documentBlocks, when set, are the whole document's blocks for the rules
	// that hold over a document, where the blocks checked are only some of them
	// (a diff-scoped check).
	documentBlocks []*model.Block
	// point is where the project resolved the governance above, nil outside a
	// project.
	point *check.Point
	// comments is the governance at the point a file's comments sit at, nil
	// when they share the file's point.
	comments *atPoint
	// change is the change a diff-scoped check reads the file for, with the
	// lines its comment layer classified. It is nil in a whole-file check, and
	// for a file no comment layer reads.
	change *commentChange
}

// collectFileDiagnostics runs the source-side content checkset over one file's
// blocks and returns family-attributed, located diagnostics. Each checker family
// runs in turn; the new findings it adds to the unified annotation are tagged
// with the family and the block location.
func (a *App) collectFileDiagnostics(ctx context.Context, blocks []*model.Block, file string, opts checkRunOptions) ([]check.Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var diags []check.Diagnostic
	seen := make([]int, len(blocks)) // per-block count of findings already mapped

	// Each analyzer is given its canaries after the content, through the same
	// checker instance, so one that finds nothing in anything is recorded as
	// invalid instead of as a clean pass.
	start := time.Now()
	// Hygiene — always on, no configuration needed.
	hygiene := hygieneTool()
	if err := a.runFamily(ctx, blocks, hygiene); err != nil {
		return nil, fmt.Errorf("hygiene check %s: %w", DisplayName(file), err)
	}
	diags = append(diags, mapBlockDeltas(blocks, seen, "hygiene", file)...)
	canary, err := probeTool(ctx, hygiene, check.HygieneCanaries(), "")
	if err != nil {
		return nil, fmt.Errorf("hygiene check %s: %w", DisplayName(file), err)
	}
	opts.execution.completed("hygiene", file, len(diags), start, canary, true)

	// Length — only when a limit is set.
	if opts.maxChars > 0 || opts.maxWords > 0 {
		start = time.Now()
		before := len(diags)
		lengthTool, err := check.NewSourceLengthTool(opts.maxChars, opts.maxWords)
		if err != nil {
			return nil, err
		}
		if err := a.runFamily(ctx, blocks, lengthTool); err != nil {
			return nil, fmt.Errorf("length check %s: %w", DisplayName(file), err)
		}
		diags = append(diags, mapBlockDeltas(blocks, seen, "length", file)...)
		canary, err := probeTool(ctx, lengthTool, check.LengthCanaries(opts.maxChars, opts.maxWords), "")
		if err != nil {
			return nil, fmt.Errorf("length check %s: %w", DisplayName(file), err)
		}
		opts.execution.completed("length", file, len(diags)-before, start, canary, true)
	} else {
		opts.execution.skipped("length", file, "No length limit was configured.")
	}

	// Pattern — forbidden (must-not-match) and required (must-match).
	if rules := patternRules(opts.forbid, opts.require); len(rules) > 0 {
		start = time.Now()
		before := len(diags)
		patternTool, err := check.NewSourcePatternTool(rules)
		if err != nil {
			return nil, err
		}
		if err := a.runFamily(ctx, blocks, patternTool); err != nil {
			return nil, fmt.Errorf("pattern check %s: %w", DisplayName(file), err)
		}
		diags = append(diags, mapBlockDeltas(blocks, seen, "pattern", file)...)
		canaries, uncheckable := check.PatternCanaries(rules)
		canary, err := probeTool(ctx, patternTool, canaries, uncheckable)
		if err != nil {
			return nil, fmt.Errorf("pattern check %s: %w", DisplayName(file), err)
		}
		opts.execution.completed("pattern", file, len(diags)-before, start, canary, true)
	} else {
		opts.execution.skipped("pattern", file, "No explicit patterns were configured.")
	}

	// Voice rules and project terminology share the vocabulary checker. Each
	// group of blocks is held to the voice and terms of the point it sits at: a
	// file's comments at their own point when the project places them apart.
	docBlocks := blocks
	if opts.documentBlocks != nil {
		docBlocks = opts.documentBlocks
	}
	groups := opts.pointGroups(blocks, docBlocks)
	for _, g := range groups {
		mark := opts.execution.analyzerCount()
		switch {
		case g.at.profile == nil && g.at.terms == nil:
			opts.execution.skipped("voice.rules", file, "No voice profile or project terms were bound.")
		case g.at.terms == nil && commentLimitsOnly(g.at.profile, g.blocks):
			// The comment analyzers below hold these comments to the profile's
			// limits and decide the verdict for them.
			opts.execution.notApplicable("voice.rules", file,
				"The voice profile declares no term or pattern, and the comment analyzers check its comment limits.")
			recordGuidance(opts.execution, g.at.profile, file)
		default:
			start = time.Now()
			before := len(diags)
			vocab := coretools.NewVoiceVocabCheckTool(g.at.profile, g.at.terms).InSourceLocale(model.LocaleID(a.SourceLocale()))
			for _, b := range g.blocks {
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
			// The profile's required patterns hold over the document, not over any
			// one block in it (profile.DocumentFindings): the page carries the
			// notice, not every paragraph of it. They are reported against the file,
			// with no block, because an absence sits nowhere in particular.
			docLoc := check.Location{File: DisplayName(file)}
			for _, f := range profile.DocumentFindings(g.at.profile, documentText(g.doc)) {
				d := check.DiagnosticFrom(f, "voice", docLoc)
				d.Point = clonePoint(g.at.point)
				diags = append(diags, d)
			}
			canary, err := probeVoiceRules(ctx, vocab, g.at.profile)
			if err != nil {
				return nil, fmt.Errorf("voice vocabulary check %s: %w", DisplayName(file), err)
			}
			// A profile named on the command line is an analysis the invocation asked
			// for. One the project binds is configuration, and may govern tone alone.
			opts.execution.completed("voice.rules", file, len(diags)-before, start, canary, g.at.voiceContext.Selection == "override")
			recordGuidance(opts.execution, g.at.profile, file)
		}
		// The comments among the group's blocks are held to the comment limits
		// of the group's voice.
		limitDiags, err := a.checkCommentLimits(ctx, g.blocks, g.at, file, opts.change, opts.execution)
		if err != nil {
			return nil, err
		}
		diags = append(diags, limitDiags...)
		if g.apart {
			opts.execution.pointAnalyzers(mark, g.at.point)
		}
	}

	// Voice/style similarity (opt-in, --voice): drives the kapi-check plugin.
	if opts.voice {
		for _, g := range groups {
			mark := opts.execution.analyzerCount()
			start = time.Now()
			before := len(diags)
			refs := voiceExamples(g.at.profile)
			if len(refs) == 0 {
				return nil, errors.New("--voice needs a voice profile with examples. Bind one in the recipe, or name it with --profile/--pack/--profile-file")
			}
			t, closeT, derr := dialVoicePlugin(ctx)
			if derr != nil {
				return nil, derr
			}
			defer closeT()
			vf, verr := voiceSimilarityFindings(g.blocks, refs, t, opts.voiceMin)
			if verr != nil {
				return nil, fmt.Errorf("voice check: %w", verr)
			}
			for _, f := range vf {
				d := check.DiagnosticFrom(f, "voice", check.Location{File: DisplayName(file)})
				d.Point = clonePoint(g.at.point)
				diags = append(diags, d)
			}
			canary, err := check.Probe([]check.Canary{{Name: "text unlike every example", Block: check.CanaryBlock("0000 1111 2222 3333")}}, "",
				func(b *model.Block) ([]check.Finding, error) {
					return voiceSimilarityFindings([]*model.Block{b}, refs, t, opts.voiceMin)
				})
			if err != nil {
				return nil, fmt.Errorf("voice check: %w", err)
			}
			opts.execution.completed("voice.similarity", file, len(diags)-before, start, canary, true)
			if g.apart {
				opts.execution.pointAnalyzers(mark, g.at.point)
			}
		}
	} else {
		opts.execution.skipped("voice.similarity", file, "Similarity analysis was not requested.")
	}
	opts.execution.skipped("voice.llm", file, "This command does not run semantic review.")

	opts.stampPoints(diags, blocks)
	return diags, nil
}

// documentText joins a file's translatable blocks into the one text a
// document-scope rule reads. Blocks are separated by a newline so a rule
// spanning a paragraph boundary cannot match across two blocks that the file
// keeps apart, and non-translatable content is left out for the same reason the
// block-scope checkers skip it: a code fence is not part of the page's prose.
func documentText(blocks []*model.Block) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk == nil || !blk.Translatable {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(model.RunsText(blk.SourceRuns()))
	}
	return b.String()
}

// collectBilingualDiagnostics runs the target-gated localization checks
// (placeholder integrity, do-not-translate) over a source/target block set.
// A checker that could not run is an error, not an empty finding set: a silent
// skip would report the file as passing placeholder integrity it was never
// measured against.
func (a *App) collectBilingualDiagnostics(ctx context.Context, blocks []*model.Block, file string, loc model.LocaleID, dntTerms []string, termRules []profile.TermRule, executions ...*checkExecution) ([]check.Diagnostic, error) {
	var execution *checkExecution
	if len(executions) > 0 {
		execution = executions[0]
	}
	start := time.Now()
	var diags []check.Diagnostic
	// Seed the per-block delta counts from the findings already on each block:
	// in bilingual mode the source checks (collectFileDiagnostics) ran first, so
	// starting at zero would re-attribute their findings to the placeholder
	// family.
	seen := make([]int, len(blocks))
	for i, b := range blocks {
		seen[i] = len(FindingsFromBlock(b, false))
	}

	placeholder := coretools.NewPlaceholderCheckTool(coretools.NewPlaceholderCheckConfig(loc))
	if err := a.runFamily(ctx, blocks, placeholder); err != nil {
		return nil, fmt.Errorf("placeholder check %s (%s): %w", DisplayName(file), loc, err)
	}
	diags = append(diags, mapBlockDeltas(blocks, seen, "placeholder", file)...)
	canary, err := probeTool(ctx, placeholder, coretools.PlaceholderCanaries(loc), "")
	if err != nil {
		return nil, fmt.Errorf("placeholder check %s (%s): %w", DisplayName(file), loc, err)
	}
	execution.completed("placeholder", file, len(diags), start, canary, true)

	// The project's term rules for the target language, the rules the ship
	// terminology gate holds the same translation to. term-check records its
	// violations as block properties rather than findings, so they are mapped
	// here: a violation of a rule that fails is critical, which fails the check
	// as it fails the gate, and one that only warns is minor.
	if len(termRules) > 0 {
		start = time.Now()
		before := len(diags)
		cfg := &coretools.TermCheckConfig{TermRules: termRules, SourceLocale: model.LocaleID(a.SourceLocale()), TargetLocale: loc}
		tc := coretools.NewTermCheckTool(cfg)
		for _, b := range blocks {
			if err := RunCheckTool(ctx, tc, b); err != nil {
				return nil, fmt.Errorf("terminology check %s (%s): %w", DisplayName(file), loc, err)
			}
			for _, v := range []struct {
				prop     string
				severity check.Severity
			}{
				{coretools.PropTermCheckErrors, check.SeverityCritical},
				{coretools.PropTermCheckWarnings, check.SeverityMinor},
			} {
				for m := range strings.SplitSeq(b.Properties[v.prop], "; ") {
					if strings.TrimSpace(m) == "" {
						continue
					}
					f := check.Finding{Category: "terminology", Severity: v.severity, Message: m}
					diags = append(diags, check.DiagnosticFrom(f, "terms", check.Location{File: DisplayName(file), Block: blockKey(b)}))
				}
			}
		}
		canaries, uncheckable := coretools.TermCheckCanaries(cfg)
		canary, err := check.Probe(canaries, uncheckable, func(b *model.Block) ([]check.Finding, error) {
			if err := RunCheckTool(ctx, tc, b); err != nil {
				return nil, err
			}
			return termCheckFindings(b), nil
		})
		if err != nil {
			return nil, fmt.Errorf("terminology check %s (%s): %w", DisplayName(file), loc, err)
		}
		execution.completed("terms", file, len(diags)-before, start, canary, true)
	} else {
		execution.skipped("terms", file, "No terms govern this file in its target language.")
	}

	if len(dntTerms) > 0 {
		start = time.Now()
		before := len(diags)
		dntCfg := coretools.NewDNTCheckConfig(loc)
		dntCfg.Terms = dntTerms
		dnt := coretools.NewDNTCheckTool(dntCfg)
		if err := a.runFamily(ctx, blocks, dnt); err != nil {
			return nil, fmt.Errorf("do-not-translate check %s (%s): %w", DisplayName(file), loc, err)
		}
		diags = append(diags, mapBlockDeltas(blocks, seen, "dnt", file)...)
		canaries, uncheckable := coretools.DNTCanaries(dntCfg)
		canary, err := probeTool(ctx, dnt, canaries, uncheckable)
		if err != nil {
			return nil, fmt.Errorf("do-not-translate check %s (%s): %w", DisplayName(file), loc, err)
		}
		execution.completed("dnt", file, len(diags)-before, start, canary, true)
	} else {
		execution.skipped("dnt", file, "No protected terms were configured.")
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
// checkFormats answers, for one source file, the format the project declared it
// as and the reader config it declared for it — the same binding the flow runner
// and extract/merge apply.
//
// Without it the gate read every file by extension under reader defaults, so it
// judged content the project does not declare as content: a demo script's
// `command:` lines, which the recipe's keyPathPatterns exclude, and `.md` docs
// read by the markdown reader when the recipe binds mdx. Both are text no
// convergence run ever touches, held to a bar meant for prose.
type checkFormats struct {
	// byPath is keyed by absolute path; a file the project does not declare is
	// absent, and reads as "detect by extension, reader defaults".
	byPath map[string]resolvedFormat
	// directives are the project's `defaults.comments.directives`, in force for
	// a file no item declares.
	directives []string
}

type resolvedFormat struct {
	name string
	cfg  map[string]any
	// comments is the item's `comments:` declaration: the file's comments are
	// content as well as what its reader extracts.
	comments bool
	// commentsOnly reports a file declared for its comments alone that a format
	// names (narrowedToComments).
	commentsOnly bool
	// directives are the comment directives in force for the file.
	directives []string
}

// newCheckFormats builds the binding for one run. Outside a project — or with an
// explicit --format override, which names one format for everything the caller
// passed — there is nothing to bind and every file falls back to detection.
func (a *App) newCheckFormats(cmd Command) (*checkFormats, error) {
	f := &checkFormats{byPath: map[string]resolvedFormat{}}
	if a.FormatFlag != "" {
		return f, nil
	}
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return f, nil
	}
	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return nil, fmt.Errorf("load project for formats: %w", lerr)
	}
	f.directives = proj.Defaults.Comments.Directives
	pctx := project.NewProjectContext(proj, filepath.Join(filepath.Dir(projectPath), "x.kapi"))
	resolved, rerr := pctx.ResolveContent(a.FormatReg)
	if rerr != nil {
		// A recipe pattern that will not expand is a fault worth failing on — and
		// the bare `kapi check` path fails on it, in checkProjectSources, before
		// this runs. Naming a file is the other shape: the caller said which file
		// to check, and an unrelated broken pattern elsewhere in the recipe must
		// not stop them checking it. Fall back to detection, which is what naming
		// a file did before there was a binding to fall back from.
		return f, nil
	}
	f.bind(proj, resolved)
	return f, nil
}

// bind records the format, reader config and comment declaration proj gives
// each resolved file.
func (f *checkFormats) bind(proj *project.KapiProject, resolved []project.ResolvedFile) {
	for _, rf := range resolved {
		if _, seen := f.byPath[rf.Path]; seen {
			continue
		}
		f.byPath[rf.Path] = resolvedFormat{
			name:         rf.Format,
			cfg:          mergedFormatConfig(proj, rf.Format, rf.Item),
			comments:     rf.Item != nil && rf.Item.Comments.Declared,
			commentsOnly: narrowedToComments(rf),
			directives:   commentDirectives(proj, rf.Item),
		}
	}
}

// rebind replaces the bindings of the files at rels, paths relative to the
// project directory, with the ones resolved for another version of them, such
// as the version a git object holds. A path the recipe does not declare is left
// with no binding.
func (f *checkFormats) rebind(proj *project.KapiProject, pctx *project.ProjectContext, rels []string, resolved []project.ResolvedFile) {
	if f == nil {
		return
	}
	for _, rel := range rels {
		delete(f.byPath, filepath.Join(pctx.ProjectDir, filepath.FromSlash(rel)))
	}
	f.bind(proj, resolved)
}

// forFile returns the format name and reader config to read one file under. An
// empty name means "detect by extension".
func (f *checkFormats) forFile(app *App, file string) (string, map[string]any) {
	if rf, ok := f.lookup(file); ok {
		return rf.name, rf.cfg
	}
	return app.FormatFlag, nil
}

// commentsFor reports whether the recipe declares one file's comments as
// content.
func (f *checkFormats) commentsFor(file string) bool {
	rf, _ := f.lookup(file)
	return rf.comments
}

// commentsOnly reports a file a format names that the recipe declares for its
// comments alone. The format supplies its comments and its values are never
// read. A file no format names is read for its comments through its language's
// provider instead (commentLayerFor).
func (f *checkFormats) commentsOnly(file string) bool {
	rf, _ := f.lookup(file)
	return rf.commentsOnly
}

// directivesFor returns the comment directives in force for one file: its
// item's, or the project's for a file no item declares.
func (f *checkFormats) directivesFor(file string) []string {
	if rf, ok := f.lookup(file); ok {
		return rf.directives
	}
	if f == nil {
		return nil
	}
	return f.directives
}

// lookup returns what the project declared for one file.
func (f *checkFormats) lookup(file string) (resolvedFormat, bool) {
	if f == nil || len(f.byPath) == 0 {
		return resolvedFormat{}, false
	}
	abs := file
	if !filepath.IsAbs(abs) {
		if r, err := filepath.Abs(abs); err == nil {
			abs = r
		}
	}
	rf, ok := f.byPath[abs]
	return rf, ok
}

// commentDirectives returns the comment directives in force for a file an item
// claims, or the project's own when no item does.
func commentDirectives(proj *project.KapiProject, item *project.ContentItem) []string {
	if item == nil {
		return proj.Defaults.Comments.Directives
	}
	return item.CommentDirectives(proj.Defaults)
}

type checkVoice struct {
	app          *App
	cmd          Command
	fixed        *profile.VoiceProfile
	fixedContext check.VoiceContext
	proj         *project.KapiProject
	root         string
	store        profile.Store
	release      func()
	cache        map[string]checkedVoice
	// warnings receives the configuration warnings of each profile the
	// resolver loads. nil collects none.
	warnings *voiceWarnings
}

// close releases the voice store, if this run opened one of its own. Inside a
// project the store is the shared pool's and the release is a no-op.
func (v *checkVoice) close() {
	if v != nil && v.release != nil {
		v.release()
	}
}

// newCheckVoice builds the resolver for one run. A project that will not load
// leaves it with nothing to resolve, which is the ad-hoc case: `kapi check` on
// a file outside any project checks the content-only families. warnings
// receives the configuration warnings of each profile the run loads.
func (a *App) newCheckVoice(cmd Command, warnings *voiceWarnings) (*checkVoice, error) {
	v := &checkVoice{app: a, cmd: cmd, cache: map[string]checkedVoice{}, warnings: warnings}

	name, _ := cmd.Flags().GetString("profile")
	file, _ := cmd.Flags().GetString("profile-file")
	pack, _ := cmd.Flags().GetString("pack")
	if name != "" || file != "" || pack != "" {
		p, source, err := a.ResolveVoiceProfileCmd(cmd)
		if err != nil {
			return nil, err
		}
		v.fixed = p
		channel, _ := cmd.Flags().GetString("channel")
		v.fixedContext = check.VoiceContext{Selection: "override", Applied: p != nil, Source: source, Channel: channel}
		if p != nil {
			v.fixedContext.Name = p.Name
			if err := v.note(CmdContext(cmd), source); err != nil {
				return nil, err
			}
		}
		return v, nil
	}

	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return v, err
	}
	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return nil, fmt.Errorf("load project for voice: %w", lerr)
	}
	store, release, serr := a.VoiceLookupStore(cmd)
	if serr != nil {
		return nil, serr
	}
	v.proj, v.root, v.store, v.release = proj, filepath.Dir(projectPath), store, release
	return v, nil
}

// checkTerms resolves the vocabulary in force at a file, the way checkVoice
// resolves the voice profile there. The two halves of the gate are governed at
// the same granularity: a project whose profile binds its own `terms:` (or
// carries the conventional `.kapi/profiles/<name>/terms.json`) has said which
// words that region of the context space is held to, and a run that read one
// project-wide vocabulary honoured the recipe's tone binding while ignoring its
// vocabulary binding.
//
// Resolution is per governance point, not per file: the store is built once for
// each distinct (profile, channel) a run touches, so a thousand files sitting at
// two points open two vocabularies.
type checkTerms struct {
	app   *App
	cmd   Command
	proj  *project.KapiProject
	root  string
	cache map[string]terms.Terminology
}

// newCheckTerms builds the resolver for one run. Outside a project there is no
// decided vocabulary, and the resolver answers nil for every file.
func (a *App) newCheckTerms(cmd Command) (*checkTerms, error) {
	t := &checkTerms{app: a, cmd: cmd, cache: map[string]terms.Terminology{}}
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return t, err
	}
	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return nil, fmt.Errorf("load project for terms: %w", lerr)
	}
	t.proj, t.root = proj, filepath.Dir(projectPath)
	return t, nil
}

// ProjectTermsForFile resolves the vocabulary the project decided for one file:
// the terms bound at the point that file sits at, or nil outside a project.
//
// It is what `kapi check` runs its vocabulary gate against, exported so an
// embedded surface runs the same gate rather than a quieter one. A surface that
// passed no terminology reported only what a voice profile forbids and stayed
// silent about every term the project itself retired, which is a different
// answer about the same file.
func (a *App) ProjectTermsForFile(ctx context.Context, cmd Command, file string) (terms.Terminology, error) {
	resolver, err := a.newCheckTerms(cmd)
	if err != nil {
		return nil, err
	}
	return resolver.forFile(ctx, file)
}

// rulesFor returns the term rules a translation of file into target is held to:
// the rules the terms bound at the file's point give for that language. Outside
// a project there are none.
func (t *checkTerms) rulesFor(file, target string) ([]profile.TermRule, error) {
	if t == nil || t.proj == nil {
		return nil, nil
	}
	return t.app.ResolveTermRulesFor(t.cmd, target, t.app.governancePointForFile(t.root, file))
}

// forFile returns the vocabulary governing one file, or nil when nothing binds
// one there.
func (t *checkTerms) forFile(ctx context.Context, file string) (terms.Terminology, error) {
	if t == nil || t.proj == nil {
		return nil, nil
	}
	return t.forPoint(ctx, t.app.governancePointForFile(t.root, file), file)
}

// resolve returns the governance the project resolves at a point, nil outside
// a project.
func (t *checkTerms) resolve(point project.GovernancePoint) (*project.ResolvedGovernance, error) {
	if t == nil || t.proj == nil {
		return nil, nil
	}
	return t.app.ResolveGovernanceAtPoint(t.cmd, t.proj, point)
}

// forPoint returns the vocabulary governing a point. file names what is being
// checked, for an error.
func (t *checkTerms) forPoint(ctx context.Context, point project.GovernancePoint, file string) (terms.Terminology, error) {
	rc, err := t.app.ResolveGovernanceAtPoint(t.cmd, t.proj, point)
	if err != nil {
		return nil, err
	}
	key := rc.Profile + "\x00" + rc.Channel
	if tb, ok := t.cache[key]; ok {
		return tb, nil
	}

	concepts, err := t.app.projectConcepts(t.cmd, point)
	if err != nil {
		return nil, err
	}
	var tb terms.Terminology
	if len(concepts) > 0 {
		// Unlimited: the store holds exactly the vocabulary the project
		// decided, and a cap would enforce a silently truncated one.
		mem := terms.NewInMemoryStore(terms.WithMaxConcepts(0))
		for _, c := range concepts {
			if aerr := mem.AddConcept(ctx, c); aerr != nil {
				return nil, fmt.Errorf("load the vocabulary governing %s: %w", DisplayName(file), aerr)
			}
		}
		tb = mem
	}
	t.cache[key] = tb
	return tb, nil
}

// governancePointForFile is the point a source file sits at: its own, when the
// file is inside the project, and the project default otherwise.
func (a *App) governancePointForFile(root, file string) project.GovernancePoint {
	abs := file
	if !filepath.IsAbs(abs) {
		if r, err := filepath.Abs(abs); err == nil {
			abs = r
		}
	}
	if rel, ok := projectRelPath(root, abs); ok {
		return a.GovernancePointFor("", rel)
	}
	return a.GovernancePointFor("", "")
}

// governancePointForComments is the point the comments in a source file sit
// at.
func (a *App) governancePointForComments(root, file string) project.GovernancePoint {
	point := a.governancePointForFile(root, file)
	point.Comments = true
	return point
}

// checkedVoice caches a loaded profile together with the resolution that selected it.
type checkedVoice struct {
	profile *profile.VoiceProfile
	context check.VoiceContext
}

// forFile returns the effective profile and its selection metadata together.
func (v *checkVoice) forFile(ctx context.Context, file string) (*profile.VoiceProfile, check.VoiceContext, error) {
	if v.fixed != nil || v.proj == nil {
		return v.forPoint(ctx, project.GovernancePoint{})
	}
	return v.forPoint(ctx, v.app.governancePointForFile(v.root, file))
}

// forComments returns the profile governing the comments in one file, at the
// point they sit at, and its selection metadata.
func (v *checkVoice) forComments(ctx context.Context, file string) (*profile.VoiceProfile, check.VoiceContext, error) {
	if v.fixed != nil || v.proj == nil {
		return v.forPoint(ctx, project.GovernancePoint{})
	}
	return v.forPoint(ctx, v.app.governancePointForComments(v.root, file))
}

// forPoint returns the profile governing a point and its selection metadata.
func (v *checkVoice) forPoint(ctx context.Context, point project.GovernancePoint) (*profile.VoiceProfile, check.VoiceContext, error) {
	if v.fixed != nil {
		return v.fixed, v.fixedContext, nil
	}
	if v.proj == nil {
		return nil, check.VoiceContext{Selection: "none"}, nil
	}
	rc, err := v.app.ResolveGovernanceAtPoint(v.cmd, v.proj, point)
	if err != nil {
		return nil, check.VoiceContext{}, err
	}
	key := rc.Profile + "\x00" + rc.Channel
	if cached, ok := v.cache[key]; ok {
		return cached.profile, cached.context, nil
	}
	p, source, found, err := v.app.resolveVoiceForGovernance(ctx, v.root, v.store, rc, VoiceResolveOptions{Point: point})
	if err != nil {
		return nil, check.VoiceContext{}, err
	}
	if !found {
		p = nil
	} else if err := v.note(ctx, source); err != nil {
		return nil, check.VoiceContext{}, err
	}
	selected := check.VoiceContext{Selection: "project", Applied: p != nil, Source: source, Profile: rc.Profile, Channel: rc.Channel}
	if p != nil {
		selected.Name = p.Name
	}
	v.cache[key] = checkedVoice{profile: p, context: selected}
	return p, selected, nil
}
