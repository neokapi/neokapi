package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/spf13/cobra"
)

// Gate names for `kapi verify`.
const (
	gateBrand = "brand"
	gateTerms = "terminology"
	gateQA    = "qa"
)

// DefaultBrandMinScore is the brand compliance score below which the brand
// gate fails when the user does not override it with --min-score.
const DefaultBrandMinScore = 80

// VerifyFinding is a single actionable problem found by one of the verify
// gates. The shape is shared by the human and JSON renderers and is the unit
// an AI assistant reads, fixes, and re-runs against.
type VerifyFinding struct {
	Gate       string `json:"gate"`
	File       string `json:"file,omitempty"`
	Locale     string `json:"locale,omitempty"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// VerifyGateResult is the outcome of one gate: whether it passed and the
// findings it produced.
type VerifyGateResult struct {
	Gate     string          `json:"gate"`
	Pass     bool            `json:"pass"`
	Findings []VerifyFinding `json:"findings"`
}

// VerifySummary carries the aggregate counts for a verify run.
type VerifySummary struct {
	Gates    int `json:"gates"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Findings int `json:"findings"`
	Errors   int `json:"errors"`   // findings with severity "error"
	Warnings int `json:"warnings"` // findings with severity "warning"
}

// VerifyOutput is the single structured result of a `kapi verify` run.
type VerifyOutput struct {
	Pass    bool               `json:"pass"`
	Gates   []VerifyGateResult `json:"gates"`
	Summary VerifySummary      `json:"summary"`
}

// FormatText renders the verify result as a human-readable summary,
// implementing output.TextFormatter.
func (o VerifyOutput) FormatText(w io.Writer) error {
	for _, g := range o.Gates {
		status := "PASS"
		if !g.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(w, "%-13s %s  (%d finding(s))\n", g.Gate, status, len(g.Findings))
		for _, f := range g.Findings {
			loc := ""
			if f.File != "" {
				loc = f.File
			}
			if f.Locale != "" {
				if loc != "" {
					loc += " "
				}
				loc += "[" + f.Locale + "]"
			}
			if loc != "" {
				loc = " " + loc
			}
			fmt.Fprintf(w, "  %s%s: %s\n", strings.ToUpper(f.Severity), loc, f.Message)
			if f.Suggestion != "" {
				fmt.Fprintf(w, "      suggestion: %s\n", f.Suggestion)
			}
		}
	}
	fmt.Fprintln(w)
	verdict := "PASS"
	if !o.Pass {
		verdict = "FAIL"
	}
	fmt.Fprintf(w, "%s — %d gate(s), %d passed, %d failed, %d finding(s) (%d error, %d warning)\n",
		verdict, o.Summary.Gates, o.Summary.Passed, o.Summary.Failed,
		o.Summary.Findings, o.Summary.Errors, o.Summary.Warnings)
	return nil
}

// NewVerifyCmd creates the `kapi verify` command: one project-aware quality
// gate that runs a project's bound brand, terminology, and QA checks in a
// single shot. It returns a single structured pass/fail plus actionable
// findings and exits non-zero on failure, so both CI and an AI assistant can
// loop on it: produce content, run verify, read findings, fix, re-run.
func (a *App) NewVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "verify [files...]",
		Short:   "Run a project's bound quality gates (brand, terminology, QA) in one shot",
		GroupID: "quality",
		Long: `Run a localization project's bound quality gates in a single shot and
return a single structured pass/fail plus actionable findings.

Gates (each runs only when the project binds the resource it needs):

  brand        If a brand voice is bound (defaults.brand_voice), score the
               source-language content against it. Fails when the score is
               below the threshold (default ` + strconv.Itoa(DefaultBrandMinScore) + `, override with --min-score).
  terminology  If a termbase is bound (defaults.termbase), check that target
               files use the required translations from the project glossary.
  qa           For translated target files, check placeholder/tag integrity
               against the source and flag untranslated/empty targets.

With no file arguments, verify inspects the project's content: brand on the
source files, terminology and QA on the target files derived from each
content item's target template and the project target languages.

Pass file paths to verify just those files instead.

Selecting gates (--brand/--terms/--qa) and missing bindings: with no gate flag,
every gate runs and a gate whose binding is missing (no defaults.brand_voice, no
defaults.termbase) is skipped silently — there is nothing to check. But when you
explicitly request a gate whose binding is missing, verify does not skip it: it
fails the gate with a clear "misconfigured" finding, so a CI run that asked for
--brand or --terms cannot pass by silently doing nothing. Bind the resource in
the .kapi project, drop the flag, or pass --no-fail to keep it report-only.

Exit codes: 0 pass, 3 when any gate fails (including a requested-but-unbound
gate), 1 for operational errors. Exit 3 means "not on-spec yet", not a crash — in
an assistant fix-loop, read the findings and fix. Pass --no-fail to always exit 0
(report mode) when looping; omit it for CI gating.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runVerify(cmd, args)
		},
	}

	AddProjectFlag(cmd)
	cmd.Flags().String("source-lang", "", "source language (overrides the project's source_language)")
	cmd.Flags().Bool("brand", false, "run only the brand gate (combine with --terms/--qa to select several)")
	cmd.Flags().Bool("terms", false, "run only the terminology gate")
	cmd.Flags().Bool("qa", false, "run only the QA gate")
	cmd.Flags().Int("min-score", DefaultBrandMinScore, "brand compliance score below which the brand gate fails")
	cmd.Flags().String("locale", "", "scope terminology and QA to a single target locale (e.g. fr)")
	cmd.Flags().String("termbase", "", "named termbase or path to a glossary (defaults to the project termbase)")
	cmd.Flags().Bool("json", false, "output the structured result as JSON")
	cmd.Flags().Bool("no-fail", false, "report only: exit 0 even when a gate fails (verdict is in the output/--json). Use inside an assistant fix-loop; omit for CI gating.")
	return cmd
}

// gateSelection records which gates the user asked to run. When no gate flag
// is set, every gate runs (explicit is false). When at least one gate flag is
// set, only the named gates run and explicit is true — which turns a gate whose
// project binding is missing from a silent skip into a reported failure, so a CI
// user who wrote `--brand`/`--terms` learns the gate is misconfigured rather
// than seeing a false pass.
type gateSelection struct {
	brand    bool
	terms    bool
	qa       bool
	explicit bool
}

// cmdContext returns the command's context, or context.Background() when the
// command was invoked outside cobra's Execute (e.g. in unit tests) and has no
// context set. Format readers select on ctx.Done(), so a nil context panics.
func cmdContext(cmd *cobra.Command) context.Context {
	if cmd != nil {
		if ctx := cmd.Context(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

func resolveGateSelection(cmd *cobra.Command) gateSelection {
	b, _ := cmd.Flags().GetBool("brand")
	t, _ := cmd.Flags().GetBool("terms")
	q, _ := cmd.Flags().GetBool("qa")
	if !b && !t && !q {
		return gateSelection{brand: true, terms: true, qa: true, explicit: false}
	}
	return gateSelection{brand: b, terms: t, qa: q, explicit: true}
}

// runVerify orchestrates the verify gates, emits the structured result, and
// maps the verdict to an exit code (0 pass, 3 gate fail, unless --no-fail).
func (a *App) runVerify(cmd *cobra.Command, args []string) error {
	out, err := a.computeVerify(cmd, args)
	if err != nil {
		return err
	}
	if err := output.Print(cmd, out); err != nil {
		return err
	}
	if !out.Pass {
		// Report mode: an assistant looping on verify reads `pass`/findings from the
		// output and fixes them — a not-yet-passing gate is expected feedback, not a
		// failure, so don't exit non-zero (which a shell or `set -e` treats as error).
		// CI gating omits --no-fail and gets the non-zero exit.
		if noFail, _ := cmd.Flags().GetBool("no-fail"); noFail {
			return nil
		}
		return ErrQualityGate
	}
	return nil
}

// computeVerify runs the selected gates and returns the structured result
// without printing or mapping to an exit code. Both `kapi verify` and the
// Claude Code stop hook (`kapi hook stop`) call this so they evaluate a project
// identically. The returned error is operational (no project, load failure);
// a failing gate is reported in VerifyOutput.Pass, not as an error.
func (a *App) computeVerify(cmd *cobra.Command, args []string) (VerifyOutput, error) {
	a.InitRegistries()

	// The verify path threads cmd.Context() into ctx-aware TM/termbase lookups
	// (e.g. resolveProjectGlossary). When computeVerify runs outside cobra's
	// Execute — the Stop hook builds a fresh NewVerifyCmd(), and tests call
	// runVerify directly — that context is nil, which panics deep in
	// database/sql and then deadlocks on the deferred store Close. Seed a real
	// context once here so every downstream cmd.Context() is non-nil.
	if cmd != nil && cmd.Context() == nil {
		cmd.SetContext(context.Background())
	}

	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		// Operational error (no project) → exit 1, the cobra default.
		return VerifyOutput{}, err
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return VerifyOutput{}, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)

	sel := resolveGateSelection(cmd)
	localeFilter, _ := cmd.Flags().GetString("locale")

	// Resolve the source language: explicit --source-lang flag wins, else the
	// project's declared source language, else fall back to "en".
	sourceLang, _ := cmd.Flags().GetString("source-lang")
	if sourceLang == "" {
		sourceLang = string(proj.Defaults.SourceLanguage)
	}
	if sourceLang == "" {
		sourceLang = "en"
	}
	a.SourceLang = sourceLang

	var gates []VerifyGateResult

	// --- brand gate -------------------------------------------------------
	if sel.brand {
		gate, err := a.verifyBrand(cmd, proj, root, args)
		if err != nil {
			return VerifyOutput{}, err
		}
		switch {
		case gate != nil:
			gates = append(gates, *gate)
		case sel.explicit:
			// --brand was requested but the project binds no brand voice. Fail
			// loudly rather than skip, so the misconfiguration is visible.
			gates = append(gates, unboundGate(gateBrand, "defaults.brand_voice", "--brand"))
		}
	}

	// --- terminology gate binding check ----------------------------------
	// The terminology gate needs a bound termbase to enforce against. When one
	// is bound it runs; when not, an explicitly requested gate fails (loud
	// misconfiguration) while a default run skips it silently.
	runTerms := false
	if sel.terms {
		bound, err := a.projectTermbaseBound(cmd)
		if err != nil {
			return VerifyOutput{}, err
		}
		switch {
		case bound:
			runTerms = true
		case sel.explicit:
			gates = append(gates, unboundGate(gateTerms, "defaults.termbase", "--terms"))
		}
	}

	// --- terminology + qa gates ------------------------------------------
	if runTerms || sel.qa {
		// Resolve the (source, target, locale) units to inspect: either the
		// explicit file args, or the project's content × target languages.
		units, err := a.resolveVerifyUnits(cmd, proj, root, args, localeFilter)
		if err != nil {
			return VerifyOutput{}, err
		}

		if runTerms {
			termGate, err := a.verifyTerminology(cmd, units)
			if err != nil {
				return VerifyOutput{}, err
			}
			gates = append(gates, termGate)
		}
		if sel.qa {
			qaGate, err := a.verifyQA(cmdContext(cmd), units)
			if err != nil {
				return VerifyOutput{}, err
			}
			gates = append(gates, qaGate)
		}
	}

	return buildVerifyOutput(gates), nil
}

// buildVerifyOutput aggregates per-gate results into the final structured
// output, computing the overall pass/fail and summary counts.
func buildVerifyOutput(gates []VerifyGateResult) VerifyOutput {
	out := VerifyOutput{Pass: true, Gates: gates}
	out.Summary.Gates = len(gates)
	for i := range out.Gates {
		sortFindings(out.Gates[i].Findings)
	}
	for _, g := range gates {
		if g.Pass {
			out.Summary.Passed++
		} else {
			out.Summary.Failed++
			out.Pass = false
		}
		for _, f := range g.Findings {
			out.Summary.Findings++
			switch f.Severity {
			case "error":
				out.Summary.Errors++
			case "warning":
				out.Summary.Warnings++
			}
		}
	}
	if out.Gates == nil {
		out.Gates = []VerifyGateResult{}
	}
	return out
}

// unboundGate returns a failing gate result for a gate the user explicitly
// requested (e.g. --brand) whose required project binding is missing. Surfacing
// it as a failure — rather than silently skipping — means a CI user learns the
// gate is misconfigured instead of seeing a false pass. The verdict is a normal
// gate failure, so --no-fail still downgrades it to report-only (exit 0).
func unboundGate(gate, binding, flag string) VerifyGateResult {
	return VerifyGateResult{
		Gate: gate,
		Pass: false,
		Findings: []VerifyFinding{{
			Gate:       gate,
			Severity:   "error",
			Message:    fmt.Sprintf("%s gate was requested with %s but the project binds no %s — nothing to check", gate, flag, binding),
			Suggestion: fmt.Sprintf("add %s to the .kapi project, or drop %s to skip this gate", binding, flag),
		}},
	}
}

// projectTermbaseBound reports whether the terminology gate has a termbase to
// enforce against: a --termbase flag, a defaults.termbase binding, or the
// convention .kapi/termbase.db. It mirrors the resolution the gate itself uses
// (resolveProjectTermbasePath), so "bound" means the same thing here and there.
func (a *App) projectTermbaseBound(cmd *cobra.Command) (bool, error) {
	tbPath, err := a.resolveProjectTermbasePath(cmd)
	if err != nil {
		return false, err
	}
	return tbPath != "", nil
}

// --- brand gate -------------------------------------------------------------

// verifyBrand scores the source-language content against the project's bound
// brand voice profile. Returns nil (no gate) when the project binds no brand
// voice — the gate only runs when there is something to check. Reuses the
// brand check path (NewBrandVocabCheckTool + CalculateScore).
func (a *App) verifyBrand(cmd *cobra.Command, proj *project.KapiProject, root string, args []string) (*VerifyGateResult, error) {
	profile, _, found, err := a.resolveProjectBrandProfile(cmd, "", "")
	if err != nil {
		return nil, err
	}
	if !found || profile == nil {
		return nil, nil
	}

	minScore, _ := cmd.Flags().GetInt("min-score")

	// Source files to score: explicit args, else the project's source content.
	files, err := a.brandSourceFiles(proj, root, args)
	if err != nil {
		return nil, err
	}

	gate := VerifyGateResult{Gate: gateBrand, Pass: true, Findings: []VerifyFinding{}}
	ctx := cmdContext(cmd)

	var allFindings []brand.BrandVoiceFinding
	for _, f := range files {
		blocks, rerr := a.readBlocks(ctx, f, a.SourceLang)
		if rerr != nil {
			return nil, fmt.Errorf("brand: read %s: %w", f, rerr)
		}
		vocab := coretools.NewBrandVocabCheckTool(profile, nil)
		for _, b := range blocks {
			findings := runBrandVocabOnBlock(ctx, vocab, b)
			for _, fd := range findings {
				gate.Findings = append(gate.Findings, brandFindingToVerify(f, fd))
			}
			allFindings = append(allFindings, findings...)
		}
	}

	score := brand.CalculateScore(allFindings)
	if score.Overall < minScore {
		gate.Pass = false
		// Lead with a summary finding so the assistant sees the score gap
		// even when individual term findings are sparse.
		gate.Findings = append([]VerifyFinding{{
			Gate:     gateBrand,
			Severity: "error",
			Message:  fmt.Sprintf("brand compliance score %d is below the required minimum %d", score.Overall, minScore),
		}}, gate.Findings...)
	}
	return &gate, nil
}

// runBrandVocabOnBlock runs the brand vocab check tool over a single block and
// returns the findings it recorded on the block's annotation/properties.
func runBrandVocabOnBlock(ctx context.Context, vocab *coretools.BrandVocabCheckTool, b *model.Block) []brand.BrandVoiceFinding {
	part := &model.Part{Type: model.PartBlock, Resource: b}
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- vocab.Process(ctx, in, out)
	}()
	for range out { //nolint:revive // drain
	}
	if err := <-errc; err != nil {
		return nil
	}
	if ann, ok := model.AnnoAs[*brand.BrandVoiceAnnotation](b, "brand-voice"); ok {
		return ann.Findings
	}
	if raw := b.Properties["brand-vocab-findings"]; raw != "" {
		var fs []brand.BrandVoiceFinding
		if json.Unmarshal([]byte(raw), &fs) == nil {
			return fs
		}
	}
	return nil
}

func brandFindingToVerify(file string, f brand.BrandVoiceFinding) VerifyFinding {
	sev := "warning"
	switch f.Severity {
	case brand.SeverityMajor, brand.SeverityCritical:
		sev = "error"
	}
	return VerifyFinding{
		Gate:       gateBrand,
		File:       file,
		Severity:   sev,
		Message:    f.Message,
		Suggestion: f.Suggestion,
	}
}

// brandSourceFiles returns the source files the brand gate should score.
func (a *App) brandSourceFiles(proj *project.KapiProject, root string, args []string) ([]string, error) {
	if len(args) > 0 {
		return resolveFiles(args)
	}
	files, err := a.projectSourceFiles(proj, root)
	if err != nil {
		return nil, err
	}
	return files, nil
}

// projectSourceFiles resolves every source file declared by the project's
// content items (the source-side glob, with {lang} expansion not applied — the
// content path is the source).
func (a *App) projectSourceFiles(proj *project.KapiProject, root string) ([]string, error) {
	ctx := project.NewProjectContext(proj, filepath.Join(root, "x.kapi"))
	resolved, err := ctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil, err
	}
	var files []string
	seen := map[string]bool{}
	for _, rf := range resolved {
		if seen[rf.Path] {
			continue
		}
		seen[rf.Path] = true
		files = append(files, rf.Path)
	}
	return files, nil
}

// --- verify units (source/target pairing) ----------------------------------

// verifyUnit is one source file paired with one target file for a locale.
// The QA and terminology gates read the source file for source text and the
// target file for translated text, then pair blocks by name.
type verifyUnit struct {
	sourcePath string
	targetPath string
	locale     string
	// displayPath is the path reported in findings (the target file, relative
	// to the project root when possible).
	displayPath string
}

// resolveVerifyUnits builds the list of (source, target, locale) units the
// terminology and QA gates inspect. With explicit file args, each arg is
// treated as a target file paired with itself as the source (so monolingual
// QA still flags placeholder/empty issues against the file's own content) —
// unless the file matches a project content target template, in which case the
// matching source file is paired. With no args, units come from the project's
// content × target languages.
func (a *App) resolveVerifyUnits(cmd *cobra.Command, proj *project.KapiProject, root string, args []string, localeFilter string) ([]verifyUnit, error) {
	if len(args) > 0 {
		return a.unitsFromArgs(proj, root, args, localeFilter)
	}
	return a.unitsFromProject(proj, root, localeFilter)
}

// unitsFromProject expands every content item against the project's target
// languages, pairing each source file with its derived target file per locale.
func (a *App) unitsFromProject(proj *project.KapiProject, root string, localeFilter string) ([]verifyUnit, error) {
	ctx := project.NewProjectContext(proj, filepath.Join(root, "x.kapi"))
	resolved, err := ctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil, err
	}

	var units []verifyUnit
	for _, rf := range resolved {
		if rf.Item == nil || rf.Item.Target == "" {
			continue
		}
		locales := rf.Item.ResolvedTargetLanguages(nil, proj.Defaults)
		for _, loc := range locales {
			if localeFilter != "" && string(loc) != localeFilter {
				continue
			}
			targetPath := expandTargetTemplate(rf.Item.Target, rf.Relative, string(loc), root)
			rel, relErr := filepath.Rel(root, targetPath)
			if relErr != nil {
				rel = targetPath
			}
			units = append(units, verifyUnit{
				sourcePath:  rf.Path,
				targetPath:  targetPath,
				locale:      string(loc),
				displayPath: rel,
			})
		}
	}
	return units, nil
}

// unitsFromArgs treats each file argument as a target file. When the file
// matches a project content target template, the source file and locale are
// recovered so QA/terminology run bilingually; otherwise the file is paired
// with itself and the locale falls back to --locale or the first project
// target language.
func (a *App) unitsFromArgs(proj *project.KapiProject, root string, args []string, localeFilter string) ([]verifyUnit, error) {
	files, err := resolveFiles(args)
	if err != nil {
		return nil, err
	}
	var units []verifyUnit
	for _, f := range files {
		abs, _ := filepath.Abs(f)
		src, loc, ok := matchTargetToSource(proj, root, abs)
		if !ok {
			// Monolingual fallback: pair the file with itself; locale from
			// --locale or the first project target.
			loc = localeFilter
			if loc == "" && len(proj.Defaults.TargetLanguages) > 0 {
				loc = string(proj.Defaults.TargetLanguages[0])
			}
			src = abs
		}
		if localeFilter != "" && loc != localeFilter {
			continue
		}
		rel, relErr := filepath.Rel(root, abs)
		if relErr != nil {
			rel = f
		}
		units = append(units, verifyUnit{
			sourcePath:  src,
			targetPath:  abs,
			locale:      loc,
			displayPath: rel,
		})
	}
	return units, nil
}

// matchTargetToSource finds the content item whose target template (for some
// project target language) expands to the given target file, returning the
// matching source file and locale. Returns ok=false when no item matches.
func matchTargetToSource(proj *project.KapiProject, root, targetAbs string) (sourcePath, locale string, ok bool) {
	ctxReg := registry.NewFormatRegistry()
	pctx := project.NewProjectContext(proj, filepath.Join(root, "x.kapi"))
	resolved, err := pctx.ResolveContent(ctxReg)
	if err != nil {
		return "", "", false
	}
	for _, rf := range resolved {
		if rf.Item == nil || rf.Item.Target == "" {
			continue
		}
		for _, loc := range rf.Item.ResolvedTargetLanguages(nil, proj.Defaults) {
			candidate := expandTargetTemplate(rf.Item.Target, rf.Relative, string(loc), root)
			candAbs, _ := filepath.Abs(candidate)
			if candAbs == targetAbs {
				return rf.Path, string(loc), true
			}
		}
	}
	return "", "", false
}

// expandTargetTemplate expands a content item's target template against a
// source file (relative to the project root) and a locale. {lang} → locale and
// "*" → the source basename without extension; the result is rooted at the
// project directory. Mirrors resolveMergeOutputPath's expansion.
func expandTargetTemplate(tmpl, sourceRel, locale, root string) string {
	out := strings.ReplaceAll(tmpl, "{lang}", locale)
	base := strings.TrimSuffix(filepath.Base(sourceRel), filepath.Ext(sourceRel))
	out = strings.ReplaceAll(out, "*", base)
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	return out
}

// --- terminology gate -------------------------------------------------------

// verifyTerminology term-checks each target file against the project glossary,
// reusing core/tools.NewTermCheckTool. The glossary is resolved per target
// locale from the project termbase (resolveProjectGlossary). A locale with no
// glossary entries contributes no findings; a missing target file
// (untranslated) is flagged by the QA gate, so terminology skips it.
func (a *App) verifyTerminology(cmd *cobra.Command, units []verifyUnit) (VerifyGateResult, error) {
	ctx := cmdContext(cmd)
	gate := VerifyGateResult{Gate: gateTerms, Pass: true, Findings: []VerifyFinding{}}

	// Cache the glossary per locale — building it opens the termbase.
	glossaryByLocale := map[string][]coretools.GlossaryEntry{}
	glossaryFor := func(locale string) ([]coretools.GlossaryEntry, error) {
		if g, ok := glossaryByLocale[locale]; ok {
			return g, nil
		}
		g, err := a.resolveProjectGlossary(cmd, locale)
		if err != nil {
			return nil, err
		}
		glossaryByLocale[locale] = g
		return g, nil
	}

	for _, u := range units {
		glossary, err := glossaryFor(u.locale)
		if err != nil {
			return gate, err
		}
		if len(glossary) == 0 {
			// No bound glossary for this locale → nothing to enforce.
			continue
		}
		blocks, missing, err := a.bilingualBlocks(ctx, u)
		if err != nil {
			return gate, err
		}
		if missing {
			// Untranslated target — terminology can't be checked; QA reports
			// the missing file, so skip here to avoid duplicate noise.
			continue
		}
		cfg := &coretools.TermCheckConfig{
			Glossary:     glossary,
			TargetLocale: model.LocaleID(u.locale),
		}
		tc := coretools.NewTermCheckTool(cfg)
		for _, b := range blocks {
			runCheckTool(ctx, tc, b)
			if b.Properties[coretools.PropTermCheckPassed] == "false" {
				gate.Pass = false
				msg := b.Properties[coretools.PropTermCheckErrors]
				for m := range strings.SplitSeq(msg, "; ") {
					if strings.TrimSpace(m) == "" {
						continue
					}
					gate.Findings = append(gate.Findings, VerifyFinding{
						Gate:       gateTerms,
						File:       u.displayPath,
						Locale:     u.locale,
						Severity:   "error",
						Message:    m,
						Suggestion: "use the glossary's required translation",
					})
				}
			}
		}
	}
	return gate, nil
}

// --- qa gate ----------------------------------------------------------------

// verifyQA checks placeholder/tag integrity against the source and flags
// untranslated/empty targets for each target file, reusing
// core/tools.NewQACheckTool.
func (a *App) verifyQA(ctx context.Context, units []verifyUnit) (VerifyGateResult, error) {
	gate := VerifyGateResult{Gate: gateQA, Pass: true, Findings: []VerifyFinding{}}

	for _, u := range units {
		blocks, missing, err := a.bilingualBlocks(ctx, u)
		if err != nil {
			return gate, err
		}
		if missing {
			gate.Pass = false
			gate.Findings = append(gate.Findings, VerifyFinding{
				Gate:       gateQA,
				File:       u.displayPath,
				Locale:     u.locale,
				Severity:   "error",
				Message:    "target file is missing — content is untranslated",
				Suggestion: "translate the source content for this locale",
			})
			continue
		}

		cfg := coretools.NewQACheckConfig(model.LocaleID(u.locale))
		// Add default placeholder patterns so the QA gate flags dropped
		// placeholders even in plain-text formats where the reader does not
		// extract them as inline codes (e.g. {name}, {{var}}, %s, $t). The
		// inline-code difference check still runs for formats that do extract
		// codes; the pattern check is additive.
		cfg.Patterns = append(cfg.Patterns, defaultPlaceholderPatterns()...)
		qa := coretools.NewQACheckTool(cfg)
		for _, b := range blocks {
			runCheckTool(ctx, qa, b)
			for _, f := range check.Findings(tool.NewBlockView(b)) {
				failing := qaFindingFails(f)
				sev := verifySeverity(f.Severity)
				if failing {
					sev = "error"
				}
				gate.Findings = append(gate.Findings, VerifyFinding{
					Gate:       gateQA,
					File:       u.displayPath,
					Locale:     u.locale,
					Severity:   sev,
					Message:    f.Message,
					Suggestion: qaFindingSuggestion(f),
				})
				if failing {
					gate.Pass = false
				}
			}
		}
	}
	return gate, nil
}

// verifySeverity maps a check.Severity to the "error"/"warning" severity the
// verify output uses: critical/major problems are errors, minor/neutral are
// warnings.
func verifySeverity(s check.Severity) string {
	switch s {
	case check.SeverityCritical, check.SeverityMajor:
		return "error"
	default:
		return "warning"
	}
}

// qaFailingCategories are the QA finding categories verify treats as gate
// failures: integrity problems that break the translation (dropped/extra
// placeholders or tags, missing required codes, untranslated/empty targets).
// Cosmetic issues (whitespace, doubled words, length ratios) are reported as
// warnings but do not fail the gate.
var qaFailingCategories = map[string]bool{
	"empty-target":                  true,
	"pattern-mismatch":              true,
	"missing-code":                  true,
	"extra-code":                    true,
	"code-order":                    true,
	"non-deletable-span-missing":    true,
	"non-cloneable-span-duplicated": true,
	"target-same-as-source":         true,
}

// qaFindingFails reports whether a QA finding should fail the verify QA gate.
func qaFindingFails(f check.Finding) bool {
	if qaFailingCategories[f.Category] {
		return true
	}
	// Any major/critical finding fails regardless of category.
	return f.Severity == check.SeverityMajor || f.Severity == check.SeverityCritical
}

// qaFindingSuggestion returns a short remediation hint for the assistant for the
// integrity finding categories verify cares about.
func qaFindingSuggestion(f check.Finding) string {
	switch f.Category {
	case "empty-target":
		return "translate the source content for this entry"
	case "pattern-mismatch", "missing-code", "non-deletable-span-missing":
		return "keep every placeholder/tag from the source in the target"
	case "extra-code", "non-cloneable-span-duplicated":
		return "remove placeholders/tags that are not present in the source"
	case "target-same-as-source":
		return "translate the target — it is identical to the source"
	}
	return ""
}

// defaultPlaceholderPatterns returns QA patterns that flag dropped placeholders
// in plain-text formats. Each pattern requires every placeholder occurrence in
// the source to appear verbatim in the target.
func defaultPlaceholderPatterns() []coretools.QAPattern {
	specs := []struct{ src, desc string }{
		{`\{\{[^}]+\}\}`, "Mustache/Handlebars placeholder ({{...}}) dropped in target"},
		{`\{[^{}]+\}`, "Brace placeholder ({...}) dropped in target"},
		{`%(?:\d+\$)?[sdfv@]`, "printf placeholder (%s, %d, ...) dropped in target"},
		{`\$\{[^}]+\}`, "Template literal placeholder (${...}) dropped in target"},
	}
	patterns := make([]coretools.QAPattern, 0, len(specs))
	for _, s := range specs {
		patterns = append(patterns, coretools.QAPattern{
			Enabled:     true,
			Source:      s.src,
			Target:      "<same>",
			Description: s.desc,
		})
	}
	return patterns
}

// --- shared block helpers ---------------------------------------------------

// bilingualBlocks reads the source file (source text) and the target file
// (translated text), overlaying each target block's text onto the matching
// source block as the unit's target locale. Blocks are paired by Name (the
// format's stable key, e.g. the JSON key path), falling back to ID. Source
// blocks with no matching target keep an empty target so QA flags them as
// untranslated. Returns missing=true when the target file does not exist.
func (a *App) bilingualBlocks(ctx context.Context, u verifyUnit) ([]*model.Block, bool, error) {
	if _, err := os.Stat(u.targetPath); err != nil {
		if os.IsNotExist(err) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("stat %s: %w", u.targetPath, err)
	}

	sourceBlocks, err := a.readBlocks(ctx, u.sourcePath, a.SourceLang)
	if err != nil {
		return nil, false, fmt.Errorf("read source %s: %w", u.sourcePath, err)
	}

	// When source == target (monolingual arg fallback), the file's own blocks
	// serve as both source and target.
	var targetBlocks []*model.Block
	if u.targetPath == u.sourcePath {
		targetBlocks = sourceBlocks
	} else {
		targetBlocks, err = a.readBlocks(ctx, u.targetPath, a.SourceLang)
		if err != nil {
			return nil, false, fmt.Errorf("read target %s: %w", u.targetPath, err)
		}
	}

	targetByKey := make(map[string]*model.Block, len(targetBlocks))
	for _, tb := range targetBlocks {
		targetByKey[blockKey(tb)] = tb
	}

	locale := model.LocaleID(u.locale)
	for _, sb := range sourceBlocks {
		sb.SourceLocale = model.LocaleID(a.SourceLang)
		tb, ok := targetByKey[blockKey(sb)]
		if !ok {
			continue // no target → empty; QA flags as untranslated.
		}
		// Carry the target text and runs onto the source block as the target
		// locale so QA can compare inline codes structurally.
		if runs := tb.SourceRuns(); len(runs) > 0 {
			sb.SetTargetRuns(locale, runs)
		} else {
			sb.SetTargetText(locale, tb.SourceText())
		}
	}
	return sourceBlocks, false, nil
}

// blockKey returns the stable pairing key for a block: its Name when set
// (the format's content key), else its ID.
func blockKey(b *model.Block) string {
	if b.Name != "" {
		return b.Name
	}
	return b.ID
}

// runCheckTool runs an annotate-only block tool (qa / term-check) over a
// single block in place. The tool records its findings on block.Properties.
func runCheckTool(ctx context.Context, t interface {
	Process(context.Context, <-chan *model.Part, chan<- *model.Part) error
}, b *model.Block) {
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- t.Process(ctx, in, out)
	}()
	for range out { //nolint:revive // drain
	}
	<-errc
}

// readBlocks reads a file through its detected format reader and returns the
// translatable blocks, with sourceLang as the source locale. It does not write
// any output — verify only inspects content.
func (a *App) readBlocks(ctx context.Context, path, sourceLang string) ([]*model.Block, error) {
	fmtName := a.FormatFlag
	if fmtName == "" {
		ext := filepath.Ext(path)
		detected, err := a.FormatReg.DetectByExtension(ext)
		if err != nil {
			return nil, fmt.Errorf("detect format for %q: %w", filepath.Base(path), err)
		}
		fmtName = string(detected)
	}

	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return nil, fmt.Errorf("no reader for %q: %w", fmtName, err)
	}
	defer reader.Close()

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     firstNonEmpty(a.Encoding, "UTF-8"),
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("open %q: %w", filepath.Base(path), err)
	}

	var blocks []*model.Block
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return nil, fmt.Errorf("read %q: %w", filepath.Base(path), result.Error)
		}
		if result.Part == nil || result.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := result.Part.Resource.(*model.Block); ok && b.Translatable {
			blocks = append(blocks, b)
		}
	}
	return blocks, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// sortFindings orders findings deterministically (by file, locale, severity,
// message) so JSON output and tests are stable. Currently applied per gate at
// build time; exported helper kept small for reuse.
func sortFindings(fs []VerifyFinding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].File != fs[j].File {
			return fs[i].File < fs[j].File
		}
		if fs[i].Locale != fs[j].Locale {
			return fs[i].Locale < fs[j].Locale
		}
		if fs[i].Severity != fs[j].Severity {
			return fs[i].Severity < fs[j].Severity
		}
		return fs[i].Message < fs[j].Message
	})
}
