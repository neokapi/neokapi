package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/terms"
)

// VoiceVocabConfig holds configuration for the voice vocabulary check tool.
type VoiceVocabConfig struct {
	Profile *profile.VoiceProfile `schema:"description=Voice profile containing vocabulary rules"`
}

func (c *VoiceVocabConfig) ToolName() string { return "voice-vocab-check" }
func (c *VoiceVocabConfig) Reset()           {}
func (c *VoiceVocabConfig) Validate() error  { return nil }

// VoiceVocabCheckTool checks text against voice vocabulary rules (preferred/forbidden/competitor terms).
// This is a rule-based check that runs before the LLM-based voice-check.
type VoiceVocabCheckTool struct {
	tool.BaseTool
	profile     *profile.VoiceProfile
	terminology terms.Terminology       // optional — if provided, filters by term_source=brand_vocabulary
	resolver    profile.ProfileResolver // optional: lazy profile resolution
	rc          profile.ResolveContext  // context for resolver
	resolved    bool                    // true after first resolution attempt
}

// NewVoiceVocabCheckTool creates a new voice vocabulary check tool.
func NewVoiceVocabCheckTool(profile *profile.VoiceProfile, tb terms.Terminology) *VoiceVocabCheckTool {
	t := &VoiceVocabCheckTool{
		profile:     profile,
		terminology: tb,
	}
	t.ToolName = "voice-vocab-check"
	t.ToolDescription = "Checks text against voice vocabulary rules (forbidden, competitor, preferred terms)"
	t.Cfg = &VoiceVocabConfig{Profile: profile}
	t.Annotate = t.annotateBlock
	return t
}

// NewVoiceVocabCheckToolWithResolver creates a voice vocabulary check tool that
// lazily resolves its profile from the organizational context hierarchy.
func NewVoiceVocabCheckToolWithResolver(resolver profile.ProfileResolver, rc profile.ResolveContext, tb terms.Terminology) *VoiceVocabCheckTool {
	t := &VoiceVocabCheckTool{
		terminology: tb,
		resolver:    resolver,
		rc:          rc,
	}
	t.ToolName = "voice-vocab-check"
	t.ToolDescription = "Checks text against voice vocabulary rules (forbidden, competitor, preferred terms)"
	t.Cfg = &VoiceVocabConfig{}
	t.Annotate = t.annotateBlock
	return t
}

func (t *VoiceVocabCheckTool) resolveOnce(ctx context.Context) {
	if t.resolved || t.resolver == nil {
		return
	}
	t.resolved = true
	profile, err := t.resolver.ResolveProfile(ctx, t.rc)
	if err == nil && profile != nil {
		t.profile = profile
	}
}

func (t *VoiceVocabCheckTool) annotateBlock(v tool.BlockView) error {
	t.resolveOnce(v.Context())

	sourceText := v.SourceText()
	if strings.TrimSpace(sourceText) == "" {
		return nil
	}

	sourceRuns := v.SourceRuns()

	// Forbidden and competitor terms are matched by the shared vocabulary matcher
	// (profile.MatchVocabulary) — whole-word, Unicode-aware (check.FindTerm), so
	// "use" never matches inside "user" — and mapped to findings by the shared
	// profile.HitsToFindings (message, structured replacement, concept_id). The same
	// matcher + mapper back the blast-radius evaluator, the /check endpoint, and
	// the check_vocabulary MCP tool, so none of these paths diverge.
	findings := profile.HitsToFindings(profile.MatchVocabulary(t.profile, sourceText), sourceText, sourceRuns)

	// If terminology is available, also look up voice vocabulary terms.
	if t.terminology != nil {
		matches, err := t.terminology.LookupAll(v.Context(), sourceText, terms.LookupOptions{
			SourceFilter: []terms.TermSource{terms.TermSourceBrandVocabulary},
		})
		if err != nil {
			return err
		}
		for _, m := range matches {
			var f profile.VoiceFinding
			switch {
			case m.Term.CompetitorTerm:
				f = profile.VoiceFinding{
					Category:     string(profile.DimensionVocabulary),
					Severity:     profile.SeverityCritical,
					Message:      fmt.Sprintf("Competitor term %q found in terms", m.Term.Text),
					Position:     model.RunRangeForBytes(sourceRuns, m.Position.Start, m.Position.End),
					OriginalText: m.Term.Text,
				}
			case m.Term.Status == model.TermForbidden:
				f = profile.VoiceFinding{
					Category:     string(profile.DimensionVocabulary),
					Severity:     profile.SeverityMajor,
					Message:      fmt.Sprintf("Forbidden term %q found in terms", m.Term.Text),
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
			// path so a terms store-sourced hit pivots to the concept story too.
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
		// Add the voice annotation (which carries the findings + score).
		score := profile.CalculateScore(findings)
		profileID := ""
		if t.profile != nil {
			profileID = t.profile.ID
		}
		v.Annotate("voice", &profile.VoiceAnnotation{
			ProfileID: profileID,
			Score:     score.Overall,
			Findings:  findings,
		})
	}

	return nil
}
