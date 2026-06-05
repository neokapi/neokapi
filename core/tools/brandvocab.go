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

	// Forbidden and competitor terms are matched by the shared vocabulary matcher
	// (brand.MatchVocabulary) — whole-word, Unicode-aware (check.FindTerm), so
	// "use" never matches inside "user". The same matcher backs the blast-radius
	// evaluator, so the streaming check and the governance preview never diverge.
	// Here we map each hit's byte range onto run-anchored positions and build the
	// presentation message; preferred terms are surfaced via a forbidden rule's
	// replacement suggestion.
	for _, hit := range brand.MatchVocabulary(t.profile, sourceText) {
		f := brand.BrandVoiceFinding{
			Category:     string(hit.Category),
			Severity:     hit.Severity,
			Position:     model.RunRangeForBytes(sourceRuns, hit.Start, hit.End),
			OriginalText: sourceText[hit.Start:hit.End],
		}
		switch hit.Kind {
		case brand.VocabCompetitor:
			f.Message = fmt.Sprintf("Competitor term %q found", hit.Term)
		default:
			f.Message = fmt.Sprintf("Forbidden term %q found", hit.Term)
			if hit.Note != "" {
				f.Message = fmt.Sprintf("Forbidden term %q found: %s", hit.Term, hit.Note)
			}
		}
		if hit.Replacement != "" {
			f.Suggestion = fmt.Sprintf("Use %q instead", hit.Replacement)
			// Carry the preferred term as a structured replacement so a
			// host (the desktop Checks panel) can offer a one-click fix in
			// addition to the human-readable suggestion above.
			if f.Metadata == nil {
				f.Metadata = make(map[string]string)
			}
			f.Metadata["replacement"] = hit.Replacement
		}
		findings = append(findings, f)
	}

	// If termBase is available, also look up brand vocabulary terms.
	if t.termBase != nil {
		matches, err := t.termBase.LookupAll(v.Context(), sourceText, termbase.LookupOptions{
			SourceFilter: []termbase.TermSource{termbase.TermSourceBrandVocabulary},
		})
		if err != nil {
			return err
		}
		for _, m := range matches {
			if m.Term.CompetitorTerm {
				findings = append(findings, brand.BrandVoiceFinding{
					Category:     string(brand.DimensionVocabulary),
					Severity:     brand.SeverityCritical,
					Message:      fmt.Sprintf("Competitor term %q found in termbase", m.Term.Text),
					Position:     model.RunRangeForBytes(sourceRuns, m.Position.Start, m.Position.End),
					OriginalText: m.Term.Text,
				})
			} else if m.Term.Status == model.TermForbidden {
				findings = append(findings, brand.BrandVoiceFinding{
					Category:     string(brand.DimensionVocabulary),
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
