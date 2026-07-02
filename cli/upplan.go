package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/spf13/cobra"
)

// UpPlanScope is the planned work for one (collection, locale) scope: how many
// units have no target yet, how many of those an exact TM hit would cover, and
// what remains for AI translation with a rough token estimate.
type UpPlanScope struct {
	Locale     string `json:"locale,omitempty"`
	Collection string `json:"collection,omitempty"`
	// MissingTarget is the count of translatable units with no committed
	// target for the locale.
	MissingTarget int `json:"missingTarget"`
	// TMExact is the count of missing units covered by an exact-hash TM hit
	// (the cheap leverage estimate — fuzzy leverage is not counted).
	TMExact int `json:"tmExact"`
	// AIRemaining is the count of missing units left for AI translation
	// after TM leverage.
	AIRemaining int `json:"aiRemaining"`
	// TokenEstimate approximates the input tokens for the remaining AI work:
	// source characters / 4 (a common chars-per-token heuristic — the
	// providers expose no tokenizer here, so this is an estimate, not a
	// quote).
	TokenEstimate int `json:"tokenEstimate"`
}

// UpPlanOutput is the structured result of `kapi up --plan`: the dry-run work
// plan per (collection, locale), with totals. No provider calls are made and
// nothing is written.
type UpPlanOutput struct {
	Flow   string        `json:"flow,omitempty"`
	Scopes []UpPlanScope `json:"scopes"`
	Totals UpPlanScope   `json:"totals"`
	// Note documents the estimation method for agents reading the JSON.
	Note string `json:"note"`
}

// upPlanNote is the estimation-method disclosure carried in the output.
const upPlanNote = "TM leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining units (no tokenizer, no provider calls)."

// FormatText renders the plan as a table.
func (o UpPlanOutput) FormatText(w io.Writer) error {
	if len(o.Scopes) == 0 {
		fmt.Fprintln(w, "Nothing to do: every unit has a committed target.")
		return nil
	}
	fmt.Fprintf(w, "Plan for flow %q (dry run — nothing written, no provider calls):\n\n", o.Flow)
	fmt.Fprintf(w, "  %-24s %8s %9s %8s %9s\n", "scope", "missing", "TM exact", "AI work", "~tokens")
	for _, s := range o.Scopes {
		scope := s.Locale
		if s.Collection != "" {
			scope = s.Locale + "/" + s.Collection
		}
		fmt.Fprintf(w, "  %-24s %8d %9d %8d %9d\n", scope, s.MissingTarget, s.TMExact, s.AIRemaining, s.TokenEstimate)
	}
	fmt.Fprintf(w, "  %-24s %8d %9d %8d %9d\n", "total", o.Totals.MissingTarget, o.Totals.TMExact, o.Totals.AIRemaining, o.Totals.TokenEstimate)
	fmt.Fprintf(w, "\n%s\n", o.Note)
	return nil
}

// runUpPlan is `kapi up --plan`: a read-only dry run of the convergence work.
// For every (collection, locale) scope it counts the units missing a target,
// estimates TM leverage by exact-hash lookup against the project TM, and
// prices the remainder as AI work with a chars/4 token estimate. It makes no
// provider calls and writes nothing — not even the block store.
func (a *App) runUpPlan(cmd *cobra.Command, proj *project.KapiProject, projectPath string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	root := filepath.Dir(projectPath)

	// Source language: an explicit --source-lang wins; otherwise the project's
	// source_language (the flag's static default would shadow it).
	if !cmd.Flags().Changed("source-lang") && proj.Defaults.SourceLanguage != "" {
		a.SourceLang = string(proj.Defaults.SourceLanguage)
	}
	if a.SourceLang == "" {
		a.SourceLang = "en"
	}

	units, err := a.unitsFromProject(proj, root, "")
	if err != nil {
		return fmt.Errorf("resolve content: %w", err)
	}

	// Project TM, read-only leverage source. Only an existing tm.db is
	// opened — plan must not create files.
	var tm sievepen.TranslationMemory
	if a.TMBackend != nil {
		tm = a.TMBackend
	} else if layout, lerr := project.LayoutFor(projectPath); lerr == nil {
		tmPath := filepath.Join(layout.StateDir, "tm.db")
		if _, statErr := os.Stat(tmPath); statErr == nil {
			if loaded, terr := sievepen.NewSQLiteTM(tmPath); terr == nil {
				defer loaded.Close()
				tm = loaded
			}
		}
	}

	plan, err := a.computeUpPlan(ctx, tm, proj, units)
	if err != nil {
		return err
	}
	return output.Print(cmd, plan)
}

// computeUpPlan derives the per-scope work plan from the verify units.
func (a *App) computeUpPlan(ctx context.Context, tm sievepen.TranslationMemory, proj *project.KapiProject, units []verifyUnit) (UpPlanOutput, error) {
	type scopeKey struct{ locale, collection string }
	scopes := map[scopeKey]*UpPlanScope{}
	scopeFor := func(k scopeKey) *UpPlanScope {
		if s, ok := scopes[k]; ok {
			return s
		}
		s := &UpPlanScope{Locale: k.locale, Collection: k.collection}
		scopes[k] = s
		return s
	}

	source := model.LocaleID(a.SourceLang)
	for _, u := range units {
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if errors.Is(berr, errTargetUnreadable) {
				continue // unmeasurable target — coverage counts it by presence
			}
			return UpPlanOutput{}, berr
		}
		if missing {
			// No target file yet: every translatable source unit is pending.
			srcs, serr := a.readBlocks(ctx, u.sourcePath, a.SourceLang)
			if serr != nil {
				return UpPlanOutput{}, serr
			}
			blocks = srcs
		}
		s := scopeFor(scopeKey{locale: u.locale, collection: u.collection})
		target := model.LocaleID(u.locale)
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			if !missing && strings.TrimSpace(b.TargetText(target)) != "" {
				continue // already produced
			}
			s.MissingTarget++
			if planTMExactHit(ctx, tm, b, source, target) {
				s.TMExact++
				continue
			}
			s.AIRemaining++
			s.TokenEstimate += estimateTokens(b.SourceText())
		}
	}

	out := UpPlanOutput{Flow: proj.Defaults.Flow, Note: upPlanNote}
	for _, s := range scopes {
		if s.MissingTarget == 0 {
			continue
		}
		out.Scopes = append(out.Scopes, *s)
		out.Totals.MissingTarget += s.MissingTarget
		out.Totals.TMExact += s.TMExact
		out.Totals.AIRemaining += s.AIRemaining
		out.Totals.TokenEstimate += s.TokenEstimate
	}
	sort.Slice(out.Scopes, func(i, j int) bool {
		if out.Scopes[i].Locale != out.Scopes[j].Locale {
			return out.Scopes[i].Locale < out.Scopes[j].Locale
		}
		return out.Scopes[i].Collection < out.Scopes[j].Collection
	})
	return out, nil
}

// planTMExactHit reports whether the block's source has an unambiguous exact
// (score 1.0) TM hit with a non-empty target variant — the cheap "TM exact"
// leverage counted by the plan.
func planTMExactHit(ctx context.Context, tm sievepen.TranslationMemory, b *model.Block, source, target model.LocaleID) bool {
	if tm == nil {
		return false
	}
	matches, err := tm.Lookup(ctx, b, source, target, sievepen.LookupOptions{MinScore: 1.0, MaxResults: 1})
	if err != nil || len(matches) == 0 || matches[0].Ambiguous || matches[0].Score < 1.0 {
		return false
	}
	return matches[0].Entry.VariantText(target) != ""
}

// estimateTokens approximates the token count of a text as chars/4 (rounded
// up) — the widely used chars-per-token rule of thumb. The AI providers here
// expose no tokenizer, so a real count is not available without a call.
func estimateTokens(text string) int {
	chars := utf8.RuneCountInString(text)
	if chars == 0 {
		return 0
	}
	return (chars + 3) / 4
}
