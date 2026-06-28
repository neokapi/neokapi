package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
)

// ConvergeLocaleResult is the per-locale outcome of a convergence run.
type ConvergeLocaleResult struct {
	Locale    string         `json:"locale"`
	Shippable bool           `json:"shippable"`        // every gated scope for this locale clears its gate
	Parked    bool           `json:"parked,omitempty"` // still short of its gate after the loop (needs human)
	Pct       map[string]int `json:"pct,omitempty"`    // ladder state → "at least" percent
}

// ConvergeOutput is the structured result of `kapi run` driving the default
// flow over a project's content. One pass by default; looped to the ship gate
// under --until-gate.
type ConvergeOutput struct {
	Flow      string                 `json:"flow"`
	Passes    int                    `json:"passes"`
	Converged bool                   `json:"converged"` // every gated scope is shippable
	Locales   []ConvergeLocaleResult `json:"locales"`
}

// FormatText renders the convergence summary.
func (o ConvergeOutput) FormatText(w io.Writer) error {
	verb := "pass"
	if o.Passes != 1 {
		verb = "passes"
	}
	fmt.Fprintf(w, "Ran flow %q over %d locale(s) in %d %s.\n\n", o.Flow, len(o.Locales), o.Passes, verb)
	for _, lc := range o.Locales {
		state := "pending"
		switch {
		case lc.Parked:
			state = "parked (needs human)"
		case lc.Shippable:
			state = "✓ shippable"
		}
		drafted := lc.Pct["draft"]
		translated := lc.Pct["translated"]
		fmt.Fprintf(w, "  %-10s drafted %d%%  translated %d%%  %s\n", lc.Locale, drafted, translated, state)
	}
	fmt.Fprintln(w)
	if o.Converged {
		fmt.Fprintln(w, "Converged: every gated scope is shippable.")
	} else {
		fmt.Fprintln(w, "Not fully converged — parked locales await human review (never a build failure).")
	}
	return nil
}

// runDefaultFlowConverge executes the project's default flow (defaults.flow)
// over its content across every target language — the no-argument `kapi run`.
// Without untilGate it runs a single pass; with untilGate it loops the pass,
// re-deriving coverage each time, until every gated scope is shippable, a pass
// makes no progress, or maxPasses is reached — parking the locales that remain
// short of their gate. It never fails the build: parked work is reported, not an
// error (target drift is normal toil, not a break).
func (a *App) runDefaultFlowConverge(cmd *cobra.Command, proj *project.KapiProject, projectPath string, untilGate bool, maxPasses int) error {
	flowName := proj.Defaults.Flow
	if flowName == "" {
		return errors.New("no default flow configured: set `defaults.flow` in the project, or name one explicitly (kapi run <flow>)")
	}
	spec := proj.Flow(flowName)
	if spec == nil {
		if builtinComposedFlowNames()[flowName] {
			return fmt.Errorf("defaults.flow %q is a built-in flow; define it under the project's `flows:` map to use it as the convergence default", flowName)
		}
		return fmt.Errorf("default flow %q not found in the project's `flows:`", flowName)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Project context + content sources (the flow reads source, writes per-locale
	// targets via the project's target template).
	pctx := project.NewProjectContext(proj, projectPath)
	if a.SourceLang == "" {
		a.SourceLang = string(pctx.SourceLocale)
	}
	if a.SourceLang == "" {
		a.SourceLang = "en"
	}
	locales := pctx.TargetLocales
	if len(locales) == 0 {
		return errors.New("no target languages configured (defaults.target_languages)")
	}

	resolved, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return fmt.Errorf("resolve content: %w", err)
	}
	var sources []string
	for _, rf := range resolved {
		sources = append(sources, rf.Path)
	}
	if len(sources) == 0 {
		return errors.New("no content to converge (add content patterns to the project)")
	}

	// Standing project context + bindings, so flow steps honor brand-voice /
	// glossary and write to the right per-locale target paths.
	a.projectContext = pctx
	defer func() { a.projectContext = nil }()
	bindings, err := a.resolveProjectBindings(cmd, proj, projectPath)
	if err != nil {
		return err
	}
	a.projectBindings = bindings
	defer func() { a.projectBindings = nil }()

	root := filepath.Dir(projectPath)
	absProjectPath, _ := filepath.Abs(projectPath)
	projectDir := filepath.Dir(absProjectPath)

	savedTarget := a.TargetLang
	defer func() { a.TargetLang = savedTarget }()

	// Convergence materializes the localized target files (not just block-store
	// overlays) so its file-derived coverage sees each pass's output — uniformly
	// across single- and multi-file projects.
	a.convergeWriteFiles = true
	defer func() { a.convergeWriteFiles = false }()

	if maxPasses < 1 {
		maxPasses = 1
	}

	// Share one parse cache across every pass: unchanged source files parse once,
	// not once per pass; only the targets a pass rewrites re-parse.
	return a.withParseCache(root, func() error {
		passes := 0
		for {
			cov, err := a.deriveCoverage(ctx, proj, root)
			if err != nil {
				return err
			}
			pending := localesNeedingPass(cov, locales)
			if len(pending) == 0 {
				// Already converged before this pass (or after the previous one).
				return a.printConverge(cmd, flowName, passes, cov, locales)
			}

			before := producedUnits(cov)
			passes++
			for _, loc := range pending {
				a.TargetLang = string(loc)
				rCtx := flow.ResourceContext{ProjectDir: projectDir, SourceLocale: a.SourceLang, TargetLocale: string(loc)}
				if err := a.runProjectStepsOver(ctx, cmd, flowName, spec, &rCtx, sources); err != nil {
					return fmt.Errorf("converge %s: %w", loc, err)
				}
			}

			cov2, err := a.deriveCoverage(ctx, proj, root)
			if err != nil {
				return err
			}
			if !untilGate {
				return a.printConverge(cmd, flowName, passes, cov2, locales)
			}
			if len(localesNeedingPass(cov2, locales)) == 0 {
				return a.printConverge(cmd, flowName, passes, cov2, locales)
			}
			// Stop looping when capped or when a full pass produced nothing new —
			// the remaining locales park (the flow can't advance them unaided).
			if passes >= maxPasses || producedUnits(cov2) <= before {
				return a.printConverge(cmd, flowName, passes, cov2, locales)
			}
		}
	})
}

// deriveCoverage recomputes per-scope ship coverage from the working tree —
// the same derivation `kapi status` uses, re-read each pass (state is derived,
// never tracked).
func (a *App) deriveCoverage(ctx context.Context, proj *project.KapiProject, root string) ([]LocaleCoverage, error) {
	units, err := a.unitsFromProject(proj, root, "")
	if err != nil {
		return nil, err
	}
	return a.computeShipCoverage(ctx, proj, root, units)
}

// localesNeedingPass returns the locales (in target order) that still have work:
// a gated scope that is not shippable, or — when ungated — content with no
// committed target yet (below the lowest rung).
func localesNeedingPass(cov []LocaleCoverage, locales []model.LocaleID) []model.LocaleID {
	var out []model.LocaleID
	for _, loc := range locales {
		l := string(loc)
		needs := false
		for _, c := range cov {
			if c.Locale != l {
				continue
			}
			if c.Gated && !c.Shippable {
				needs = true
				break
			}
			// Ungated scope: there is work while any unit has no committed target
			// yet (below `draft`, the lowest rung).
			if !c.Gated && c.Pct["draft"] < 100 {
				needs = true
				break
			}
		}
		if needs {
			out = append(out, loc)
		}
	}
	return out
}

// producedUnits is the progress metric: the count of units that have reached at
// least `draft` (any committed target), summed across scopes. A pass that does
// not raise it has stalled.
func producedUnits(cov []LocaleCoverage) int {
	total := 0
	for _, c := range cov {
		total += c.Total * c.Pct["draft"] / 100
	}
	return total
}

// printConverge derives the final per-locale standing and emits the structured
// convergence result. It always returns nil (parked work is not a build error).
func (a *App) printConverge(cmd *cobra.Command, flowName string, passes int, cov []LocaleCoverage, locales []model.LocaleID) error {
	out := ConvergeOutput{Flow: flowName, Passes: passes, Converged: true}
	pendingSet := map[string]bool{}
	for _, loc := range localesNeedingPass(cov, locales) {
		pendingSet[string(loc)] = true
	}
	for _, loc := range locales {
		l := string(loc)
		res := ConvergeLocaleResult{Locale: l, Shippable: true, Pct: map[string]int{}}
		gatedSomewhere := false
		for _, c := range cov {
			if c.Locale != l {
				continue
			}
			for k, v := range c.Pct {
				if v > res.Pct[k] {
					res.Pct[k] = v
				}
			}
			if c.Gated {
				gatedSomewhere = true
				if !c.Shippable {
					res.Shippable = false
				}
			}
		}
		if gatedSomewhere && !res.Shippable {
			res.Parked = pendingSet[l]
			out.Converged = false
		}
		out.Locales = append(out.Locales, res)
	}
	return output.Print(cmd, out)
}

// convergeMaxPassesDefault caps the --until-gate loop. A handful of passes is
// plenty: a deterministic flow converges in one, and a stalled unit parks rather
// than spinning.
const convergeMaxPassesDefault = 5
