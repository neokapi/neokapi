package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/spf13/cobra"
)

// CheckFinding is one finding in a `kapi check` run, flattened for output.
type CheckFinding struct {
	File       string `json:"file,omitempty"`
	Category   string `json:"category"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// CheckSummary carries aggregate counts for a check run.
type CheckSummary struct {
	Findings int `json:"findings"`
	Critical int `json:"critical"`
	Major    int `json:"major"`
	Minor    int `json:"minor"`
	Score    int `json:"score"` // 0-100 roll-up across all findings
}

// CheckOutput is the structured result of a `kapi check` run — the unit an AI
// assistant reads, fixes, and re-runs against, the way it loops on tests.
type CheckOutput struct {
	Pass     bool           `json:"pass"`
	Findings []CheckFinding `json:"findings"`
	Summary  CheckSummary   `json:"summary"`
}

// FormatText renders the check result as a human-readable summary.
func (o CheckOutput) FormatText(w io.Writer) error {
	for _, f := range o.Findings {
		loc := ""
		if f.File != "" {
			loc = " " + f.File
		}
		fmt.Fprintf(w, "  %-8s %-16s%s  %s\n", strings.ToUpper(f.Severity), f.Category, loc, f.Message)
		if f.Suggestion != "" {
			fmt.Fprintf(w, "           ↳ %s\n", f.Suggestion)
		}
	}
	if len(o.Findings) == 0 {
		fmt.Fprintln(w, "  No findings.")
	}
	fmt.Fprintln(w)
	verdict := "PASS"
	if !o.Pass {
		verdict = "FAIL"
	}
	fmt.Fprintf(w, "%s — score %d/100 · %d finding(s) (%d critical, %d major, %d minor)\n",
		verdict, o.Summary.Score, o.Summary.Findings, o.Summary.Critical, o.Summary.Major, o.Summary.Minor)
	return nil
}

// NewCheckCmd creates `kapi check`: run content checks over a file (or a
// source/target pair) the way tests run over code, and gate on severity.
//
//	kapi check app.json app.de.json --target-lang de   # bilingual: dnt, placeholders, brand
//	kapi check app.json --pack professional-b2b         # source-side: brand vocabulary
//
// Checks emit one finding model (core/check.Finding); the gate fails on any
// critical by default and exits non-zero so CI and an assistant fix-loop both
// act on the findings.
func (a *App) NewCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "check <file> [target-file]",
		Short:   "Run content checks over a file like tests over code (do-not-translate, placeholders, terminology, brand)",
		GroupID: "quality",
		Args:    cobra.RangeArgs(1, 2),
		Long: `Run content checks over a file — or a source/target pair — and return
structured findings plus a pass/fail, gating on severity.

With two files, the second is the translated target and the bilingual checks run:
do-not-translate (terms that must survive verbatim) and placeholder/tag integrity.
With one file, source-side checks run (brand vocabulary). A bound brand profile
(--profile/--pack/--profile-file) adds vocabulary checks in either mode.

Exit codes: 0 pass, 3 when the gate fails (a critical finding by default), 1 for
operational errors. Pass --no-fail to always exit 0 (report mode) in a fix-loop.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runCheck(cmd, args)
		},
	}
	cmd.Flags().String("target-lang", "", "target locale of the second file (e.g. de)")
	cmd.Flags().StringSlice("dnt", nil, "do-not-translate terms that must survive verbatim into the target")
	cmd.Flags().String("profile", "", "brand profile name from the local store")
	cmd.Flags().String("profile-file", "", "path to a brand profile YAML")
	cmd.Flags().String("pack", "", "built-in brand starter pack")
	cmd.Flags().Int("max-critical", 0, "fail if critical findings exceed this count")
	cmd.Flags().Int("max-major", -1, "fail if major findings exceed this count (-1 = no limit)")
	cmd.Flags().Int("min-score", 0, "fail if the roll-up score is below this (0 = no score gate)")
	cmd.Flags().Bool("json", false, "output the structured result as JSON")
	cmd.Flags().Bool("no-fail", false, "report only: exit 0 even when the gate fails")
	return cmd
}

func (a *App) runCheck(cmd *cobra.Command, args []string) error {
	out, err := a.computeCheck(cmd, args)
	if err != nil {
		return err
	}
	if err := output.Print(cmd, out); err != nil {
		return err
	}
	if !out.Pass {
		if noFail, _ := cmd.Flags().GetBool("no-fail"); noFail {
			return nil
		}
		return ErrQualityGate
	}
	return nil
}

func (a *App) computeCheck(cmd *cobra.Command, args []string) (CheckOutput, error) {
	a.InitRegistries()
	ctx := cmdContext(cmd)

	targetLang, _ := cmd.Flags().GetString("target-lang")
	dntTerms, _ := cmd.Flags().GetStringSlice("dnt")
	profile, err := a.resolveCheckProfile(cmd)
	if err != nil {
		return CheckOutput{}, err
	}

	var findings []check.Finding
	sourcePath := args[0]

	if len(args) == 2 {
		// Bilingual: source + translated target.
		if targetLang == "" {
			targetLang = "und"
		}
		unit := verifyUnit{sourcePath: sourcePath, targetPath: args[1], locale: targetLang, displayPath: args[1]}
		blocks, missing, berr := a.bilingualBlocks(ctx, unit)
		if berr != nil {
			return CheckOutput{}, berr
		}
		if missing {
			return CheckOutput{}, fmt.Errorf("target file %q does not exist", args[1])
		}
		loc := model.LocaleID(targetLang)
		findings = append(findings, a.runBilingualChecks(ctx, blocks, loc, dntTerms)...)
		findings = append(findings, a.runSourceChecks(ctx, blocks, profile)...)
	} else {
		// Monolingual: source-side checks only.
		blocks, rerr := a.readBlocks(ctx, sourcePath, a.SourceLang)
		if rerr != nil {
			return CheckOutput{}, rerr
		}
		findings = append(findings, a.runSourceChecks(ctx, blocks, profile)...)
	}

	return buildCheckOutput(cmd, sourcePath, findings), nil
}

// runBilingualChecks runs the checks that compare a target against its source.
func (a *App) runBilingualChecks(ctx context.Context, blocks []*model.Block, loc model.LocaleID, dntTerms []string) []check.Finding {
	var out []check.Finding

	placeholder := coretools.NewPlaceholderCheckTool(coretools.NewPlaceholderCheckConfig(loc))
	for _, b := range blocks {
		runCheckTool(ctx, placeholder, b)
		out = append(out, findingsFromBlock(b)...)
	}

	if len(dntTerms) > 0 {
		dntCfg := coretools.NewDNTCheckConfig(loc)
		dntCfg.Terms = dntTerms
		dnt := coretools.NewDNTCheckTool(dntCfg)
		for _, b := range blocks {
			runCheckTool(ctx, dnt, b)
			out = append(out, findingsFromBlock(b)...)
		}
	}
	return out
}

// runSourceChecks runs source-side checks (brand vocabulary) when a profile is
// bound.
func (a *App) runSourceChecks(ctx context.Context, blocks []*model.Block, profile *brand.VoiceProfile) []check.Finding {
	if profile == nil {
		return nil
	}
	var out []check.Finding
	vocab := coretools.NewBrandVocabCheckTool(profile, nil)
	for _, b := range blocks {
		runCheckTool(ctx, vocab, b)
		if ann, ok := b.Annotations["brand-voice"].(*brand.BrandVoiceAnnotation); ok {
			out = append(out, ann.Findings...)
		}
	}
	return out
}

// findingsFromBlock reads the unified check annotation off a block.
func findingsFromBlock(b *model.Block) []check.Finding {
	if ann, ok := b.Annotations[check.AnnotationKey].(*check.FindingsAnnotation); ok {
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

func buildCheckOutput(cmd *cobra.Command, file string, findings []check.Finding) CheckOutput {
	out := CheckOutput{Findings: []CheckFinding{}}
	for _, f := range findings {
		out.Findings = append(out.Findings, CheckFinding{
			File:       file,
			Category:   f.Category,
			Severity:   string(f.Severity),
			Message:    f.Message,
			Suggestion: f.Suggestion,
		})
		switch f.Severity {
		case check.SeverityCritical:
			out.Summary.Critical++
		case check.SeverityMajor:
			out.Summary.Major++
		case check.SeverityMinor:
			out.Summary.Minor++
		}
	}
	out.Summary.Findings = len(findings)
	out.Summary.Score = check.CalculateScore(findings).Overall
	sortCheckFindings(out.Findings)

	maxCrit, _ := cmd.Flags().GetInt("max-critical")
	maxMajor, _ := cmd.Flags().GetInt("max-major")
	minScore, _ := cmd.Flags().GetInt("min-score")
	out.Pass = out.Summary.Critical <= maxCrit &&
		(maxMajor < 0 || out.Summary.Major <= maxMajor) &&
		(minScore <= 0 || out.Summary.Score >= minScore)
	return out
}

// severityRank orders findings critical → minor for stable, useful output.
var severityRank = map[string]int{"critical": 0, "major": 1, "minor": 2, "neutral": 3}

func sortCheckFindings(fs []CheckFinding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if severityRank[fs[i].Severity] != severityRank[fs[j].Severity] {
			return severityRank[fs[i].Severity] < severityRank[fs[j].Severity]
		}
		return fs[i].Category < fs[j].Category
	})
}
