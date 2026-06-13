package termbase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// TermLookupConfig holds configuration for the term lookup tool.
type TermLookupConfig struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	Domains      []string // optional domain filter
	MinScore     float64  // fuzzy threshold (default: 0.8)
	// AsOf evaluates term and relation validity at this RFC3339 instant
	// instead of now. Empty = now.
	AsOf string
	// Tags constrains the validity scope (e.g. market: dach). With neither
	// AsOf nor Tags set, no validity filtering is applied.
	Tags map[string]string
}

// ToolName returns the tool name.
func (c *TermLookupConfig) ToolName() string { return "term-lookup" }

// Reset restores default values.
func (c *TermLookupConfig) Reset() {
	c.MinScore = 0.8
	c.Domains = nil
	c.AsOf = ""
	c.Tags = nil
}

// Validate checks configuration validity.
func (c *TermLookupConfig) Validate() error {
	if c.SourceLocale.IsEmpty() {
		return errors.New("term-lookup: SourceLocale is required")
	}
	if _, err := lookupScope(c.AsOf, c.Tags); err != nil {
		return fmt.Errorf("term-lookup: %w", err)
	}
	return nil
}

// TermLookupTool scans translatable blocks for terms from a TermBase and
// attaches TermAnnotation entries to each block. This is a discovery tool:
// it identifies which source terms appear and what their preferred translations
// are, without modifying the target text.
type TermLookupTool struct {
	tool.BaseTool
	tb  TermBase
	cfg TermLookupConfig
}

// NewTermLookupTool creates a new term lookup pipeline tool.
func NewTermLookupTool(tb TermBase, cfg TermLookupConfig) *TermLookupTool {
	if cfg.MinScore <= 0 {
		cfg.MinScore = 0.8
	}

	t := &TermLookupTool{tb: tb, cfg: cfg}
	t.ToolName = "term-lookup"
	t.ToolDescription = "Annotates blocks with matching terms from a termbase"
	t.Annotate = t.annotate
	return t
}

func (t *TermLookupTool) annotate(v tool.BlockView) error {
	if !v.Translatable() {
		return nil
	}

	sourceText := v.SourceText()
	if sourceText == "" {
		return nil
	}

	scope, err := lookupScope(t.cfg.AsOf, t.cfg.Tags)
	if err != nil {
		return fmt.Errorf("term-lookup: %w", err)
	}

	// Find all term occurrences in the source text.
	matches, err := t.tb.LookupAll(v.Context(), sourceText, LookupOptions{
		SourceLocale: t.cfg.SourceLocale,
		Domains:      t.cfg.Domains,
		MinScore:     t.cfg.MinScore,
		Scope:        scope,
	})
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		return nil
	}

	// Attach each matched term as a TermAnnotation.
	for i, match := range matches {
		// Build target term refs for the target locale. A forbidden or
		// deprecated source term redirects through its USE_INSTEAD or
		// REPLACED_BY relation: the refs then name the replacement concept's
		// preferred term instead of the matched concept's own translations.
		var targetRefs []model.TermRef
		if !t.cfg.TargetLocale.IsEmpty() {
			targetRefs = termRefs(match.Concept, t.cfg.TargetLocale)
			if replaceableStatus(match.Term.Status) {
				repl, ok, err := resolveReplacement(v.Context(), t.tb, match.Concept.ID, scope)
				if err != nil {
					return err
				}
				if ok {
					if pt := repl.PreferredTerm(t.cfg.TargetLocale); pt != nil {
						targetRefs = []model.TermRef{{Text: pt.Text, Locale: pt.Locale, Status: pt.Status}}
					}
				}
			}
		}

		annotation := &model.TermAnnotation{
			SourceTerm:  match.Term.Text,
			ConceptID:   match.Concept.ID,
			TargetTerms: targetRefs,
			Status:      match.Term.Status,
			Score:       match.Score,
			MatchType:   match.MatchType,
		}

		v.AddOverlaySpan(model.OverlayTerm, model.Span{
			ID:    fmt.Sprintf("term:%d", i),
			Range: model.RunRangeForBytes(v.SourceRuns(), match.Position.Start, match.Position.End),
			Value: annotation,
		})
	}

	// Set the count as a property for quick access.
	v.SetProperty("term-count", strconv.Itoa(len(matches)))

	return nil
}

// TermEnforceConfig holds configuration for the term enforcement tool.
type TermEnforceConfig struct {
	SourceLocale  model.LocaleID
	TargetLocale  model.LocaleID
	Domains       []string
	CaseSensitive bool
	// CheckStatuses controls which term statuses are enforced.
	// Default: preferred and approved.
	CheckStatuses []model.TermStatus
	// AsOf evaluates term and relation validity at this RFC3339 instant
	// instead of now. Empty = now.
	AsOf string
	// Tags constrains the validity scope (e.g. market: dach). With neither
	// AsOf nor Tags set, no validity filtering is applied.
	Tags map[string]string
}

// ToolName returns the tool name.
func (c *TermEnforceConfig) ToolName() string { return "term-enforce" }

// Reset restores default values.
func (c *TermEnforceConfig) Reset() {
	c.CaseSensitive = false
	c.CheckStatuses = []model.TermStatus{model.TermPreferred, model.TermApproved}
	c.AsOf = ""
	c.Tags = nil
}

// Validate checks configuration validity.
func (c *TermEnforceConfig) Validate() error {
	if c.SourceLocale.IsEmpty() {
		return errors.New("term-enforce: SourceLocale is required")
	}
	if c.TargetLocale.IsEmpty() {
		return errors.New("term-enforce: TargetLocale is required")
	}
	if _, err := lookupScope(c.AsOf, c.Tags); err != nil {
		return fmt.Errorf("term-enforce: %w", err)
	}
	return nil
}

// TermEnforceTool checks that translated blocks use the correct terminology
// from a TermBase. For each source term found, it verifies that a preferred
// or approved target term appears in the translation. Issues are reported
// as properties and TermAnnotations on the block.
type TermEnforceTool struct {
	tool.BaseTool
	tb  TermBase
	cfg TermEnforceConfig
}

// NewTermEnforceTool creates a new term enforcement pipeline tool.
func NewTermEnforceTool(tb TermBase, cfg TermEnforceConfig) *TermEnforceTool {
	if len(cfg.CheckStatuses) == 0 {
		cfg.CheckStatuses = []model.TermStatus{model.TermPreferred, model.TermApproved}
	}

	t := &TermEnforceTool{tb: tb, cfg: cfg}
	t.ToolName = "term-enforce"
	t.ToolDescription = "Verifies correct terminology usage in translations"
	t.Annotate = t.annotate
	return t
}

func (t *TermEnforceTool) annotate(v tool.BlockView) error {
	if !v.Translatable() {
		return nil
	}

	if !v.HasTarget(t.cfg.TargetLocale) {
		return nil
	}

	sourceText := v.SourceText()
	targetText := v.TargetText(t.cfg.TargetLocale)
	if sourceText == "" || targetText == "" {
		return nil
	}

	scope, err := lookupScope(t.cfg.AsOf, t.cfg.Tags)
	if err != nil {
		return fmt.Errorf("term-enforce: %w", err)
	}

	// Find terms in source.
	matches, err := t.tb.LookupAll(v.Context(), sourceText, LookupOptions{
		SourceLocale:  t.cfg.SourceLocale,
		CaseSensitive: t.cfg.CaseSensitive,
		Domains:       t.cfg.Domains,
		StatusFilter:  t.cfg.CheckStatuses,
		Scope:         scope,
	})
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		return nil
	}

	var violations []string
	violationCount := 0

	for i, match := range matches {
		// A forbidden or deprecated source term redirects through its
		// USE_INSTEAD or REPLACED_BY relation: the expected translation is
		// then the replacement concept's preferred term, not the matched
		// concept's own translations.
		expected := match.Concept
		var replacement *Term
		if replaceableStatus(match.Term.Status) {
			repl, ok, err := resolveReplacement(v.Context(), t.tb, match.Concept.ID, scope)
			if err != nil {
				return err
			}
			if ok {
				if pt := repl.PreferredTerm(t.cfg.TargetLocale); pt != nil {
					expected = repl
					replacement = pt
				}
			}
		}

		// Get expected target terms for this concept.
		targetTerms := expected.TargetTerms(t.cfg.TargetLocale)
		if len(targetTerms) == 0 {
			continue // no target terms defined, skip
		}

		// Check if an acceptable target term appears in the translation: the
		// replacement's preferred term when redirected, otherwise any of the
		// concept's own acceptable-status terms.
		found := false
		var targetRefs []model.TermRef
		if replacement != nil {
			found = containsText(targetText, replacement.Text, t.cfg.CaseSensitive)
			targetRefs = []model.TermRef{{Text: replacement.Text, Locale: replacement.Locale, Status: replacement.Status}}
		} else {
			for _, tt := range targetTerms {
				if !isAcceptableStatus(tt.Status, t.cfg.CheckStatuses) {
					continue
				}
				if containsText(targetText, tt.Text, t.cfg.CaseSensitive) {
					found = true
					break
				}
			}
			targetRefs = termRefs(expected, t.cfg.TargetLocale)
		}

		if !found {
			// Build violation message.
			if replacement != nil {
				violations = append(violations, fmt.Sprintf(
					"source term %q (concept %s) is %s; expected replacement %q (concept %s) does not appear in target",
					match.Term.Text, match.Concept.ID, match.Term.Status, replacement.Text, expected.ID))
			} else {
				var expectedTexts []string
				for _, tt := range targetTerms {
					if isAcceptableStatus(tt.Status, t.cfg.CheckStatuses) {
						expectedTexts = append(expectedTexts, tt.Text)
					}
				}
				violations = append(violations, fmt.Sprintf(
					"source term %q (concept %s) found but none of the expected translations [%s] appear in target",
					match.Term.Text, match.Concept.ID, strings.Join(expectedTexts, ", ")))
			}

			// Annotate the block with the violation.
			v.AddOverlaySpan(model.OverlayTerm, model.Span{
				ID:    fmt.Sprintf("term-violation:%d", violationCount),
				Range: model.RunRangeForBytes(v.SourceRuns(), match.Position.Start, match.Position.End),
				Value: &model.TermAnnotation{
					SourceTerm:  match.Term.Text,
					ConceptID:   match.Concept.ID,
					TargetTerms: targetRefs,
					Status:      match.Term.Status,
					Score:       match.Score,
					MatchType:   match.MatchType,
				},
			})
			violationCount++
		}

		// Also add discovery annotations.
		v.AddOverlaySpan(model.OverlayTerm, model.Span{
			ID:    fmt.Sprintf("term:%d", i),
			Range: model.RunRangeForBytes(v.SourceRuns(), match.Position.Start, match.Position.End),
			Value: &model.TermAnnotation{
				SourceTerm:  match.Term.Text,
				ConceptID:   match.Concept.ID,
				TargetTerms: targetRefs,
				Status:      match.Term.Status,
				Score:       match.Score,
				MatchType:   match.MatchType,
			},
		})
	}

	if len(violations) == 0 {
		v.SetProperty("term-enforce-passed", "true")
	} else {
		v.SetProperty("term-enforce-passed", "false")
		v.SetProperty("term-enforce-errors", strings.Join(violations, "; "))
	}
	v.SetProperty("term-enforce-violations", strconv.Itoa(violationCount))

	return nil
}

// lookupScope builds the validity scope for a tool from an optional RFC3339
// AsOf instant and tag constraints. With neither set it returns nil — no
// validity filtering, the pre-scope behavior.
func lookupScope(asOf string, tags map[string]string) (*graph.Scope, error) {
	if asOf == "" && len(tags) == 0 {
		return nil, nil
	}
	at := time.Now()
	if asOf != "" {
		parsed, err := time.Parse(time.RFC3339, asOf)
		if err != nil {
			return nil, fmt.Errorf("invalid AsOf %q: %w", asOf, err)
		}
		at = parsed
	}
	return &graph.Scope{At: at, Tags: tags}, nil
}

// replaceableStatus reports whether a matched source term should redirect its
// translation guidance through a USE_INSTEAD or REPLACED_BY relation: forbidden
// and deprecated terms point away from themselves.
func replaceableStatus(s model.TermStatus) bool {
	return s == model.TermForbidden || s == model.TermDeprecated
}

// resolveReplacement follows the concept's outgoing USE_INSTEAD (guidance,
// preferred) or REPLACED_BY (succession) relation and returns the target
// concept. ok is false when the concept has no such relation in scope or the
// target concept no longer exists. RelationsOf returns relations in ID order,
// so ties resolve deterministically.
func resolveReplacement(ctx context.Context, tb TermBase, conceptID string, scope *graph.Scope) (Concept, bool, error) {
	rels, err := tb.RelationsOf(ctx, conceptID, scope)
	if err != nil {
		return Concept{}, false, err
	}
	var useInstead, replacedBy string
	for _, rel := range rels {
		if rel.SourceID != conceptID {
			continue
		}
		switch rel.RelationType {
		case graph.LabelUseInstead:
			if useInstead == "" {
				useInstead = rel.TargetID
			}
		case graph.LabelReplacedBy:
			if replacedBy == "" {
				replacedBy = rel.TargetID
			}
		}
	}
	targetID := useInstead
	if targetID == "" {
		targetID = replacedBy
	}
	if targetID == "" {
		return Concept{}, false, nil
	}
	concept, ok, err := tb.GetConcept(ctx, targetID)
	if err != nil || !ok {
		return Concept{}, false, err
	}
	return concept, true, nil
}

// termRefs converts a concept's terms for a locale into annotation refs.
func termRefs(c Concept, locale model.LocaleID) []model.TermRef {
	var refs []model.TermRef
	for _, term := range c.TargetTerms(locale) {
		refs = append(refs, model.TermRef{
			Text:   term.Text,
			Locale: term.Locale,
			Status: term.Status,
		})
	}
	return refs
}

func containsText(text, substr string, caseSensitive bool) bool {
	if caseSensitive {
		return strings.Contains(text, substr)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(substr))
}

func isAcceptableStatus(status model.TermStatus, accepted []model.TermStatus) bool {
	if len(accepted) == 0 {
		return true
	}
	return slices.Contains(accepted, status)
}
