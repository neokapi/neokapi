package profile

import (
	"slices"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// channelVocabularyProfile forbids `utilize` and prefers `sign in` at the base,
// adds its own preferred wording for Norwegian, and declares two channels:
// `child`, which tightens the vocabulary, and `adult`, which states none.
func channelVocabularyProfile() *VoiceProfile {
	return &VoiceProfile{
		Name: "Service",
		Vocabulary: VocabularyRules{
			ForbiddenTerms: []TermRule{{
				Term: "utilize", Replacement: "use", Severity: "critical", Forms: []string{"utilizes"},
			}},
			PreferredTerms: []TermRule{{Term: "sign in"}},
		},
		Locales: map[model.LocaleID]LocaleOverride{
			"nb": {VocabularyOverrides: []TermRule{{Term: "e-post", Replacement: "e-post"}}},
		},
		Channels: map[string]ChannelOverride{
			"child": {Vocabulary: &VocabularyRules{
				ForbiddenTerms:  []TermRule{{Term: "authenticate", Replacement: "sign in", Severity: "major"}},
				CompetitorTerms: []TermRule{{Term: "Globex"}},
				PreferredTerms:  []TermRule{{Term: "grown-up"}},
				Abbreviations:   map[string]string{"FAQ": "questions"},
			}},
			"adult": {Tone: &ToneProfile{Formality: "neutral"}},
		},
	}
}

func termsOf(rules []TermRule) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Term)
	}
	return out
}

// severitiesOf lists the severity of every hit on term, so a test can see both
// rules when two layers name the same word.
func severitiesOf(p *VoiceProfile, text, term string) []Severity {
	var out []Severity
	for _, h := range MatchVocabulary(p, text) {
		if h.Term == term {
			out = append(out, h.Severity)
		}
	}
	return out
}

func TestResolveProfile_ChannelVocabularyAppliesOnlyInItsChannel(t *testing.T) {
	p := channelVocabularyProfile()
	const text = "Authenticate before you call Globex."

	child := ResolveProfile(p, "", "child", "")
	require.NotNil(t, child)
	assert.Equal(t, []Severity{SeverityMajor}, severitiesOf(child, text, "authenticate"),
		"a channel's forbidden term is a finding in that channel")
	assert.Equal(t, []Severity{SeverityCritical}, severitiesOf(child, text, "Globex"),
		"a channel's competitor term is a finding in that channel")
	assert.Equal(t, []string{"sign in", "grown-up"}, termsOf(child.Vocabulary.PreferredTerms))
	assert.Equal(t, "questions", child.Vocabulary.Abbreviations["FAQ"])
	assert.Equal(t, []Severity{SeverityCritical}, severitiesOf(child, "Please utilize it.", "utilize"),
		"the profile's own rule still applies inside the channel")

	for _, channel := range []string{"", "adult", "unknown"} {
		resolved := ResolveProfile(p, "", channel, "")
		assert.Empty(t, MatchVocabulary(resolved, text), "channel %q states no such rule", channel)
	}
}

func TestResolveProfile_ChannelPreferredCannotReAllowForbiddenTerm(t *testing.T) {
	p := channelVocabularyProfile()
	p.Channels["child"] = ChannelOverride{Vocabulary: &VocabularyRules{
		CompetitorTerms: []TermRule{{Term: "Globex"}},
		PreferredTerms: []TermRule{
			{Term: "utilize", Note: "fine for children"},
			{Term: "Utilizes"}, // a form the profile's rule declares, in another casing
			{Term: "Globex"},   // the channel's own competitor term
		},
	}}

	resolved := ResolveProfile(p, "", "child", "")
	require.NotNil(t, resolved)
	assert.Equal(t, []string{"sign in"}, termsOf(resolved.Vocabulary.PreferredTerms),
		"a channel's preferred term must never re-allow a forbidden or competitor term")
	assert.Equal(t, []Severity{SeverityCritical}, severitiesOf(resolved, "utilize it", "utilize"),
		"the term stays forbidden in the channel")
}

func TestResolveProfile_ChannelCannotLoosenEarlierLayers(t *testing.T) {
	p := channelVocabularyProfile()
	p.Channels["child"] = ChannelOverride{Vocabulary: &VocabularyRules{
		ForbiddenTerms: []TermRule{{Term: "utilize", Severity: "minor"}},
		PreferredTerms: []TermRule{
			{Term: "Sign in", Replacement: "log in"}, // rewords the profile's preferred term
			{Term: "e-post", Replacement: "epost"},   // rewords the locale's preferred term
		},
		Abbreviations: map[string]string{"FAQ": "questions"},
	}}
	p.Vocabulary.Abbreviations = map[string]string{"FAQ": "frequently asked questions"}

	resolved := ResolveProfile(p, "nb", "child", "")
	require.NotNil(t, resolved)
	assert.Contains(t, severitiesOf(resolved, "Please utilize it.", "utilize"), SeverityCritical,
		"a channel restating a forbidden term at a lower severity leaves the profile's rule firing")
	assert.Equal(t,
		[]TermRule{{Term: "sign in"}, {Term: "e-post", Replacement: "e-post"}},
		resolved.Vocabulary.PreferredTerms,
		"the profile's and the locale's wording survive the channel, which cannot reword either")
	assert.Equal(t, "frequently asked questions", resolved.Vocabulary.Abbreviations["FAQ"])
}

func TestResolveProfile_CaseSensitiveRuleLeavesOtherCasingPreferable(t *testing.T) {
	p := &VoiceProfile{
		Name: "ripgrep",
		Vocabulary: VocabularyRules{ForbiddenTerms: []TermRule{{
			Term: "Ripgrep", Replacement: "ripgrep", CaseSensitive: true,
		}}},
		Channels: map[string]ChannelOverride{"docs": {Vocabulary: &VocabularyRules{
			PreferredTerms: []TermRule{{Term: "ripgrep"}, {Term: "Ripgrep"}},
		}}},
	}
	resolved := ResolveProfile(p, "", "docs", "")
	assert.Equal(t, []string{"ripgrep"}, termsOf(resolved.Vocabulary.PreferredTerms),
		"a case-sensitive rule forbids only its own casing")
}

func TestResolveProfile_PersonaIsBoundedByTheChannel(t *testing.T) {
	p := channelVocabularyProfile()
	p.Personas = map[string]PersonaOverride{
		"sam": {Preferred: []TermRule{{Term: "authenticate"}, {Term: "let's"}}},
	}

	child := ResolveProfile(p, "", "child", "sam")
	assert.Equal(t, []string{"sign in", "grown-up", "let's"}, termsOf(child.Vocabulary.PreferredTerms),
		"a persona applies after the channel, so the channel's forbidden term bounds it")

	adult := ResolveProfile(p, "", "adult", "sam")
	assert.Equal(t, []string{"sign in", "authenticate", "let's"}, termsOf(adult.Vocabulary.PreferredTerms),
		"where no channel forbids the term, the persona's preference stands")
}

func TestResolveProfile_OverridesNeverWriteIntoTheSource(t *testing.T) {
	p := channelVocabularyProfile()
	// Spare capacity is what lets a plain append write through a shallow copy.
	p.Vocabulary.ForbiddenTerms = slices.Grow(p.Vocabulary.ForbiddenTerms, 8)
	p.Vocabulary.PreferredTerms = slices.Grow(p.Vocabulary.PreferredTerms, 8)
	p.Locales["sv"] = LocaleOverride{VocabularyOverrides: []TermRule{{Term: "mejl"}}}
	p.Channels["teen"] = ChannelOverride{Vocabulary: &VocabularyRules{
		ForbiddenTerms: []TermRule{{Term: "lit"}},
		PreferredTerms: []TermRule{{Term: "hey"}},
	}}

	nb := ResolveProfile(p, "nb", "", "")
	child := ResolveProfile(p, "", "child", "")
	nbVocabulary := cloneVocabulary(nb.Vocabulary)
	childVocabulary := cloneVocabulary(child.Vocabulary)

	_ = ResolveProfile(p, "sv", "teen", "")

	assert.Equal(t, nbVocabulary, nb.Vocabulary, "a later resolution must not rewrite an earlier locale's")
	assert.Equal(t, childVocabulary, child.Vocabulary, "a later resolution must not rewrite an earlier channel's")
	assert.Equal(t, []string{"utilize"}, termsOf(p.Vocabulary.ForbiddenTerms))
	assert.Equal(t, []string{"sign in"}, termsOf(p.Vocabulary.PreferredTerms))
}

func TestClone_CopiesChannelVocabulary(t *testing.T) {
	p := channelVocabularyProfile()
	c := p.Clone()
	c.Channels["child"].Vocabulary.ForbiddenTerms[0].Term = "changed"
	c.Channels["child"].Vocabulary.Abbreviations["FAQ"] = "changed"

	assert.Equal(t, "authenticate", p.Channels["child"].Vocabulary.ForbiddenTerms[0].Term)
	assert.Equal(t, "questions", p.Channels["child"].Vocabulary.Abbreviations["FAQ"])
}

const channelVocabularyYAML = `name: Service
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
channels:
  child:
    vocabulary:
      forbidden_terms:
        - term: authenticate
          replacement: sign in
          severity: major
`

func TestChannelVocabularyDecodesInBothLoaders(t *testing.T) {
	lenient, err := LoadProfileYAML(strings.NewReader(channelVocabularyYAML))
	require.NoError(t, err)
	strict, err := DecodeProfileStrict(strings.NewReader(channelVocabularyYAML))
	require.NoError(t, err, "the strict decoder must accept a channel's vocabulary")

	for name, p := range map[string]*VoiceProfile{"lenient": lenient, "strict": strict} {
		v := p.Channels["child"].Vocabulary
		require.NotNil(t, v, "the %s loader must keep the channel's vocabulary", name)
		assert.Equal(t, []TermRule{{Term: "authenticate", Replacement: "sign in", Severity: "major"}}, v.ForbiddenTerms)
	}
	assert.Empty(t, ValidateProfile(strict))
}

func TestValidateProfile_OverrideVocabulary(t *testing.T) {
	p := channelVocabularyProfile()
	p.Channels["child"] = ChannelOverride{Vocabulary: &VocabularyRules{
		ForbiddenTerms: []TermRule{{Term: " "}},
		PreferredTerms: []TermRule{{Term: "grown-up"}, {Term: "Utilize"}},
	}}
	p.Personas = map[string]PersonaOverride{"sam": {
		Preferred: []TermRule{{Term: "sign in", Replacement: "log in"}},
		Avoided:   []TermRule{{Term: "", Severity: "nope"}},
	}}

	byField := map[string]ProfileProblem{}
	for _, pr := range ValidateProfile(p) {
		byField[pr.Field] = pr
	}

	empty := byField["channels.child.vocabulary.forbidden_terms[0].term"]
	assert.Equal(t, "term is empty", empty.Message)
	assert.False(t, empty.Warning)

	dropped, ok := byField["channels.child.vocabulary.preferred_terms[1]"]
	require.True(t, ok, "a preferred term resolution drops must be reported")
	assert.True(t, dropped.Warning, "the profile still resolves, so this warns")
	assert.Contains(t, dropped.Message, `channel "child" prefers "Utilize"`)
	assert.NotContains(t, byField, "channels.child.vocabulary.preferred_terms[0]")

	assert.True(t, byField["personas.sam.preferred_terms[0]"].Warning)
	assert.Contains(t, byField, "personas.sam.avoided_terms[0].term")
	assert.Contains(t, byField, "personas.sam.avoided_terms[0].severity")
}
