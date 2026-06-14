package tools

import (
	"context"
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

func (t *BrandVocabCheckTool) resolveOnce(ctx context.Context) {
	if t.resolved || t.resolver == nil {
		return
	}
	t.resolved = true
	profile, err := t.resolver.ResolveProfile(ctx, t.rc)
	if err == nil && profile != nil {
		t.profile = profile
	}
}

func (t *BrandVocabCheckTool) annotateBlock(v tool.BlockView) error {
	t.resolveOnce(v.Context())

	sourceText := v.SourceText()
	if strings.TrimSpace(sourceText) == "" {
		return nil
	}

	sourceRuns := v.SourceRuns()

	// Forbidden and competitor terms are matched by the shared vocabulary matcher
	// (brand.MatchVocabulary) — whole-word, Unicode-aware (check.FindTerm), so
	// "use" never matches inside "user" — and mapped to findings by the shared
	// brand.HitsToFindings (message, structured replacement, concept_id). The same
	// matcher + mapper back the blast-radius evaluator, the /check endpoint, and
	// the check_vocabulary MCP tool, so none of these paths diverge.
	findings := brand.HitsToFindings(brand.MatchVocabulary(t.profile, sourceText), sourceText, sourceRuns)

	// If termBase is available, also look up brand vocabulary terms.
	if t.termBase != nil {
		matches, err := t.termBase.LookupAll(v.Context(), sourceText, termbase.LookupOptions{
			SourceFilter: []termbase.TermSource{termbase.TermSourceBrandVocabulary},
		})
		if err != nil {
			return err
		}
		for _, m := range matches {
			var f brand.BrandVoiceFinding
			switch {
			case m.Term.CompetitorTerm:
				f = brand.BrandVoiceFinding{
					Category:     string(brand.DimensionVocabulary),
					Severity:     brand.SeverityCritical,
					Message:      fmt.Sprintf("Competitor term %q found in termbase", m.Term.Text),
					Position:     model.RunRangeForBytes(sourceRuns, m.Position.Start, m.Position.End),
					OriginalText: m.Term.Text,
				}
			case m.Term.Status == model.TermForbidden:
				f = brand.BrandVoiceFinding{
					Category:     string(brand.DimensionVocabulary),
					Severity:     brand.SeverityMajor,
					Message:      fmt.Sprintf("Forbidden term %q found in termbase", m.Term.Text),
					Position:     model.RunRangeForBytes(sourceRuns, m.Position.Start, m.Position.End),
					OriginalText: m.Term.Text,
				}
			default:
				continue
			}
			// When the matched concept carries a preferred term in the source
			// locale, surface it as the structured replacement — symmetric with the
			// profile path, which carries the rule's replacement.
			if pref := m.Concept.PreferredTerm(v.SourceLocale()); pref != nil && pref.Text != "" {
				f.Suggestion = fmt.Sprintf("Use %q instead", pref.Text)
				if f.Metadata == nil {
					f.Metadata = make(map[string]string)
				}
				f.Metadata["replacement"] = pref.Text
			}
			// Link the finding to its knowledge-graph concept, mirroring the profile
			// path so a termbase-sourced hit pivots to the concept story too.
			if m.Concept.ID != "" {
				if f.Metadata == nil {
					f.Metadata = make(map[string]string)
				}
				f.Metadata["concept_id"] = m.Concept.ID
			}
			findings = append(findings, f)
		}
	}

	if len(findings) > 0 {
		// Add the brand-voice annotation (which carries the findings + score).
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
