package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/termbase"
)

// BrandVocabConfig holds configuration for the brand vocabulary check tool.
type BrandVocabConfig struct {
	Profile *brand.VoiceProfile `schema:"description=Brand voice profile containing vocabulary rules"`
}

func (c *BrandVocabConfig) ToolName() string { return "brand-vocab-check" }
func (c *BrandVocabConfig) Reset()           {}
func (c *BrandVocabConfig) Validate() error  { return nil }

// BrandVocabCheckTool checks text against brand vocabulary rules (preferred/forbidden/competitor terms).
// This is a rule-based check that runs before the LLM-based brand-voice-check.
type BrandVocabCheckTool struct {
	tool.BaseTool
	profile  *brand.VoiceProfile
	termBase termbase.TermBase     // optional — if provided, filters by term_source=brand_vocabulary
	resolver brand.ProfileResolver // optional: lazy profile resolution
	rc       brand.ResolveContext  // context for resolver
	resolved bool                  // true after first resolution attempt
}

// NewBrandVocabCheckTool creates a new brand vocabulary check tool.
func NewBrandVocabCheckTool(profile *brand.VoiceProfile, tb termbase.TermBase) *BrandVocabCheckTool {
	t := &BrandVocabCheckTool{
		profile:  profile,
		termBase: tb,
	}
	t.ToolName = "brand-vocab-check"
	t.ToolDescription = "Checks text against brand vocabulary rules (forbidden, competitor, preferred terms)"
	t.Cfg = &BrandVocabConfig{Profile: profile}
	t.Annotate = t.annotateBlock
	return t
}

// NewBrandVocabCheckToolWithResolver creates a brand vocabulary check tool that
// lazily resolves its profile from the organizational context hierarchy.
func NewBrandVocabCheckToolWithResolver(resolver brand.ProfileResolver, rc brand.ResolveContext, tb termbase.TermBase) *BrandVocabCheckTool {
	t := &BrandVocabCheckTool{
		termBase: tb,
		resolver: resolver,
		rc:       rc,
	}
	t.ToolName = "brand-vocab-check"
	t.ToolDescription = "Checks text against brand vocabulary rules (forbidden, competitor, preferred terms)"
	t.Cfg = &BrandVocabConfig{}
	t.Annotate = t.annotateBlock
	return t
}

func (t *BrandVocabCheckTool) resolveOnce() {
	if t.resolved || t.resolver == nil {
		return
	}
	t.resolved = true
	profile, err := t.resolver.ResolveProfile(context.Background(), t.rc)
	if err == nil && profile != nil {
		t.profile = profile
	}
}

func (t *BrandVocabCheckTool) annotateBlock(v tool.BlockView) error {
	t.resolveOnce()

	sourceText := v.SourceText()
	if strings.TrimSpace(sourceText) == "" {
		return nil
	}

	sourceRuns := v.SourceRuns()
	var findings []brand.BrandVoiceFinding
	lowerText := strings.ToLower(sourceText)

	// Check forbidden terms — major severity.
	if t.profile != nil {
		for _, rule := range t.profile.Vocabulary.ForbiddenTerms {
			lowerTerm := strings.ToLower(rule.Term)
			idx := 0
			for {
				pos := strings.Index(lowerText[idx:], lowerTerm)
				if pos < 0 {
					break
				}
				absPos := idx + pos
				f := brand.BrandVoiceFinding{
					Dimension:    brand.DimensionVocabulary,
					Severity:     brand.SeverityMajor,
					Message:      fmt.Sprintf("Forbidden term %q found", rule.Term),
					Position:     model.RunRangeForBytes(sourceRuns, absPos, absPos+len(rule.Term)),
					OriginalText: sourceText[absPos : absPos+len(rule.Term)],
				}
				if rule.Replacement != "" {
					f.Suggestion = fmt.Sprintf("Use %q instead", rule.Replacement)
				}
				if rule.Note != "" {
					f.Message = fmt.Sprintf("Forbidden term %q found: %s", rule.Term, rule.Note)
				}
				findings = append(findings, f)
				idx = absPos + len(lowerTerm)
			}
		}

		// Check competitor terms — critical severity.
		for _, rule := range t.profile.Vocabulary.CompetitorTerms {
			lowerTerm := strings.ToLower(rule.Term)
			idx := 0
			for {
				pos := strings.Index(lowerText[idx:], lowerTerm)
				if pos < 0 {
					break
				}
				absPos := idx + pos
				f := brand.BrandVoiceFinding{
					Dimension:    brand.DimensionVocabulary,
					Severity:     brand.SeverityCritical,
					Message:      fmt.Sprintf("Competitor term %q found", rule.Term),
					Position:     model.RunRangeForBytes(sourceRuns, absPos, absPos+len(rule.Term)),
					OriginalText: sourceText[absPos : absPos+len(rule.Term)],
				}
				if rule.Replacement != "" {
					f.Suggestion = fmt.Sprintf("Use %q instead", rule.Replacement)
				}
				findings = append(findings, f)
				idx = absPos + len(lowerTerm)
			}
		}

		// Check preferred terms — suggest when a forbidden term has a preferred replacement.
		// This is handled above in the forbidden check via rule.Replacement.
	}

	// If termBase is available, also look up brand vocabulary terms.
	if t.termBase != nil {
		matches := t.termBase.LookupAll(sourceText, termbase.LookupOptions{
			SourceFilter: []termbase.TermSource{termbase.TermSourceBrandVocabulary},
		})
		for _, m := range matches {
			if m.Term.CompetitorTerm {
				findings = append(findings, brand.BrandVoiceFinding{
					Dimension:    brand.DimensionVocabulary,
					Severity:     brand.SeverityCritical,
					Message:      fmt.Sprintf("Competitor term %q found in termbase", m.Term.Text),
					Position:     model.RunRangeForBytes(sourceRuns, m.Position.Start, m.Position.End),
					OriginalText: m.Term.Text,
				})
			} else if m.Term.Status == model.TermForbidden {
				findings = append(findings, brand.BrandVoiceFinding{
					Dimension:    brand.DimensionVocabulary,
					Severity:     brand.SeverityMajor,
					Message:      fmt.Sprintf("Forbidden term %q found in termbase", m.Term.Text),
					Position:     model.RunRangeForBytes(sourceRuns, m.Position.Start, m.Position.End),
					OriginalText: m.Term.Text,
				})
			}
		}
	}

	if len(findings) > 0 {
		findingsJSON, _ := json.Marshal(findings)
		v.SetProperty("brand-vocab-findings", string(findingsJSON))

		// Calculate score and add annotation.
		score := brand.CalculateScore(findings)
		profileID := ""
		if t.profile != nil {
			profileID = t.profile.ID
		}
		v.Annotate("brand-voice", &brand.BrandVoiceAnnotation{
			ProfileID: profileID,
			Score:     score.Overall,
			Findings:  findings,
		})
	}

	return nil
}
