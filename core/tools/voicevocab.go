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

// vocabularyLookupLocales is the set of locales a source text's vocabulary is
// looked up in: the text's own locale and, when that names a region, the bare
// language beneath it.
//
// A terms lookup matches the locale exactly, so a project writing en-GB against
// a vocabulary recorded as `en` would be held to nothing at all — a gate that
// reports PASS because it asked the wrong question. An empty locale yields no
// candidate: there is no language to look terms up in, and the empty string is a
// locale like any other in the store.
func vocabularyLookupLocales(loc model.LocaleID) []model.LocaleID {
	if loc == "" {
		return nil
	}
	out := []model.LocaleID{loc}
	if i := strings.IndexAny(string(loc), "-_"); i > 0 {
		out = append(out, loc[:i])
	}
	return out
}

// preferredTerm answers "what should this say instead" for a matched term: the
// preferred term its concept carries in the same language, or the replacement
// the term's own note names when the concept carries no preferred sibling.
//
// The match is consulted first and the store second, because a lookup can return
// either shape: the in-memory store hands back the whole concept, while the
// SQLite one selects a row per term and fills in a concept STUB carrying the id
// and definition but none of its other terms. Reading only the match therefore
// left every finding from a real project without the alternative — the one part
// of a vocabulary finding that says what to do.
//
// The note is the last resort rather than the first, because a concept that
// carries both words is the better record and the one `kapi apply` now writes.
// It still has to be read: a term decided before that, or imported from a
// vocabulary that files each word separately, has the replacement in the note
// and nowhere else, and a finding that names no alternative is the beat of the
// correction loop that lands without its fix.
func (t *VoiceVocabCheckTool) preferredTerm(ctx context.Context, m terms.TermMatch, loc model.LocaleID) (string, error) {
	if pref := m.Concept.PreferredTerm(loc); pref != nil && pref.Text != "" {
		return pref.Text, nil
	}
	if m.Concept.ID == "" || len(m.Concept.Terms) > 0 {
		// Nothing to look up, or the concept was whole and named none.
		return terms.ReplacementFromNote(m.Term.Note), nil
	}
	full, found, err := t.terminology.GetConcept(ctx, m.Concept.ID)
	if err != nil {
		return "", fmt.Errorf("read concept %s: %w", m.Concept.ID, err)
	}
	if !found {
		return terms.ReplacementFromNote(m.Term.Note), nil
	}
	if pref := full.PreferredTerm(loc); pref != nil && pref.Text != "" {
		return pref.Text, nil
	}
	return terms.ReplacementFromNote(m.Term.Note), nil
}

func (t *VoiceVocabCheckTool) annotateBlock(v tool.BlockView) error {
	t.resolveOnce(v.Context())

	sourceText := v.SourceText()
	if strings.TrimSpace(sourceText) == "" {
		return nil
	}

	sourceRuns := v.SourceRuns()

	// The profile's whole deterministic gate (coreprofile.Findings): forbidden and
	// competitor terms, matched whole-word and Unicode-aware (check.FindTerm) so
	// "use" never matches inside "user", plus the prohibited style patterns
	// matched as authored. The same entry point backs the /check endpoint, the
	// check_vocabulary MCP tool and the desktop inspector, so none of these paths
	// enforces half a coreprofile.
	findings := coreprofile.Findings(t.profile, sourceText, sourceRuns)

	// The bound terms store, when there is one: the vocabulary the project
	// DECIDED, enforced alongside the profile's own lists.
	//
	// Every source is consulted, not just the voice vocabulary. A term decision
	// lands in the terminology source (`kapi apply` writes concepts there), so a
	// voice-vocabulary-only filter enforced a source nothing writes to and
	// ignored the one everything writes to — retrieval reported a word as retired
	// while every gate passed it.
	if t.terminology != nil {
		lookupIn := v.SourceLocale()
		if lookupIn == "" {
			lookupIn = t.sourceLocale
		}
		var matches []terms.TermMatch
		// Deduped across the candidate languages: a term recorded in both en-GB
		// and en is one decision about one word, and two findings for it would
		// penalize the block's score twice.
		type hit struct {
			text string
			pos  int
		}
		seen := map[hit]bool{}
		for _, loc := range vocabularyLookupLocales(lookupIn) {
			found, err := t.terminology.LookupAll(v.Context(), sourceText, terms.LookupOptions{SourceLocale: loc})
			if err != nil {
				return err
			}
			for _, m := range found {
				key := hit{text: strings.ToLower(m.Term.Text), pos: m.Position.Start}
				if seen[key] {
					continue
				}
				seen[key] = true
				matches = append(matches, m)
			}
		}
		for _, m := range matches {
			var f coreprofile.VoiceFinding
			switch {
			case m.Term.CompetitorTerm:
				f = coreprofile.VoiceFinding{
					Category:     string(coreprofile.DimensionVocabulary),
					Severity:     coreprofile.SeverityCritical,
					Message:      fmt.Sprintf("Competitor term %q found in terms", m.Term.Text),
					Position:     model.RangeAnchorForBytes(sourceRuns, m.Position.Start, m.Position.End),
					OriginalText: m.Term.Text,
				}
			case m.Term.Status == model.TermForbidden:
				f = coreprofile.VoiceFinding{
					Category:     string(coreprofile.DimensionVocabulary),
					Severity:     coreprofile.SeverityMajor,
					Message:      fmt.Sprintf("Forbidden term %q found in terms", m.Term.Text),
					Position:     model.RangeAnchorForBytes(sourceRuns, m.Position.Start, m.Position.End),
					OriginalText: m.Term.Text,
				}
			case m.Term.Status == model.TermDeprecated:
				// Retired, not banned: the word was the project's own until a
				// decision replaced it, so it reads as the softer finding the
				// retrieval surface already reports it as ("discouraged — say X").
				// Minor keeps `--strict` from turning every legacy spelling in a
				// corpus into a build failure the day a term is retired.
				f = coreprofile.VoiceFinding{
					Category:     string(coreprofile.DimensionVocabulary),
					Severity:     coreprofile.SeverityMinor,
					Message:      fmt.Sprintf("Retired term %q found in terms", m.Term.Text),
					Position:     model.RangeAnchorForBytes(sourceRuns, m.Position.Start, m.Position.End),
					OriginalText: m.Term.Text,
				}
			default:
				continue
			}
			// What to say instead: the concept's preferred term in the same
			// language, surfaced as the structured replacement — symmetric with
			// the profile path, which carries the rule's replacement.
			pref, perr := t.preferredTerm(v.Context(), m, lookupIn)
			if perr != nil {
				return perr
			}
			if pref != "" {
				f.Suggestion = fmt.Sprintf("Use %q instead", pref)
				if f.Metadata == nil {
					f.Metadata = make(map[string]string)
				}
				f.Metadata["replacement"] = pref
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
