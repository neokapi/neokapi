package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/terms"
)

// VoiceVocabConfig holds configuration for the voice vocabulary check tool.
type VoiceVocabConfig struct {
	Profile *coreprofile.VoiceProfile `schema:"description=Voice profile containing vocabulary rules"`
}

func (c *VoiceVocabConfig) ToolName() string { return "voice-vocab-check" }
func (c *VoiceVocabConfig) Reset()           {}
func (c *VoiceVocabConfig) Validate() error  { return nil }

// VoiceVocabCheckTool checks text against voice vocabulary rules (preferred/forbidden/competitor terms).
// This is a rule-based check that runs before the LLM-based voice-check.
type VoiceVocabCheckTool struct {
	tool.BaseTool
	profile     *coreprofile.VoiceProfile
	terminology terms.Terminology           // optional — the project's decided vocabulary
	resolver    coreprofile.ProfileResolver // optional: lazy profile resolution
	rc          coreprofile.ResolveContext  // context for resolver
	resolved    bool                        // true after first resolution attempt
	// sourceLocale is the language the terms lookup asks in when a block carries
	// no locale of its own. Most readers stamp the locale on the document layer
	// rather than on every block, so without it a caller with a terms store bound
	// would look its vocabulary up in the empty language and match nothing.
	sourceLocale model.LocaleID
}

// InSourceLocale sets the language the vocabulary lookup asks in for blocks that
// carry no locale, and returns the tool so a caller can chain it onto the
// constructor. A block that does carry one always wins.
func (t *VoiceVocabCheckTool) InSourceLocale(loc model.LocaleID) *VoiceVocabCheckTool {
	t.sourceLocale = loc
	return t
}

// NewVoiceVocabCheckTool creates a new voice vocabulary check tool.
func NewVoiceVocabCheckTool(profile *coreprofile.VoiceProfile, tb terms.Terminology) *VoiceVocabCheckTool {
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
func NewVoiceVocabCheckToolWithResolver(resolver coreprofile.ProfileResolver, rc coreprofile.ResolveContext, tb terms.Terminology) *VoiceVocabCheckTool {
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

	// The profile's prohibited style patterns, which are the profile's own and
	// nothing else declares.
	findings := coreprofile.PatternHitsToFindings(
		coreprofile.MatchPatterns(t.profile, sourceText), sourceText, sourceRuns)

	// Every declared term, from the profile's vocabulary and from the bound
	// terms store, located in one pass. Both are the same kind of statement
	// about the same words, and a gate that asked them separately would be two
	// gates that can disagree.
	lookupIn := v.SourceLocale()
	if lookupIn == "" {
		lookupIn = t.sourceLocale
	}
	occurrences, err := terms.Locate(v.Context(), terms.LocateRequest{
		Text:     sourceText,
		Runs:     sourceRuns,
		RuleSets: coreprofile.VocabularyRuleSets(t.profile),
		Store:    t.terminology,
		Locale:   lookupIn,
	})
	if err != nil {
		return err
	}
	findings = append(findings, findingsFor(violations(occurrences), sourceText, sourceRuns)...)

	if len(findings) > 0 {
		// Add the voice annotation (which carries the findings + score).
		score := coreprofile.CalculateScore(findings)
		profileID := ""
		if t.profile != nil {
			profileID = t.profile.ID
		}
		v.Annotate("voice", &coreprofile.VoiceAnnotation{
			ProfileID: profileID,
			Score:     score.Overall,
			Findings:  findings,
		})
	}

	return nil
}

// violations keeps the occurrences this gate is about, and grades them.
//
// Locating a term and objecting to it are different questions, and only the
// second is this gate's. A terms store holds preferred and approved terms too;
// term-lookup annotates those for context and is right to. Here they are simply
// uses of words the project likes.
//
// A rule the caller declared is a violation by construction — a voice profile's
// forbidden and competitor lists exist to be objected to — so it keeps the kind
// and severity its rule set gave it. A store match is graded by the concept's
// standing: a competitor's name is critical and a forbidden term major, matching
// how the two are weighted in a profile. A retired term is minor, because the
// word was the project's own until a decision replaced it, and `--strict` should
// not turn every legacy spelling in a corpus into a build failure the day a term
// is retired.
func violations(occurrences []terms.Occurrence) []terms.Occurrence {
	out := make([]terms.Occurrence, 0, len(occurrences))
	for _, occ := range occurrences {
		if occ.Source == terms.SourceRule {
			out = append(out, occ)
			continue
		}
		switch {
		case occ.Competitor:
			occ.Kind, occ.Severity = coreprofile.VocabCompetitor, coreprofile.SeverityCritical
		case occ.Status == model.TermForbidden:
			occ.Kind, occ.Severity = coreprofile.VocabForbidden, coreprofile.SeverityMajor
		case occ.Status == model.TermDeprecated:
			occ.Kind, occ.Severity = coreprofile.VocabForbidden, coreprofile.SeverityMinor
		default:
			continue
		}
		out = append(out, occ)
	}
	return out
}

// findingsFor presents located occurrences as voice findings.
//
// The mapping is profile.HitsToFindings, the one every vocabulary surface
// shares — the /check endpoint, the check_vocabulary MCP tool, the desktop
// panel — so the streaming tool cannot drift from them on message wording,
// suggestion phrasing or concept propagation.
//
// On top of it, an occurrence the terms store declared says so. The two
// phrasings are deliberate: "forbidden by the profile" and "forbidden in terms"
// send a writer to different places to argue with the decision, and a single
// wording would hide which one is holding them.
func findingsFor(occurrences []terms.Occurrence, text string, runs []model.Run) []coreprofile.VoiceFinding {
	if len(occurrences) == 0 {
		return nil
	}
	hits := make([]coreprofile.VocabHit, 0, len(occurrences))
	for _, occ := range occurrences {
		hits = append(hits, occ.Hit())
	}
	findings := coreprofile.HitsToFindings(hits, text, runs)
	for i, occ := range occurrences {
		if occ.Source == terms.SourceStore {
			findings[i].Message = storeMessage(occ)
		}
	}
	return findings
}

// storeMessage names the terms store as where the decision lives. A retired
// term reads as the softer complaint it is: the word was the project's own
// until a decision replaced it.
func storeMessage(occ terms.Occurrence) string {
	switch {
	case occ.Competitor:
		return fmt.Sprintf("Competitor term %q found in terms", occ.Term)
	case occ.Status == model.TermDeprecated:
		return fmt.Sprintf("Retired term %q found in terms", occ.Term)
	}
	return fmt.Sprintf("Forbidden term %q found in terms", occ.Term)
}
