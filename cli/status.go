package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
)

// StatusOutput is the structured result of `kapi status`: per-locale coverage
// and ship-gate standing for the project's tracked content.
type StatusOutput struct {
	Project string           `json:"project,omitempty"`
	Source  *SourceCoverage  `json:"source,omitempty"`
	Locales []LocaleCoverage `json:"locales"`
}

// statusLadder is the column order for the human grid.
var statusLadder = gate.TargetLadder()

// FormatText renders the coverage grid, implementing output.TextFormatter.
func (o StatusOutput) FormatText(w io.Writer) error {
	// Source readiness — how far the author's own content has progressed, shown
	// above the translation grid (the source feeds every target).
	if o.Source != nil && o.Source.Total > 0 {
		writeSourceLine(w, *o.Source)
	}
	if len(o.Locales) == 0 {
		fmt.Fprintln(w, "No localized content tracked (no content collections with target locales).")
		return nil
	}
	// Header. The scope is the locale, or "locale/collection" when the project
	// has named collections with their own gates.
	fmt.Fprintf(w, "%-14s %6s", "scope", "units")
	for _, s := range statusLadder {
		fmt.Fprintf(w, " %11s", s)
	}
	fmt.Fprintf(w, "  %s\n", "ship")
	for _, lc := range o.Locales {
		fmt.Fprintf(w, "%-14s %6d", scopeLabel(lc), lc.Total)
		for _, s := range statusLadder {
			fmt.Fprintf(w, " %10d%%", lc.Pct[s])
		}
		fmt.Fprintf(w, "  %s\n", shipCell(lc))
	}
	return nil
}

// sourceLadder is the column order for the source-readiness line.
var sourceLadder = gate.SourceLadder()

// writeSourceLine renders the one-line source-readiness summary: per-rung
// coverage of the author's content (labeled, since its ladder differs from the
// translation grid) plus its source-gate standing.
func writeSourceLine(w io.Writer, sc SourceCoverage) {
	cells := make([]string, 0, len(sourceLadder))
	for _, s := range sourceLadder {
		cells = append(cells, fmt.Sprintf("%s %d%%", s, sc.Pct[s]))
	}
	var standing string
	switch {
	case !sc.Gated:
		standing = ""
	case sc.Shippable:
		standing = "  ✓ ready"
	default:
		parts := make([]string, 0, len(sc.Pending))
		for _, sf := range sc.Pending {
			parts = append(parts, fmt.Sprintf("%s %d%%<%d%%", sf.State, int(sf.Actual), sf.Required))
		}
		standing = "  pending (" + strings.Join(parts, ", ") + ")"
	}
	fmt.Fprintf(w, "source: %d units  %s%s\n\n", sc.Total, strings.Join(cells, "  "), standing)
}

// scopeLabel renders a coverage row's scope: the locale, or "locale/collection"
// when the row is collection-scoped.
func scopeLabel(lc LocaleCoverage) string {
	if lc.Collection != "" {
		return lc.Locale + "/" + lc.Collection
	}
	return lc.Locale
}

// shipCell renders the ship column: shippable, pending (with the binding
// shortfall), or "—" when no gate applies to the locale.
func shipCell(lc LocaleCoverage) string {
	if !lc.Gated {
		return "—"
	}
	if lc.Shippable {
		return "✓ shippable"
	}
	parts := make([]string, 0, len(lc.Pending))
	for _, sf := range lc.Pending {
		parts = append(parts, fmt.Sprintf("%s %d%%<%d%%", sf.State, int(sf.Actual), sf.Required))
	}
	return "pending (" + strings.Join(parts, ", ") + ")"
}

// NewStatusCmd creates `kapi status`: a project dashboard showing per-locale
// translation coverage and ship-gate standing — the informational counterpart
// to `kapi verify` (the gate). State is derived from the project's content ×
// target files, so it is always current with the working tree.
//
// When a plugin provides its own `status` (e.g. kapi-bowrain's sync status), the
// plugin's command takes precedence and this built-in is not registered (see the
// command wiring in cmd/kapi/root.go).
func (a *App) NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		GroupID: "content",
		Short:   "Show per-locale translation coverage and ship-gate standing",
		Long: `Show, per target locale, how much of the project's tracked content is
translated and whether it clears its ship gate — a derived dashboard, like
git status. Coverage is recomputed from the content × target files on every run;
nothing is tracked as state.

This is the informational counterpart to 'kapi verify' (the quality gate). It
never fails: a locale that is behind is reported as pending, not an error —
target-language drift is normal, expected work, not a build break.`,
		Args: cobra.NoArgs,
		RunE: a.runStatus,
	}
	AddProjectFlag(cmd)
	cmd.Flags().String("locale", "", "limit to a single target locale")
	cmd.Flags().String("source-lang", "", "source language (overrides the project's source_language)")
	cmd.Flags().Bool("json", false, "output the structured result as JSON")
	return cmd
}

func (a *App) runStatus(cmd *cobra.Command, _ []string) error {
	a.InitRegistries()
	if cmd.Context() == nil {
		cmd.SetContext(context.Background())
	}

	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)

	sourceLang, _ := cmd.Flags().GetString("source-lang")
	if sourceLang == "" {
		sourceLang = string(proj.Defaults.SourceLanguage)
	}
	if sourceLang == "" {
		sourceLang = "en"
	}
	a.SourceLang = sourceLang

	localeFilter, _ := cmd.Flags().GetString("locale")
	units, err := a.resolveVerifyUnits(cmd, proj, root, nil, localeFilter)
	if err != nil {
		return fmt.Errorf("resolve content: %w", err)
	}

	cov, err := a.computeShipCoverage(cmd.Context(), proj, units)
	if err != nil {
		return fmt.Errorf("compute coverage: %w", err)
	}

	src, err := a.computeSourceReadiness(cmd.Context(), proj, units)
	if err != nil {
		return fmt.Errorf("compute source readiness: %w", err)
	}

	out := StatusOutput{Project: proj.Name, Locales: cov}
	if src.Total > 0 {
		out.Source = &src
	}
	return output.Print(cmd, out)
}
