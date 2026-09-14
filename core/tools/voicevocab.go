package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/check"
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
	findings := coreprofile.PatternFindings(t.profile, sourceText, sourceRuns)

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

// Canaries returns the known-bad inputs this checker must flag under its
// configuration: the first term of each vocabulary list the profile declares,
// text the first checkable prohibited pattern matches, text the first applicable
// constraint pattern matches, and a term the bound terms store forbids, retires
// or names as a competitor's in the lookup language.
//
// Each is kept only when the profile's own matcher, or the store lookup, flags it
// without this tool, so a canary is known to be bad before the tool sees it. With
// none, uncheckable says the configuration gives the checker nothing to catch.
func (t *VoiceVocabCheckTool) Canaries(ctx context.Context) (canaries []check.Canary, uncheckable string, err error) {
	t.resolveOnce(ctx)
	if p := t.profile; p != nil {
		add := func(name string, texts ...string) bool {
			for _, text := range texts {
				if text != "" && len(coreprofile.Findings(p, text, nil)) > 0 {
					canaries = append(canaries, check.Canary{Name: name, Block: check.CanaryBlock(text)})
					return true
				}
			}
			return false
		}
		for _, set := range coreprofile.VocabularyRuleSets(p) {
			for _, rule := range set.Rules {
				term := strings.TrimSpace(rule.Term)
				if term != "" && add(fmt.Sprintf("%s term %q", set.Kind, term), term, "`"+term+"`") {
					break
				}
			}
		}
		for _, pat := range p.Style.ProhibitedPatterns {
			// A pattern whose not_after excludes the start of a text, or the
			// inside of a code span, is still caught after a word of prose.
			if text, ok := matchingText(pat.Regex); ok && add(fmt.Sprintf("prohibited pattern %q", pat.Regex), text, "`"+text+"`", "Canary "+text) {
				break
			}
		}
		for _, r := range coreprofile.ConstraintResolutions(p) {
			if r.Status != "applicable" || r.Constraint.Kind != coreprofile.ConstraintProhibitedPattern {
				continue
			}
			if text, ok := matchingText(r.Constraint.Regex); ok && add("constraint "+r.Constraint.ID, text) {
				break
			}
		}
	}
	if t.terminology != nil {
		canary, err := t.storeCanary(ctx)
		if err != nil {
			return nil, "", err
		}
		if canary != nil {
			canaries = append(canaries, *canary)
		}
	}
	if len(canaries) == 0 {
		return nil, "no forbidden, competitor or retired term and no prohibited pattern is declared for this content", nil
	}
	return canaries, "", nil
}

// RequiredPatternCanaries is the known-bad document text for a profile's
// required patterns: text the first checkable one does not match, confirmed by
// the profile's document-scope matcher. With no required pattern there is
// nothing to catch, and uncheckable says so.
func RequiredPatternCanaries(p *coreprofile.VoiceProfile) ([]check.Canary, string) {
	if p == nil {
		return nil, "no voice profile is bound"
	}
	for _, pat := range p.Style.RequiredPatterns {
		re, err := regexp.Compile(strings.TrimSpace(pat.Regex))
		if err != nil {
			continue
		}
		if text, ok := check.TextNotMatching(re); ok && len(coreprofile.DocumentFindings(p, text)) > 0 {
			return []check.Canary{{Name: fmt.Sprintf("required pattern %q", pat.Regex), Block: check.CanaryBlock(text)}}, ""
		}
	}
	return nil, "the voice profile declares no checkable required pattern"
}

// storeCanary is the first term the bound store forbids, retires or names as a
// competitor's that the store lookup finds in the language this tool asks in.
func (t *VoiceVocabCheckTool) storeCanary(ctx context.Context) (*check.Canary, error) {
	concepts, err := t.terminology.Concepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the terms store's concepts: %w", err)
	}
	for _, c := range concepts {
		for _, term := range c.Terms {
			if !term.CompetitorTerm && term.Status != model.TermForbidden && term.Status != model.TermDeprecated {
				continue
			}
			occurrences, err := terms.Locate(ctx, terms.LocateRequest{Text: term.Text, Store: t.terminology, Locale: t.sourceLocale})
			if err != nil {
				return nil, err
			}
			if len(violations(occurrences)) > 0 {
				return &check.Canary{Name: fmt.Sprintf("terms store term %q", term.Text), Block: check.CanaryBlock(term.Text)}, nil
			}
		}
	}
	return nil, nil
}

func matchingText(pattern string) (string, bool) {
	re, err := regexp.Compile(strings.TrimSpace(pattern))
	if err != nil {
		return "", false
	}
	return check.TextMatching(re)
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
