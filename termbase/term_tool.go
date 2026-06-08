package termbase

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// TermLookupConfig holds configuration for the term lookup tool.
type TermLookupConfig struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	Domains      []string // optional domain filter
	MinScore     float64  // fuzzy threshold (default: 0.8)
}

// ToolName returns the tool name.
func (c *TermLookupConfig) ToolName() string { return "term-lookup" }

// Reset restores default values.
func (c *TermLookupConfig) Reset() {
	c.MinScore = 0.8
	c.Domains = nil
}

// Validate checks configuration validity.
func (c *TermLookupConfig) Validate() error {
	if c.SourceLocale.IsEmpty() {
		return errors.New("term-lookup: SourceLocale is required")
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

	// Find all term occurrences in the source text.
	matches, err := t.tb.LookupAll(v.Context(), sourceText, LookupOptions{
		SourceLocale: t.cfg.SourceLocale,
		Domains:      t.cfg.Domains,
		MinScore:     t.cfg.MinScore,
	})
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		return nil
	}

	// Attach each matched term as a TermAnnotation.
	for i, match := range matches {
		// Build target term refs for the target locale.
		var targetRefs []model.TermRef
		if !t.cfg.TargetLocale.IsEmpty() {
			for _, term := range match.Concept.TargetTerms(t.cfg.TargetLocale) {
				targetRefs = append(targetRefs, model.TermRef{
					Text:   term.Text,
					Locale: term.Locale,
					Status: term.Status,
				})
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

		v.AddFacetSpan(model.FacetTerm, model.Span{
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
}

// ToolName returns the tool name.
func (c *TermEnforceConfig) ToolName() string { return "term-enforce" }

// Reset restores default values.
func (c *TermEnforceConfig) Reset() {
	c.CaseSensitive = false
	c.CheckStatuses = []model.TermStatus{model.TermPreferred, model.TermApproved}
}

// Validate checks configuration validity.
func (c *TermEnforceConfig) Validate() error {
	if c.SourceLocale.IsEmpty() {
		return errors.New("term-enforce: SourceLocale is required")
	}
	if c.TargetLocale.IsEmpty() {
		return errors.New("term-enforce: TargetLocale is required")
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

	// Find terms in source.
	matches, err := t.tb.LookupAll(v.Context(), sourceText, LookupOptions{
		SourceLocale:  t.cfg.SourceLocale,
		CaseSensitive: t.cfg.CaseSensitive,
		Domains:       t.cfg.Domains,
		StatusFilter:  t.cfg.CheckStatuses,
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
		// Get expected target terms for this concept.
		targetTerms := match.Concept.TargetTerms(t.cfg.TargetLocale)
		if len(targetTerms) == 0 {
			continue // no target terms defined, skip
		}

		// Check if any acceptable target term appears in the translation.
		found := false
		for _, tt := range targetTerms {
			if !isAcceptableStatus(tt.Status, t.cfg.CheckStatuses) {
				continue
			}
			if containsText(targetText, tt.Text, t.cfg.CaseSensitive) {
				found = true
				break
			}
		}

		if !found {
			// Build violation message.
			var expected []string
			for _, tt := range targetTerms {
				if isAcceptableStatus(tt.Status, t.cfg.CheckStatuses) {
					expected = append(expected, tt.Text)
				}
			}
			violations = append(violations, fmt.Sprintf(
				"source term %q (concept %s) found but none of the expected translations [%s] appear in target",
				match.Term.Text, match.Concept.ID, strings.Join(expected, ", ")))

			// Annotate the block with the violation.
			var targetRefs []model.TermRef
			for _, tt := range targetTerms {
				targetRefs = append(targetRefs, model.TermRef{
					Text:   tt.Text,
					Locale: tt.Locale,
					Status: tt.Status,
				})
			}
			v.AddFacetSpan(model.FacetTerm, model.Span{
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
		var targetRefs []model.TermRef
		for _, tt := range targetTerms {
			targetRefs = append(targetRefs, model.TermRef{
				Text:   tt.Text,
				Locale: tt.Locale,
				Status: tt.Status,
			})
		}
		v.AddFacetSpan(model.FacetTerm, model.Span{
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
