package host

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/encoding"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/host/output"
)

// Gate names for the project gates (`kapi check --ship`).
const (
	gateVoice  = "voice"
	gateTerms  = "terminology"
	gateQA     = "qa"
	gateShip   = "ship"
	gateSource = "source"
)

// DefaultVoiceMinScore is the voice compliance score below which the voice
// gate fails when the user does not override it with --min-score.
const DefaultVoiceMinScore = 80

// verifyFinding is a single actionable problem found by one of the verify
// gates. The shape is shared by the human and JSON renderers and is the unit
// an AI assistant reads, fixes, and re-runs against.
type verifyFinding struct {
	Gate string `json:"gate"`
	// File is the project-relative path, the way `kapi check` names it. An
	// absolute path names a machine rather than a repository: it is unusable in
	// a CI log or a recording, and it leaks the directory layout.
	File string `json:"file,omitempty"`
	// Block is the block the finding sits in, when the gate resolves one.
	Block      string `json:"block,omitempty"`
	Locale     string `json:"locale,omitempty"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// verifyGateResult is the outcome of one gate: whether it passed and the
// findings it produced.
type verifyGateResult struct {
	Gate     string          `json:"gate"`
	Pass     bool            `json:"pass"`
	Findings []verifyFinding `json:"findings"`
}

// verifySummary carries the aggregate counts for a verify run.
type verifySummary struct {
	Gates    int `json:"gates"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Findings int `json:"findings"`
	Errors   int `json:"errors"`   // findings with severity "error"
	Warnings int `json:"warnings"` // findings with severity "warning"
}

// verifyOutput is the single structured result of a project-gates run.
type verifyOutput struct {
	Pass    bool               `json:"pass"`
	Gates   []verifyGateResult `json:"gates"`
	Summary verifySummary      `json:"summary"`
}

// FormatText renders the verify result as a human-readable summary,
// implementing output.TextFormatter.
func (o verifyOutput) FormatText(w io.Writer) error {
	gates := output.NewTable(w).Accent(0).Headers("gate", "result", "findings")
	gs := gates.Styles()
	for _, g := range o.Gates {
		result := gs.Success.Render("PASS")
		if !g.Pass {
			result = gs.Error.Render("FAIL")
		}
		gates.Rowf(g.Gate, result, len(g.Findings))
	}
	gates.Render()

	// The findings follow the gate roll-up rather than nesting inside it: one
	// aligned block per gate, so the gate column above stays a scannable list.
	for _, g := range o.Gates {
		if len(g.Findings) == 0 {
			continue
		}
		fmt.Fprintln(w)
		output.Title(w, g.Gate+":")
		t := output.NewTable(w).Headers("severity", "location", "message")
		s := t.Styles()
		for _, f := range g.Findings {
			loc := f.File
			if f.Block != "" {
				if loc != "" {
					loc += ":"
				}
				loc += f.Block
			}
			if f.Locale != "" {
				if loc != "" {
					loc += " "
				}
				loc += "[" + f.Locale + "]"
			}
			t.Row(severityCell(s, f.Severity), s.Dim(loc), f.Message)
			if f.Suggestion != "" {
				t.Row("", "", s.Muted.Render("↳ "+f.Suggestion))
			}
		}
		t.Render()
	}
	fmt.Fprintln(w)
	verdict := gs.Success.Render("PASS")
	if !o.Pass {
		verdict = gs.Error.Render("FAIL")
	}
	fmt.Fprintf(w, "%s — %d gate(s), %d passed, %d failed, %d finding(s) (%d error, %d warning)\n",
		verdict, o.Summary.Gates, o.Summary.Passed, o.Summary.Failed,
		o.Summary.Findings, o.Summary.Errors, o.Summary.Warnings)
	return nil
}

// severityCell renders a finding's severity with the style its urgency asks
// for. It spans both severity vocabularies in play: the verify gates emit
// error/warning, core/check emits critical/major/minor.
func severityCell(s *output.Styles, sev string) string {
	label := strings.ToUpper(sev)
	switch strings.ToLower(sev) {
	case "error", "critical":
		return s.Error.Render(label)
	case "warning", "major":
		return s.Warn.Render(label)
	default:
		return s.Muted.Render(label)
	}
}

// gateSelection records which gates the user asked to run. With no --gate, every
// gate runs (explicit is false). With at least one, only the named gates run and
// explicit is true — which turns a gate whose project binding is missing from a
// silent skip into a reported failure, so a CI user who named a gate learns it is
// misconfigured rather than seeing a false pass.
type gateSelection struct {
	voice    bool
	terms    bool
	qa       bool
	explicit bool
}

// gateFlagName is the flag that names which project gates to run. One flag
// naming gates by their own names, rather than a boolean per gate: the gates
// share their names with other things a check command already flags (--voice is
// the style-similarity opt-in, --terms is the do-not-translate boolean
// elsewhere), and a selector that collides with them is a selector a user
// cannot read off the help text.
const gateFlagName = "gate"

// selectableGates are the gate names --gate accepts, in the order they run.
var selectableGates = []string{gateVoice, gateTerms, gateQA}

// CmdContext returns the command's context, or context.Background() when the
// command was invoked outside cobra's Execute (e.g. in unit tests) and has no
// context set. Format readers select on ctx.Done(), so a nil context panics.
func CmdContext(cmd Command) context.Context {
	if cmd != nil {
		if ctx := cmd.Context(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

func resolveGateSelection(cmd Command) (gateSelection, error) {
	named, _ := cmd.Flags().GetStringSlice(gateFlagName)
	if len(named) == 0 {
		return gateSelection{voice: true, terms: true, qa: true}, nil
	}
	sel := gateSelection{explicit: true}
	for _, n := range named {
		switch strings.ToLower(strings.TrimSpace(n)) {
		case gateVoice:
			sel.voice = true
		case gateTerms:
			sel.terms = true
		case gateQA:
			sel.qa = true
		default:
			return gateSelection{}, fmt.Errorf("unknown gate %q — pass one of: %s",
				n, strings.Join(selectableGates, ", "))
		}
	}
	return sel, nil
}

// RunVerify orchestrates the verify gates, emits the structured result, and
// maps the verdict to an exit code (0 pass, 3 gate fail, unless --no-fail).
func (a *App) RunVerify(cmd Command, args []string) error {
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
// without printing or mapping to an exit code. Both `kapi check --ship` and the
// Claude Code stop hook (`kapi hook stop`) call this so they evaluate a project
// identically. The returned error is operational (no project, load failure);
// a failing gate is reported in verifyOutput.Pass, not as an error.
func (a *App) computeVerify(cmd Command, args []string) (verifyOutput, error) {
	a.InitRegistries()

	// The verify path threads cmd.Context() into ctx-aware content memory/terms lookups
	// (e.g. ResolveTermRules). When computeVerify runs outside cobra's
	// Execute — the Stop hook builds a fresh EnvCommand with the verify flags, and tests call
	// RunVerify directly — that context is nil, which panics deep in
	// database/sql and then deadlocks on the deferred store Close. Seed a real
	// context once here so every downstream cmd.Context() is non-nil.
	if cmd != nil && cmd.Context() == nil {
		cmd.SetContext(context.Background())
	}

	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		// Operational error (no project) → exit 1, the cobra default.
		return verifyOutput{}, err
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return verifyOutput{}, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)
	// A gate run is where a silently-inert recipe field (automations:
	// without server:) is most misleading — surface it here and in status.
	a.WarnInertRecipeFields(cmd, proj)

	sel, err := resolveGateSelection(cmd)
	if err != nil {
		return verifyOutput{}, err
	}
	localeFilter, _ := cmd.Flags().GetString("locale")

	// The language the gates grade in: an explicit --source-lang, else the
	// project's declared source language, else the built-in default
	// (host/sourcelang.go). A gate that reads en-GB content as "en" holds it to
	// no vocabulary at all.
	sourceLang, _ := cmd.Flags().GetString(sourceLangFlag)
	a.SourceLang = ResolveSourceLocale(sourceLang, proj.Defaults.SourceLanguage)
	// And the encoding they read it in (host/encoding.go).
	a.ResolveEncoding(proj.Defaults.Encoding)

	var gates []verifyGateResult

	// --- voice gate -------------------------------------------------------
	if sel.voice {
		gate, err := a.verifyVoice(cmd, proj, root, args)
		if err != nil {
			return verifyOutput{}, err
		}
		switch {
		case gate != nil:
			gates = append(gates, *gate)
		case sel.explicit:
			// --voice was requested but the project binds no voice profile. Fail
			// loudly rather than skip, so the misconfiguration is visible.
			gates = append(gates, unboundGate(gateVoice, "defaults.voice"))
		}
	}

	// --- terminology gate binding check ----------------------------------
	// The terminology gate needs a bound terms to enforce against. When one
	// is bound it runs; when not, an explicitly requested gate fails (loud
	// misconfiguration) while a default run skips it silently.
	runTerms := false
	if sel.terms {
		bound, err := a.projectTermsBound(cmd)
		if err != nil {
			return verifyOutput{}, err
		}
		switch {
		case bound:
			runTerms = true
		case sel.explicit:
			gates = append(gates, unboundGate(gateTerms, "defaults.terms_source"))
		}
	}

	// --- terminology + qa gates ------------------------------------------
	if runTerms || sel.qa {
		// Resolve the (source, target, locale) units to inspect: either the
		// explicit file args, or the project's content × target languages.
		units, err := a.resolveVerifyUnits(cmd, proj, root, args, localeFilter)
		if err != nil {
			return verifyOutput{}, err
		}

		if runTerms {
			termGate, err := a.verifyTerminology(cmd, units)
			if err != nil {
				return verifyOutput{}, err
			}
			gates = append(gates, termGate)
		}
		if sel.qa {
			qaGate, err := a.verifyQA(cmd, proj, root, units)
			if err != nil {
				return verifyOutput{}, err
			}
			gates = append(gates, qaGate)
		}
	}

	// --- staleness gate --------------------------------------------------
	// Part of the default gate set, and it carries no flag of its own: the
	// other gates are bars you set, and this one is a fact about provenance —
	// a target's stamp either matches the context governing its file or it
	// does not. A run that named specific gates gets only those.
	if !sel.explicit {
		units, err := a.resolveVerifyUnits(cmd, proj, root, args, localeFilter)
		if err != nil {
			return verifyOutput{}, err
		}
		staleGate, judged, err := a.verifyStaleness(cmd, proj, root, units)
		if err != nil {
			return verifyOutput{}, err
		}
		if judged {
			gates = append(gates, staleGate)
		}
	}

	// --- ship gate (opt-in: the pre-release coverage bar) ----------------
	// By default the ship gate is *not* evaluated here — coverage is the job of
	// `kapi status`, and target drift is non-blocking. With --ship, a locale that
	// doesn't clear its ship gate fails verify (exit 3), for a release/tag check.
	if ship, _ := cmd.Flags().GetBool("ship"); ship && (proj.HasShipGates() || proj.HasSourceGate()) {
		shipUnits, err := a.resolveVerifyUnits(cmd, proj, root, args, localeFilter)
		if err != nil {
			return verifyOutput{}, err
		}
		if proj.HasShipGates() {
			shipGate, err := a.verifyShip(cmd, proj, root, shipUnits)
			if err != nil {
				return verifyOutput{}, err
			}
			gates = append(gates, shipGate)
		}
		if proj.HasSourceGate() {
			// The source gate measures the source, so with no files named it
			// reads the project's source content directly rather than the
			// per-locale pairing: a monolingual project has a source to gate
			// even though it resolves no unit. Named files are already their own
			// source in shipUnits.
			srcUnits := shipUnits
			if len(args) == 0 {
				srcUnits, err = a.SourceUnitsFromProject(proj, root)
				if err != nil {
					return verifyOutput{}, fmt.Errorf("resolve source content: %w", err)
				}
			}
			srcGate, err := a.verifySourceGate(CmdContext(cmd), proj, root, srcUnits)
			if err != nil {
				return verifyOutput{}, err
			}
			gates = append(gates, srcGate)
		}
	}

	return buildVerifyOutput(gates), nil
}

// verifySourceGate evaluates the project's source-readiness gate over the
// author's content. It is the source-side counterpart of verifyShip: it gates
// the source (authored → checked → approved), not the translations. Like the
// ship gate it is opt-in (--ship) — source drift never blocks an ordinary build.
func (a *App) verifySourceGate(ctx context.Context, proj *project.KapiProject, root string, units []VerifyUnit) (verifyGateResult, error) {
	sc, err := a.computeSourceReadiness(ctx, proj, root, units)
	if err != nil {
		return verifyGateResult{}, err
	}
	g := verifyGateResult{Gate: gateSource, Pass: true}
	if !sc.Gated || sc.Shippable {
		return g, nil
	}
	g.Pass = false
	for _, sf := range sc.Pending {
		g.Findings = append(g.Findings, verifyFinding{
			Gate:       gateSource,
			Severity:   "error",
			Message:    fmt.Sprintf("source %s readiness %d%% is below the required %d%%", sf.State, int(sf.Actual), sf.Required),
			Suggestion: "run the source checks (e.g. voice/terminology) and resolve findings, or relax the source gate",
		})
	}
	return g, nil
}

// verifyShip evaluates the project's ship gates over per-locale coverage. A
// locale that does not clear its gate produces one finding per unmet threshold
// and fails the gate. It is the enforcing counterpart of `kapi status`.
//
// It runs the project's bound checks first, so the ship gate answers the same
// guardrail question the checks gate in this very command answers: one invocation
// reporting `qa FAIL` beside `ship PASS` over one tree is a contradiction
// needing no second command to see (#2024).
func (a *App) verifyShip(cmd Command, proj *project.KapiProject, root string, units []VerifyUnit) (verifyGateResult, error) {
	ctx := CmdContext(cmd)
	excl, err := a.computeLoopCheckExclusions(ctx, cmd, proj, root, units)
	if err != nil {
		return verifyGateResult{}, err
	}
	cov, err := a.ComputeShipCoverage(ctx, proj, root, units, excl)
	if err != nil {
		return verifyGateResult{}, err
	}
	g := verifyGateResult{Gate: gateShip, Pass: true}
	for _, lc := range cov {
		scope := lc.Locale
		if lc.Collection != "" {
			scope = lc.Locale + "/" + lc.Collection
		}
		// A failing guardrail fails the gate whether or not the scope declared
		// one, for the reason the stale finding below gives: a coverage
		// threshold is a bar on how much is done, and this says some of what is
		// done is wrong.
		if lc.FailingChecks > 0 {
			g.Pass = false
			g.Findings = append(g.Findings, verifyFinding{
				Gate:     gateShip,
				Locale:   lc.Locale,
				Severity: "error",
				Message: fmt.Sprintf("%s: %d unit(s) fail the project's bound checks",
					scope, lc.FailingChecks),
				Suggestion: "fix the findings the qa gate lists for this locale, then re-run",
			})
		}
		// Stale pairings fail the gate whether or not the scope declared one.
		// A coverage threshold is a bar on how much is done; this is a statement
		// that some of what is done describes source the project has rewritten,
		// and no bar makes that shippable.
		if lc.Stale > 0 {
			g.Pass = false
			g.Findings = append(g.Findings, verifyFinding{
				Gate:     gateShip,
				Locale:   lc.Locale,
				Severity: "error",
				Message: fmt.Sprintf("%s: %d unit(s) stale — the source changed since the translation was decided",
					scope, lc.Stale),
				Suggestion: "re-review the stale units (kapi status --review) or retranslate them",
			})
		}
		if !lc.Gated || lc.Shippable {
			continue
		}
		g.Pass = false
		for _, sf := range lc.Pending {
			g.Findings = append(g.Findings, verifyFinding{
				Gate:       gateShip,
				Locale:     lc.Locale,
				Severity:   "error",
				Message:    fmt.Sprintf("%s: %s coverage %d%% is below the required %d%%", scope, sf.State, int(sf.Actual), sf.Required),
				Suggestion: "translate or review more content for this locale, or relax its ship gate",
			})
		}
	}
	return g, nil
}

// buildVerifyOutput aggregates per-gate results into the final structured
// output, computing the overall pass/fail and summary counts.
func buildVerifyOutput(gates []verifyGateResult) verifyOutput {
	out := verifyOutput{Pass: true, Gates: gates}
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
		out.Gates = []verifyGateResult{}
	}
	return out
}

// unboundGate returns a failing gate result for a gate the user explicitly
// named whose required project binding is missing. Surfacing it as a failure —
// rather than silently skipping — means a CI user learns the gate is
// misconfigured instead of seeing a false pass. The verdict is a normal gate
// failure, so --no-fail still downgrades it to report-only (exit 0).
func unboundGate(gate, binding string) verifyGateResult {
	flag := "--" + gateFlagName + " " + gate
	return verifyGateResult{
		Gate: gate,
		Pass: false,
		Findings: []verifyFinding{{
			Gate:       gate,
			Severity:   "error",
			Message:    fmt.Sprintf("%s gate was requested with %s but the project binds no %s — nothing to check", gate, flag, binding),
			Suggestion: fmt.Sprintf("add %s to the kapi.yaml recipe, or drop %s to skip this gate", binding, flag),
		}},
	}
}

// projectTermsBound reports whether the terminology gate has a vocabulary to
// enforce against: a --termstore flag, a profile's standalone `terms:`, a
// committed defaults.terms_source (.terms.json) resolved directly at check
// time, or concepts in the project's own store. It mirrors the resolution the
// gate itself uses (ResolveTermsStore / resolveProjectTermsSourcePath), so
// "bound" means the same thing here and there.
//
// The project store is the case that changed shape: its vocabulary tables exist
// from the store's first open, so their presence says nothing and "bound" can
// only mean the store holds a concept. Otherwise every project would report a
// terminology binding it does not have, and the gate would never be able to say
// there is nothing to check.
func (a *App) projectTermsBound(cmd Command) (bool, error) {
	sel, err := a.ResolveTermsStore(cmd, project.GovernancePoint{})
	if err != nil {
		return false, err
	}
	if sel.Path != "" {
		return true, nil
	}
	srcPath, err := a.resolveProjectTermsSourcePath(cmd, project.GovernancePoint{})
	if err != nil {
		return false, err
	}
	if srcPath != "" {
		return true, nil
	}
	if !sel.InProject() {
		return false, nil
	}
	ctx := CmdContext(cmd)
	db, err := a.ProjectDB(ctx, sel.Root)
	if err != nil {
		return false, err
	}
	return db.HasTerms(ctx)
}

// --- voice gate -------------------------------------------------------------

// verifyVoice scores the source-language content against the project's bound
// voice profile. Returns nil (no gate) when the project binds no voice
// profile — the gate only runs when there is something to check. Reuses the
// voice check path (NewVoiceVocabCheckTool + CalculateScore).
func (a *App) verifyVoice(cmd Command, proj *project.KapiProject, root string, args []string) (*verifyGateResult, error) {
	// The voice is resolved per file, at the point that file sits at, so a gate
	// over a governed project scores each file against the vocabulary in force
	// there. A project binding no voice anywhere resolves none for any file and
	// contributes no gate.
	voice, err := a.newCheckVoice(cmd)
	if err != nil {
		return nil, err
	}
	defer voice.close()

	minScore, _ := cmd.Flags().GetInt("min-score")

	// Source files to score: explicit args, else the project's source content.
	files, err := a.voiceSourceFiles(proj, root, args)
	if err != nil {
		return nil, err
	}

	gate := verifyGateResult{Gate: gateVoice, Pass: true, Findings: []verifyFinding{}}
	ctx := CmdContext(cmd)

	// The vocabulary the project decided, enforced beside the profile's lists
	// and resolved at the same point the profile is.
	terminology, err := a.newCheckTerms(cmd)
	if err != nil {
		return nil, err
	}

	// What each file IS to this project — the format it declares and the reader
	// config it declares for it, resolved the way the file checkset resolves it.
	// A gate that reads a document differently from the loop is not gating the
	// loop: it scores blocks the convergence never extracts.
	formats, err := a.newCheckFormats(cmd)
	if err != nil {
		return nil, err
	}

	governed := false
	// Formats this machine has no reader for. A voice profile can govern a
	// collection whose format comes from a plugin, so the gate has to run on a
	// machine that lacks it — reporting what it could not score rather than
	// refusing to score anything. The targeted gate that exists for such a
	// collection installs the plugin and checks it properly; this is the
	// project-wide sweep degrading, out loud.
	noReader := map[string]bool{}
	var allFindings []coreprofile.VoiceFinding
	for _, f := range files {
		profile, perr := voice.forFile(ctx, f)
		if perr != nil {
			return nil, perr
		}
		if profile == nil {
			continue
		}
		tb, terr := terminology.forFile(ctx, f)
		if terr != nil {
			return nil, terr
		}
		fmtName, fmtCfg := formats.forFile(a, f)
		blocks, rerr := a.readBlocksAs(ctx, f, fmtName, fmtCfg, a.SourceLocale())
		if rerr != nil {
			if errors.Is(rerr, registry.ErrUnknownFormat) {
				noReader[fmtName] = true
				continue
			}
			return nil, fmt.Errorf("voice: read %s: %w", f, rerr)
		}
		// Set only once a file has actually been READ. A project whose every
		// governed file needs an uninstalled plugin contributes no voice gate,
		// which is honest; claiming a gate that scored nothing is not.
		governed = true
		// Findings name the file the way `kapi check` does — relative to the
		// project, with the block — so one location format reads the same in a
		// terminal, a CI log and a recording, and none of them carries the
		// machine's directory layout.
		display := f
		if rel, ok := projectRelPath(root, f); ok {
			display = rel
		}
		vocab := coretools.NewVoiceVocabCheckTool(profile, tb).InSourceLocale(model.LocaleID(a.SourceLocale()))
		for _, b := range blocks {
			findings, verr := runVoiceVocabOnBlock(ctx, vocab, b)
			if verr != nil {
				return nil, fmt.Errorf("voice: %s: %w", f, verr)
			}
			for _, fd := range findings {
				gate.Findings = append(gate.Findings, voiceFindingToVerify(display, blockKey(b), fd))
			}
			allFindings = append(allFindings, findings...)
		}
		// The profile's document-scope rules — its required patterns — are judged
		// once over the file and reported against it with no block, the way the
		// file checkset reports them. They score beside the per-block findings:
		// a rule the profile counts is a rule the ship gate applies.
		docFindings := coreprofile.DocumentFindings(profile, documentText(blocks))
		for _, fd := range docFindings {
			gate.Findings = append(gate.Findings, voiceFindingToVerify(display, "", fd))
		}
		allFindings = append(allFindings, docFindings...)
	}

	a.warnUnreadableFormats(cmd, sortedFormatSet(noReader))

	if !governed {
		// Nothing bound a voice at any of these files — there is no gate to
		// report, which is what lets --voice say the binding is missing.
		return nil, nil
	}

	score := coreprofile.CalculateScore(allFindings)
	if score.Overall < minScore {
		gate.Pass = false
		// Lead with a summary finding so the assistant sees the score gap
		// even when individual term findings are sparse.
		gate.Findings = append([]verifyFinding{{
			Gate:     gateVoice,
			Severity: "error",
			Message:  fmt.Sprintf("voice compliance score %d is below the required minimum %d", score.Overall, minScore),
		}}, gate.Findings...)
	}
	return &gate, nil
}

// runVoiceVocabOnBlock runs the voice vocabulary check tool over a single block and
// returns the findings it recorded on the block's annotation/properties.
//
// Both failure modes are RETURNED rather than folded into "no findings". The
// caller feeds these into coreprofile.CalculateScore, so a checker that errored on
// every block used to produce an empty finding set, a perfect score, and
// `voice: PASS` — the voice gate silently disabled while CI went green. It
// shares RunCheckTool's driver rather than re-deriving it, so the two cannot
// drift on error handling again.
func runVoiceVocabOnBlock(ctx context.Context, vocab *coretools.VoiceVocabCheckTool, b *model.Block) ([]coreprofile.VoiceFinding, error) {
	if err := RunCheckTool(ctx, vocab, b); err != nil {
		return nil, err
	}
	if ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](b, "voice"); ok {
		return ann.Findings, nil
	}
	if raw := b.Properties["voice-vocab-findings"]; raw != "" {
		var fs []coreprofile.VoiceFinding
		if err := json.Unmarshal([]byte(raw), &fs); err != nil {
			return nil, fmt.Errorf("decode voice vocabulary findings for %q: %w", blockKey(b), err)
		}
		return fs, nil
	}
	return nil, nil
}

func voiceFindingToVerify(file, block string, f coreprofile.VoiceFinding) verifyFinding {
	sev := "warning"
	switch f.Severity {
	case coreprofile.SeverityMajor, coreprofile.SeverityCritical:
		sev = "error"
	}
	return verifyFinding{
		Gate:       gateVoice,
		File:       file,
		Block:      block,
		Severity:   sev,
		Message:    f.Message,
		Suggestion: f.Suggestion,
	}
}

// voiceSourceFiles returns the source files the voice gate should score.
func (a *App) voiceSourceFiles(proj *project.KapiProject, root string, args []string) ([]string, error) {
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

// VerifyUnit is one source file paired with one target file for a locale.
// The rule-based and terminology gates read the source file for source text
// and the target file for translated text, then pair blocks by name.
type VerifyUnit struct {
	SourcePath string
	TargetPath string
	Locale     string
	// collection is the parent content-collection name (empty for a bare
	// content entry). Ship-gate rules resolve against (collection, locale).
	Collection string
	// displayPath is the path reported in findings (the target file, relative
	// to the project root when possible).
	DisplayPath string

	// SourceFormat/SourceConfig and TargetFormat/TargetConfig are the reader
	// binding the recipe declares for this unit's two files: the format name
	// (empty = detect by extension) and the merged reader config —
	// `defaults.formats[<format>].config` overlaid by the content item's own
	// `format.config`, item wins per key.
	//
	// Measurement must read a file the way the run that produced it read it.
	// Reader config decides which text a reader even emits — a markdown reader
	// with `translateFrontMatter` emits the title and description as units,
	// without it they are configuration — so a coverage path blind to the
	// config counts a different set of units than the flow produced, and the
	// percentage is a fraction over two denominators.
	SourceFormat string
	SourceConfig map[string]any
	TargetFormat string
	TargetConfig map[string]any
}

// readSource reads the unit's source file under its declared reader binding.
func (a *App) readSource(ctx context.Context, u VerifyUnit) ([]*model.Block, error) {
	name, cfg := a.unitFormat(u.SourceFormat, u.SourceConfig)
	return a.readBlocksAs(ctx, u.SourcePath, name, cfg, a.SourceLocale())
}

// readTarget reads the unit's target file under its declared reader binding.
func (a *App) readTarget(ctx context.Context, u VerifyUnit) ([]*model.Block, error) {
	name, cfg := a.unitFormat(u.TargetFormat, u.TargetConfig)
	return a.readBlocksAs(ctx, u.TargetPath, name, cfg, a.SourceLocale())
}

// unitFormat resolves the reader binding to read one of a unit's files under.
// The app-wide --format override names one format for everything the caller
// passed, so it displaces the recipe's binding entirely — config included,
// since a config belongs to the format it was declared for.
func (a *App) unitFormat(name string, cfg map[string]any) (string, map[string]any) {
	if a.FormatFlag != "" {
		return a.FormatFlag, nil
	}
	return name, cfg
}

// unitFormatBinding resolves the source- and target-side reader bindings for one
// resolved content file.
//
// The target carries the source's binding because the target is written by the
// source's writer: `merge --materialize` resolves the output format from the
// source item, so the file on disk is in that format whatever its path suggests.
// A target whose extension differs is the one shape that contradicts it — a
// source read from `.po` and delivered as a compiled `.mo`, which is write-only
// through the pipeline — and there the target is detected instead, so
// bilingualBlocks still reaches its file-presence fallback rather than handing
// binary to a text reader.
func unitFormatBinding(proj *project.KapiProject, rf project.ResolvedFile, targetPath string) (srcFormat string, srcCfg map[string]any, tgtFormat string, tgtCfg map[string]any) {
	srcFormat = rf.Format
	srcCfg = mergedFormatConfig(proj, srcFormat, rf.Item)
	if !strings.EqualFold(filepath.Ext(rf.Path), filepath.Ext(targetPath)) {
		return srcFormat, srcCfg, "", nil
	}
	return srcFormat, srcCfg, srcFormat, srcCfg
}

// resolveVerifyUnits builds the list of (source, target, locale) units the
// terminology and rule-based gates inspect. With explicit file args, each arg
// is treated as a target file paired with itself as the source (so monolingual
// checks still flag placeholder/empty issues against the file's own content) —
// unless the file matches a project content target template, in which case the
// matching source file is paired. With no args, units come from the project's
// content × target languages.
func (a *App) resolveVerifyUnits(cmd Command, proj *project.KapiProject, root string, args []string, localeFilter string) ([]VerifyUnit, error) {
	if len(args) > 0 {
		return a.unitsFromArgs(proj, root, args, localeFilter)
	}
	return a.UnitsFromProject(proj, root, localeFilter)
}

// UnitsFromProject expands every content item against the project's target
// languages, pairing each source file with its derived target file per locale.
func (a *App) UnitsFromProject(proj *project.KapiProject, root string, localeFilter string) ([]VerifyUnit, error) {
	ctx := project.NewProjectContext(proj, filepath.Join(root, "x.kapi"))
	resolved, err := ctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil, err
	}

	var units []VerifyUnit
	for _, rf := range resolved {
		if rf.Item == nil || rf.Item.Target == "" {
			continue
		}
		locales := rf.Item.ResolvedTargetLanguages(nil, proj.Defaults)
		for _, loc := range locales {
			if localeFilter != "" && string(loc) != localeFilter {
				continue
			}
			targetPath := expandTargetTemplate(rf.Item.Path, rf.Item.Base, rf.Item.Target, rf.Relative, string(loc), root, proj.Defaults.LocaleFormat)
			rel, relErr := filepath.Rel(root, targetPath)
			if relErr != nil {
				rel = targetPath
			}
			// Inside a convergence run the pass's own output is a draft, not a
			// delivery: grade the draft where one exists, the delivered file
			// where none does (host/convergedrafts.go). DisplayPath keeps naming
			// the destination, which is what a reader is being told about.
			targetPath = a.draftedTargetPath(string(loc), targetPath)
			srcFormat, srcCfg, tgtFormat, tgtCfg := unitFormatBinding(proj, rf, targetPath)
			units = append(units, VerifyUnit{
				SourcePath:   rf.Path,
				TargetPath:   targetPath,
				Locale:       string(loc),
				Collection:   rf.Collection,
				DisplayPath:  rel,
				SourceFormat: srcFormat,
				SourceConfig: srcCfg,
				TargetFormat: tgtFormat,
				TargetConfig: tgtCfg,
			})
		}
	}
	return units, nil
}

// SourceUnitsFromProject expands the project's content to one unit per declared
// SOURCE file — no target, no locale. It is what the source-side derivations
// (source readiness, the source gate) measure, because the source exists whether
// or not the project has a target language to carry it into.
//
// UnitsFromProject cannot serve them: it pairs each source with a target per
// locale, so a monolingual project resolves zero units and every source-side
// answer computed over them reported an empty corpus — a project with content
// read as having none.
func (a *App) SourceUnitsFromProject(proj *project.KapiProject, root string) ([]VerifyUnit, error) {
	ctx := project.NewProjectContext(proj, filepath.Join(root, "x.kapi"))
	resolved, err := ctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(resolved))
	units := make([]VerifyUnit, 0, len(resolved))
	for _, rf := range resolved {
		if seen[rf.Path] {
			continue
		}
		seen[rf.Path] = true
		rel, relErr := filepath.Rel(root, rf.Path)
		if relErr != nil {
			rel = rf.Path
		}
		units = append(units, VerifyUnit{
			SourcePath:   rf.Path,
			Collection:   rf.Collection,
			DisplayPath:  rel,
			SourceFormat: rf.Format,
			SourceConfig: mergedFormatConfig(proj, rf.Format, rf.Item),
		})
	}
	return units, nil
}

// unitsFromArgs treats each file argument as a target file. When the file
// matches a project content target template, the source file and locale are
// recovered so the checks and terminology run bilingually; otherwise the file
// is paired with itself and the locale falls back to --locale or the first
// project target language.
func (a *App) unitsFromArgs(proj *project.KapiProject, root string, args []string, localeFilter string) ([]VerifyUnit, error) {
	files, err := resolveFiles(args)
	if err != nil {
		return nil, err
	}
	var units []VerifyUnit
	for _, f := range files {
		abs, _ := filepath.Abs(f)
		var (
			src                  string
			loc                  string
			srcFormat, tgtFormat string
			srcCfg, tgtCfg       map[string]any
		)
		rf, matchedLoc, ok := matchTargetToSource(proj, root, abs)
		if ok {
			src, loc = rf.Path, matchedLoc
			srcFormat, srcCfg, tgtFormat, tgtCfg = unitFormatBinding(proj, rf, abs)
		} else {
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
		units = append(units, VerifyUnit{
			SourcePath:   src,
			TargetPath:   abs,
			Locale:       loc,
			DisplayPath:  rel,
			SourceFormat: srcFormat,
			SourceConfig: srcCfg,
			TargetFormat: tgtFormat,
			TargetConfig: tgtCfg,
		})
	}
	return units, nil
}

// matchTargetToSource finds the content item whose target template (for some
// project target language) expands to the given target file, returning the
// resolved source file and the locale. Returns ok=false when no item matches.
func matchTargetToSource(proj *project.KapiProject, root, targetAbs string) (source project.ResolvedFile, locale string, ok bool) {
	ctxReg := registry.NewFormatRegistry()
	pctx := project.NewProjectContext(proj, filepath.Join(root, "x.kapi"))
	resolved, err := pctx.ResolveContent(ctxReg)
	if err != nil {
		return project.ResolvedFile{}, "", false
	}
	for _, rf := range resolved {
		if rf.Item == nil || rf.Item.Target == "" {
			continue
		}
		for _, loc := range rf.Item.ResolvedTargetLanguages(nil, proj.Defaults) {
			candidate := expandTargetTemplate(rf.Item.Path, rf.Item.Base, rf.Item.Target, rf.Relative, string(loc), root, proj.Defaults.LocaleFormat)
			candAbs, _ := filepath.Abs(candidate)
			if candAbs == targetAbs {
				return rf, string(loc), true
			}
		}
	}
	return project.ResolvedFile{}, "", false
}

// expandTargetTemplate resolves a content item's target template for one source
// file (relative to the project root) and locale, returning an absolute path
// rooted at the project directory. It delegates to project.ResolveTargetPath —
// the SAME resolver the write path (kapi up / run) uses — so the check and
// coverage gates look for target files exactly where up writes them, honoring
// the full token set ({lang}, {path}, {relpath}, {dir}, {name}, {ext}, "*", and
// directory-mirror targets). Resolving only {lang} here (the old behavior) made
// every {path}-templated target look "missing", falsely reporting untranslated.
func expandTargetTemplate(itemPath, base, tmpl, sourceRel, locale, root, localeFormat string) string {
	out := project.ResolveTargetPathIn(itemPath, base, tmpl, sourceRel, locale, localeFormat)
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	return out
}

// --- terminology gate -------------------------------------------------------

// verifyTerminology term-checks each target file against the term rules
// governing it, reusing core/tools.NewTermCheckTool. The rules are resolved per
// target locale AND per point: the terms a profile binds govern that profile's
// content, so a unit whose source sits under a per-item `channel:` override is
// checked against the vocabulary in force there. A locale with no rules
// contributes no findings; a missing target file (untranslated) is flagged by
// the checks gate, so terminology skips it.
func (a *App) verifyTerminology(cmd Command, units []VerifyUnit) (verifyGateResult, error) {
	ctx := CmdContext(cmd)
	gate := verifyGateResult{Gate: gateTerms, Pass: true, Findings: []verifyFinding{}}

	root := ""
	if projectPath, err := ResolveProjectPath(cmd); err == nil && projectPath != "" {
		root = filepath.Dir(projectPath)
	}

	// Cache the rules per (point, locale) — building them opens the terms store.
	ruleCache := map[string][]coreprofile.TermRule{}
	termRulesFor := func(u VerifyUnit) ([]coreprofile.TermRule, error) {
		point := a.unitGovernancePoint(root, u)
		key := point.Collection + "\x00" + point.Path + "\x00" + u.Locale
		if g, ok := ruleCache[key]; ok {
			return g, nil
		}
		g, err := a.ResolveTermRulesFor(cmd, u.Locale, point)
		if err != nil {
			return nil, err
		}
		ruleCache[key] = g
		return g, nil
	}

	for _, u := range units {
		rules, err := termRulesFor(u)
		if err != nil {
			return gate, err
		}
		if len(rules) == 0 {
			// Nothing bound at this point for this locale → nothing to enforce.
			continue
		}
		blocks, missing, err := a.bilingualBlocks(ctx, u)
		if err != nil {
			if errors.Is(err, errTargetUnreadable) {
				continue // unmeasurable target (e.g. a compiled .mo) — can't check
			}
			return gate, err
		}
		if missing {
			// Untranslated target — terminology can't be checked; the checks
			// gate reports the missing file, so skip here to avoid duplicate
			// noise.
			continue
		}
		cfg := &coretools.TermCheckConfig{
			TermRules:    rules,
			TargetLocale: model.LocaleID(u.Locale),
		}
		tc := coretools.NewTermCheckTool(cfg)
		for _, b := range blocks {
			if cerr := RunCheckTool(ctx, tc, b); cerr != nil {
				return gate, fmt.Errorf("terminology gate %s (%s): %w", u.DisplayPath, u.Locale, cerr)
			}
			// A rule's severity decides whether it fails the gate or only
			// reports: the tool has already sorted the violations into the two
			// properties, so a minor rule warns here rather than blocking.
			if b.Properties[coretools.PropTermCheckPassed] == "false" {
				gate.Pass = false
			}
			for _, v := range []struct {
				prop     string
				severity string
			}{
				{coretools.PropTermCheckErrors, "error"},
				{coretools.PropTermCheckWarnings, "warning"},
			} {
				for m := range strings.SplitSeq(b.Properties[v.prop], "; ") {
					if strings.TrimSpace(m) == "" {
						continue
					}
					gate.Findings = append(gate.Findings, verifyFinding{
						Gate:       gateTerms,
						File:       u.DisplayPath,
						Locale:     u.Locale,
						Severity:   v.severity,
						Message:    m,
						Suggestion: "use the term rule's required translation",
					})
				}
			}
		}
	}
	return gate, nil
}

// unitGovernancePoint is the point a verify unit's SOURCE file sits at — the
// place its governance is bound, honouring a content item's own `channel:`. A
// path outside the project root falls back to the unit's collection, and a unit
// with neither to the project's default point.
func (a *App) unitGovernancePoint(root string, u VerifyUnit) project.GovernancePoint {
	if root != "" && u.SourcePath != "" {
		abs := u.SourcePath
		if !filepath.IsAbs(abs) {
			if r, err := filepath.Abs(abs); err == nil {
				abs = r
			}
		}
		if rel, ok := projectRelPath(root, abs); ok {
			return a.GovernancePointFor("", rel)
		}
	}
	return a.GovernancePointFor(u.Collection, "")
}

// --- qa gate ----------------------------------------------------------------

// verifyQA checks placeholder/tag integrity against the source and flags
// untranslated/empty targets for each target file, reusing
// core/tools.NewQACheckTool.
func (a *App) verifyQA(cmd Command, proj *project.KapiProject, root string, units []VerifyUnit) (verifyGateResult, error) {
	ctx := CmdContext(cmd)
	gate := verifyGateResult{Gate: gateQA, Pass: true, Findings: []verifyFinding{}}

	// Whether a target identical to its source is a defect is settled by the
	// project's terms and its committed decisions — the same rule the loop's
	// check exclusions apply, so this gate and the convergence coverage cannot
	// give one unit two answers.
	identical, err := a.newIdenticalTargetRule(ctx, cmd, proj, root)
	if err != nil {
		return gate, err
	}

	for _, u := range units {
		blocks, missing, err := a.bilingualBlocks(ctx, u)
		if err != nil {
			if errors.Is(err, errTargetUnreadable) {
				continue // unmeasurable target (e.g. a compiled .mo) — can't check
			}
			return gate, err
		}
		if missing {
			// Distinguish "not written yet" from "cannot be written": a blocked
			// path (a directory occupying it, say) is not pending translation
			// work — it is an obstruction the user must clear, and saying
			// "translate the source content" would send them the wrong way.
			message := "target file is missing — content is untranslated"
			suggestion := "translate the source content for this locale"
			if berr := blockedTargetPath(u.TargetPath); berr != nil {
				message = berr.Error()
				suggestion = "clear the obstruction at the target path, then re-run"
			}
			gate.Pass = false
			gate.Findings = append(gate.Findings, verifyFinding{
				Gate:       gateQA,
				File:       u.DisplayPath,
				Locale:     u.Locale,
				Severity:   "error",
				Message:    message,
				Suggestion: suggestion,
			})
			continue
		}

		cfg := coretools.NewQACheckConfig(model.LocaleID(u.Locale))
		// Check placeholder integrity so the checks gate flags dropped placeholders
		// even in plain-text formats where the reader does not extract them as
		// inline codes (e.g. {name}, {{var}}, %s, ${x}). The inline-code
		// difference check still runs for formats that do extract codes; the
		// placeholder check is additive.
		cfg.CheckPlaceholders = true
		qa := coretools.NewQACheckTool(cfg)
		for _, b := range blocks {
			if cerr := RunCheckTool(ctx, qa, b); cerr != nil {
				return gate, fmt.Errorf("qa gate %s (%s): %w", u.DisplayPath, u.Locale, cerr)
			}
			for _, f := range check.Findings(tool.NewBlockViewWithContext(ctx, b)) {
				if identical.suppresses(f, u.SourcePath, b, u.Locale) {
					continue
				}
				failing := qaFindingFails(f)
				sev := verifySeverity(f.Severity)
				if failing {
					sev = "error"
				}
				gate.Findings = append(gate.Findings, verifyFinding{
					Gate:       gateQA,
					File:       u.DisplayPath,
					Locale:     u.Locale,
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

// doNotTranslateTerms returns the set of source texts the project's terms say to
// keep unchanged for a locale — entries whose target equals the source (a brand
// name, a product term). It is one half of identicalTargetRule: an identical
// target is correct for such a string rather than untranslated. Returns nil on
// any resolution error, which simply leaves the rule with nothing to suppress.
func (a *App) doNotTranslateTerms(cmd Command, locale string) map[string]bool {
	rules, err := a.ResolveTermRules(cmd, locale)
	if err != nil || len(rules) == 0 {
		return nil
	}
	dnt := make(map[string]bool, len(rules))
	for _, r := range rules {
		// A term whose required rendering is itself must survive verbatim.
		if r.Term == r.Replacement {
			dnt[r.Term] = true
		}
	}
	return dnt
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

// qaFailingCategories are the finding categories verify treats as gate
// failures: integrity problems that break the translation (dropped/extra
// placeholders or tags, missing required codes, untranslated/empty targets).
// Cosmetic issues (whitespace, doubled words, length ratios) are reported as
// warnings but do not fail the gate.
var qaFailingCategories = map[string]bool{
	"empty-target":                  true,
	"pattern-mismatch":              true,
	"placeholder":                   true,
	"missing-code":                  true,
	"extra-code":                    true,
	"code-order":                    true,
	"non-deletable-span-missing":    true,
	"non-cloneable-span-duplicated": true,
	// Both gate surfaces put a target-same-as-source finding to
	// identicalTargetRule before they consult this map: a project that settles
	// the identity — its terms keep the string, or its record approves the unit —
	// has answered the question the heuristic asks.
	"target-same-as-source": true,
}

// qaFindingFails reports whether a finding should fail the verify checks gate.
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
	case "placeholder":
		if f.Severity == check.SeverityMajor {
			return "remove placeholders/tags that are not present in the source"
		}
		return "keep every placeholder/tag from the source in the target"
	case "extra-code", "non-cloneable-span-duplicated":
		return "remove placeholders/tags that are not present in the source"
	case "target-same-as-source":
		return "translate the target — it is identical to the source"
	}
	return ""
}

// --- shared block helpers ---------------------------------------------------

// bilingualBlocks reads the source file (source text) and the target file
// (translated text), overlaying each target block's text onto the matching
// source block as the unit's target locale. Blocks are paired by Name (the
// format's stable key, e.g. the JSON key path), falling back to ID. Source
// blocks with no matching target keep an empty target so the checks flag them
// as untranslated. Returns missing=true when the target file does not exist —
// or when the path is BLOCKED (see below), which is the same thing as far as
// measurement is concerned.
// errTargetUnreadable signals that a unit's target file exists but its format
// cannot be read back to measure it (a write-only compiled catalog like .mo).
// Coverage treats such a target as present (file-presence); quality and review
// paths skip the unit. Test with errors.Is.
var errTargetUnreadable = errors.New("target format is not readable")

// A blocked target path (see host/targetpath.go) is deliberately NOT
// errTargetUnreadable: the file-presence fallback exists for a real, written
// target that merely cannot be parsed back (a compiled catalog), and counting an
// obstruction as a complete translation is what #1449 was.
func (a *App) bilingualBlocks(ctx context.Context, u VerifyUnit) ([]*model.Block, bool, error) {
	// A blocked path holds no translation, whatever it holds. Report it as
	// missing so every measurement path (coverage, plan, checks, review) treats
	// the locale as untranslated. The reason is not lost: it is reported where a
	// person can act on it — the checks gate's finding (qaGate) and the run that
	// refuses to write the path (core/flow.OutputPathError).
	if blockedTargetPath(u.TargetPath) != nil {
		return nil, true, nil
	}
	if _, err := os.Stat(u.TargetPath); err != nil {
		if os.IsNotExist(err) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("stat %s: %w", u.TargetPath, err)
	}

	sourceBlocks, err := a.readSource(ctx, u)
	if err != nil {
		return nil, false, fmt.Errorf("read source %s: %w", u.SourcePath, err)
	}

	// When source == target (monolingual arg fallback), the file's own blocks
	// serve as both source and target.
	var targetBlocks []*model.Block
	if u.TargetPath == u.SourcePath {
		targetBlocks = sourceBlocks
	} else {
		targetBlocks, err = a.readTarget(ctx, u)
		if err != nil {
			// The target file exists (we stat'd it above) but its format cannot
			// be read back — e.g. a compiled .mo catalog, which is write-only
			// through the pipeline. Per-unit measurement is impossible, so signal
			// the caller with a typed error: coverage falls back to file-presence
			// and the quality/review paths skip the unit, rather than the whole
			// command failing on one unmeasurable collection.
			return nil, false, fmt.Errorf("%w: %s: %w", errTargetUnreadable, u.TargetPath, err)
		}
	}

	for _, sb := range sourceBlocks {
		sb.SourceLocale = model.LocaleID(a.SourceLocale())
	}
	OverlayTargets(sourceBlocks, targetBlocks, model.LocaleID(u.Locale))
	return sourceBlocks, false, nil
}

// readBlocks reads a file through its detected format reader and returns the
// translatable blocks, with sourceLang as the source locale. It does not write
// any output — verify only inspects content. The app-wide --format override
// (a.FormatFlag) applies; ReadBlocksForCheck is the per-file-format variant.
func (a *App) readBlocks(ctx context.Context, path, sourceLang string) ([]*model.Block, error) {
	return a.readBlocksAs(ctx, path, a.FormatFlag, nil, sourceLang)
}

// readBlocksAs is readBlocks with an explicit format override (empty =
// detect by extension) and the project format config to read under (nil = the
// reader's defaults). It is the validation-off wrapper around
// readBlocksValidated, so every caller that does not opt into Reader
// Validation-Mode keeps the byte-identical lenient behavior.
func (a *App) readBlocksAs(ctx context.Context, path, fmtName string, cfg map[string]any, sourceLang string) ([]*model.Block, error) {
	// When the project document cache is open, serve unchanged files from it,
	// streaming the parts one at a time and projecting out the translatable blocks
	// — the read/coverage path never reconstructs output, so it never opens the
	// skeleton. The key is namespaced ("rb|") and disjoint from the runner's key.
	if a.docCache != nil {
		configKey := readBlocksConfigKey(fmtName, cfg, sourceLang)
		if doc := a.docCache.OpenDocument(path, configKey); doc != nil {
			blocks, err := streamTranslatableBlocks(ctx, doc)
			_ = doc.Close()
			if err == nil {
				return blocks, nil
			}
			// A corrupt log → fall through and re-parse (re-records the entry).
		}
		return a.recordAndCollectBlocks(ctx, path, fmtName, cfg, configKey, sourceLang)
	}
	blocks, _, err := a.readBlocksValidated(ctx, path, fmtName, cfg, sourceLang, format.ValidationOff)
	return blocks, err
}

// readBlocksConfigKey is the cache config key for the read/coverage path. It is
// namespaced ("rb|") so it never collides with the flow runner's key for the
// same file, and read-path entries carry no skeleton (status writes no file).
//
// The format config is part of the key because it decides which text a reader
// even emits: the same YAML read with and without the project's
// keyPathPatterns yields different blocks, and a key blind to the config would
// replay one run's blocks for the other's question.
func readBlocksConfigKey(formatFlag string, cfg map[string]any, sourceLang string) string {
	return "rb|" + formatFlag + "|" + formatConfigDigest(cfg) + "|" + sourceLang
}

// formatConfigDigest is a stable digest of a format config map — key order in a
// Go map is randomized, so the keys are sorted before hashing and the same
// config always digests identically. An empty config digests to "".
func formatConfigDigest(cfg map[string]any) string {
	if len(cfg) == 0 {
		return ""
	}
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s=%v\x00", k, cfg[k])
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// streamTranslatableBlocks streams a cached document's parts one at a time and
// collects the translatable blocks — never holding the whole document.
func streamTranslatableBlocks(ctx context.Context, doc flow.CachedDocument) ([]*model.Block, error) {
	ch := make(chan *model.Part, 1)
	errc := make(chan error, 1)
	go func() { errc <- doc.Feed(ctx, ch) }()
	var blocks []*model.Block
	for p := range ch {
		if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
			blocks = append(blocks, b)
		}
	}
	return blocks, <-errc
}

// recordAndCollectBlocks is the read-path cache miss: it parses the file once,
// streaming each part into the document-cache recorder (so the next read replays
// it) AND collecting the translatable blocks — holding only the blocks it returns,
// not the whole document. The recorded entry has no skeleton (the reader's
// emitter is not wired): the read path never reconstructs output, and its "rb|"
// key is disjoint from the runner's, so no writer ever reads a skeleton-less entry.
func (a *App) recordAndCollectBlocks(ctx context.Context, path, fmtName string, cfg map[string]any, configKey, sourceLang string) ([]*model.Block, error) {
	if fmtName == "" {
		detected, err := a.FormatReg.Detect(path, registry.DetectOptions{ExtensionOnly: true})
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

	if err := applyFormatConfig(reader, cfg); err != nil {
		return nil, fmt.Errorf("apply format config for %q: %w", filepath.Base(path), err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     a.InputEncoding(),
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("open %q: %w", filepath.Base(path), err)
	}

	rec := a.docCache.RecordDocument(path, configKey, fmtName)
	return streamIntoRecorder(ctx, reader, rec, path)
}

// streamIntoRecorder drains the reader once, appending every part to rec (when
// there is one) and collecting the translatable blocks. rec may be nil — no cache
// configured, or another worker already recording this document — in which case
// it is a plain collecting read.
//
// The recorder's errors are HANDLED, never discarded:
//
//   - A dropped Add error was a *persistent* correctness fault, not a missed
//     optimization. The part never reached the append log, yet Commit still wrote
//     the index row — so every later read of this (path, config) replayed a SHORT
//     document as if it were complete. Coverage's denominator is that block count,
//     so the vanished units simply stopped existing and the locale's percentage
//     climbed, sticky until the source changed or `.kapi/work/cache` was deleted. A
//     document that cannot be fully recorded is therefore not recorded at all:
//     Abort, stop recording, and let the next read re-parse the file. This is the
//     handling core/flow's recordDocument has always had; the asymmetry was the bug.
//
//   - Commit's own failure is safe to degrade on (it aborts internally, so no index
//     row is written and the next read is an honest miss).
//
// Both are still REPORTED. The read itself is unaffected — blocks come straight
// from the reader, not from the recorder — so neither fails the command; but a
// cache that silently never fills presents as nothing except an inexplicably slow
// project, which is its own kind of swallowed error.
func streamIntoRecorder(ctx context.Context, reader format.DataFormatReader, rec flow.DocumentRecorder, path string) ([]*model.Block, error) {
	var blocks []*model.Block
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			if rec != nil {
				rec.Abort()
			}
			return nil, fmt.Errorf("read %q: %w", filepath.Base(path), result.Error)
		}
		if result.Part == nil {
			continue
		}
		if rec != nil {
			if err := rec.Add(result.Part); err != nil {
				rec.Abort()
				rec = nil
				warnf(nil, "Warning: not caching %s: %v\n", DisplayName(path), err)
			}
		}
		if b, ok := result.Part.Resource.(*model.Block); ok && b.Translatable {
			blocks = append(blocks, b)
		}
	}
	if rec != nil {
		if err := rec.Commit(); err != nil {
			warnf(nil, "Warning: not caching %s: %v\n", DisplayName(path), err)
		}
	}
	return blocks, nil
}

// readBlocksValidated reads a file's translatable blocks and, when mode is not
// ValidationOff, the Reader Validation-Mode diagnostics the format reader and
// the encoding scan surfaced. The reader stays lenient — mode only adds
// diagnostics. When mode is on and the reader hard-fails on a structure problem
// it also recorded as a diagnostic, the structured diagnostic supersedes the
// opaque error (the read is reported as the located finding, not as an
// operational failure). With mode ValidationOff this is byte-identical to the
// pre-RVM readBlocks: no diagnostics, errors propagate unchanged. fmtName
// overrides format detection (empty = detect by extension).
func (a *App) readBlocksValidated(ctx context.Context, path, fmtName string, cfg map[string]any, sourceLang string, mode format.ValidationMode) ([]*model.Block, []format.Diagnostic, error) {
	if fmtName == "" {
		detected, err := a.FormatReg.Detect(path, registry.DetectOptions{ExtensionOnly: true})
		if err != nil {
			return nil, nil, fmt.Errorf("detect format for %q: %w", filepath.Base(path), err)
		}
		fmtName = string(detected)
	}

	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return nil, nil, fmt.Errorf("no reader for %q: %w", fmtName, err)
	}
	defer reader.Close()

	if err := applyFormatConfig(reader, cfg); err != nil {
		return nil, nil, fmt.Errorf("apply format config for %q: %w", filepath.Base(path), err)
	}

	// Set the validation mode on the reader's config before Open. Formats that
	// have not opted into RVM don't satisfy ValidationConfig; their ValidationMode
	// stays Off and they emit nothing.
	if vc, ok := reader.Config().(format.ValidationConfig); ok {
		vc.SetValidationMode(mode)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	declaredEnc := a.InputEncoding()

	// Format-agnostic encoding diagnostics (invalid UTF-8, declared-vs-detected
	// charset mismatch) over the same raw bytes; no-op when mode is Off.
	diags := encoding.Diagnose(content, declaredEnc, mode)

	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     declaredEnc,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, nil, fmt.Errorf("open %q: %w", filepath.Base(path), err)
	}

	var blocks []*model.Block
	var readErr error
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			readErr = fmt.Errorf("read %q: %w", filepath.Base(path), result.Error)
			break
		}
		if result.Part == nil || result.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := result.Part.Resource.(*model.Block); ok && b.Translatable {
			blocks = append(blocks, b)
		}
	}

	// Collect the reader's structure diagnostics (after the range loop, before
	// Close — state lives on the reader). Discovery is by assertion.
	if dr, ok := reader.(format.DiagnosticReader); ok {
		diags = append(diags, dr.Diagnostics()...)
	}

	if readErr != nil {
		// In validation mode, a hard read failure the reader also captured as a
		// structure diagnostic is reported as that located finding rather than an
		// operational error. With mode Off no diagnostics exist, so the error
		// propagates exactly as before.
		if mode != format.ValidationOff && len(diags) > 0 {
			return blocks, diags, nil
		}
		return nil, nil, readErr
	}
	return blocks, diags, nil
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
func sortFindings(fs []verifyFinding) {
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

// AddGateFlag registers the project-gate selector. It is registered separately
// because `kapi check` carries the rest of the gate configuration under its own
// flags and needs only this one.
func AddGateFlag(cmd Command) {
	cmd.Flags().StringSlice(gateFlagName, nil,
		"with --ship: run only the named project gates ("+strings.Join(selectableGates, ", ")+"); repeatable")
}

// AddVerifyFlags registers the verify/check gate flags on cmd — shared by
// the cobra factory and embedded surfaces (the Stop hook) so flag defaults
// stay identical everywhere.
func AddVerifyFlags(cmd Command) {
	cmd.Flags().String("source-lang", "", "source language (overrides the project's source_language)")
	AddGateFlag(cmd)
	cmd.Flags().Int("min-score", DefaultVoiceMinScore, "voice compliance score below which the voice gate fails")
	cmd.Flags().String("locale", "", "scope terminology and the rule-based checks to a single target locale (e.g. fr)")
	cmd.Flags().String("termstore", "", "named terms or path to a terms store (defaults to the project terms store)")
	cmd.Flags().Bool("json", false, "output the structured result as JSON")
	cmd.Flags().Bool("ship", false, "also enforce the project's ship gates: fail if any locale's coverage is below its gate (the pre-release bar). Off by default — target drift is non-blocking; see 'kapi status'.")
	cmd.Flags().Bool("no-fail", false, "report only: exit 0 even when a gate fails (verdict is in the output/--json). Use inside an assistant fix-loop; omit for CI gating.")
}
