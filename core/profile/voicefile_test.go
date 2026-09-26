package profile

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVoiceFileConvertsVocabulary(t *testing.T) {
	f, err := ParseVoiceFile([]byte(`name: Docs
tone:
  formality: neutral
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
    - term: "  "
  competitor_terms:
    - term: Acme
  preferred_terms:
    - term: sign in
      note: the product's wording
    - term: log in
      replacement: sign in
    - term: email
      replacement: Email
  abbreviations:
    e.g.: for example
`))
	require.NoError(t, err)
	require.NotNil(t, f.Profile)
	assert.Equal(t, "Docs", f.Profile.Name)
	assert.Equal(t, "neutral", f.Profile.Tone.Formality)

	assert.Equal(t, []TermRule{
		{Term: "utilize", Replacement: "use"},
		{Term: "Acme", Competitor: true},
		{Replacement: "sign in", Note: "the product's wording"},
		{Term: "log in", Replacement: "sign in", Advisory: true},
		{Replacement: "Email"},
	}, f.Terms, "a blank term is dropped, and a preferred term naming a different replacement becomes an advisory rule")
	assert.Equal(t, VocabularyConversion{Forbidden: 1, Competitor: 1, Preferred: 3}, f.Converted)
	assert.Equal(t, 5, f.Converted.Total())
	assert.Equal(t, "1 forbidden, 1 competitor and 3 preferred terms", f.Converted.String())
	assert.Empty(t, f.Profile.CarriedTerms().Rules, "parsing splits the rules from the profile")
}

func TestVocabularyConversionString(t *testing.T) {
	assert.Empty(t, VocabularyConversion{}.String())
	assert.Equal(t, "1 forbidden term", VocabularyConversion{Forbidden: 1}.String())
	assert.Equal(t, "2 competitor terms", VocabularyConversion{Competitor: 2}.String())
	assert.Equal(t, "1 forbidden and 1 preferred terms", VocabularyConversion{Forbidden: 1, Preferred: 1}.String())
}

func TestParseVoiceFileReadsTerms(t *testing.T) {
	f, err := ParseVoiceFile([]byte(`name: Pack
terms:
  - term: leverage
    replacement: use
    advisory: true
  - term: Globex
    competitor: true
  - replacement: sign in
vocabulary:
  forbidden_terms:
    - term: synergy
`))
	require.NoError(t, err)
	assert.Equal(t, []TermRule{
		{Term: "leverage", Replacement: "use", Advisory: true},
		{Term: "Globex", Competitor: true},
		{Replacement: "sign in"},
		{Term: "synergy"},
	}, f.Terms, "the `terms:` list is read as written, then the converted `vocabulary:` list")
	assert.Equal(t, VocabularyConversion{Forbidden: 1}, f.Converted, "only the `vocabulary:` list counts as converted")
}

func TestLoadProfileYAMLCarriesTheFileTerms(t *testing.T) {
	p, err := LoadProfileYAML(strings.NewReader("name: Docs\nterms:\n  - term: utilize\n    replacement: use\n"))
	require.NoError(t, err)
	carried := p.CarriedTerms()
	assert.Equal(t, CarriedFromVoiceFile, carried.From)
	assert.Equal(t, []TermRule{{Term: "utilize", Replacement: "use"}}, carried.Rules)

	bare, err := LoadProfileYAML(strings.NewReader("name: Docs\n"))
	require.NoError(t, err)
	assert.Empty(t, bare.CarriedTerms(), "a file with no word rules carries none")
}

func TestDecodeProfileStrictCarriesTheFileTerms(t *testing.T) {
	p, err := DecodeProfileStrict(strings.NewReader("name: Docs\nterms:\n  - term: Acme\n    competitor: true\nvocabulary:\n  forbidden_terms:\n    - term: utilize\n"))
	require.NoError(t, err)
	assert.Equal(t, []TermRule{{Term: "Acme", Competitor: true}, {Term: "utilize"}}, p.CarriedTerms().Rules)
}

func TestEncodeVoiceFileRoundTrip(t *testing.T) {
	rules := []TermRule{
		{Term: "utilize", Replacement: "use", Note: "plain words"},
		{Term: "Acme", Competitor: true},
		{Replacement: "sign in"},
	}
	p := (&VoiceProfile{
		Name: "Docs",
		Tone: ToneProfile{Formality: "neutral", Personality: []string{"plain"}},
	}).Carry("pack technical-docs", rules)

	data, err := EncodeVoiceFile(p)
	require.NoError(t, err)
	assert.Contains(t, string(data), "terms:\n", "the rules are written under `terms:`")
	assert.NotContains(t, string(data), "vocabulary:")

	f, err := ParseVoiceFile(data)
	require.NoError(t, err)
	assert.Equal(t, rules, f.Terms)
	assert.Zero(t, f.Converted.Total())
	assert.Equal(t, "Docs", f.Profile.Name)
	assert.Equal(t, p.Tone, f.Profile.Tone)

	_, err = DecodeProfileStrict(bytes.NewReader(data))
	require.NoError(t, err, "the strict decoder reads what the encoder writes")

	plain, err := EncodeVoiceFile(&VoiceProfile{Name: "Tone only"})
	require.NoError(t, err)
	assert.NotContains(t, string(plain), "terms:", "a voice carrying no word rules writes no list")
}

func TestConvertVocabularyLists(t *testing.T) {
	rules, conv := ConvertVocabularyLists(
		[]TermRule{{Term: "workspace"}},
		[]TermRule{{Term: "utilize", Replacement: "use"}},
		[]TermRule{{Term: "Acme"}},
	)
	assert.Equal(t, []TermRule{
		{Term: "utilize", Replacement: "use"},
		{Term: "Acme", Competitor: true},
		{Replacement: "workspace"},
	}, rules)
	assert.Equal(t, VocabularyConversion{Forbidden: 1, Competitor: 1, Preferred: 1}, conv)
}

func TestUnknownKeysNamesAnOverrideWordList(t *testing.T) {
	problems, err := UnknownKeys([]byte("name: Docs\nchannels:\n  x:\n    vocabulary:\n      forbidden_terms:\n        - term: utilize\n"))
	require.NoError(t, err)
	require.Len(t, problems, 1)
	assert.Equal(t, "channels.x.vocabulary", problems[0].Field)
	assert.Equal(t, CodeUnknownKey, problems[0].Code)
	assert.True(t, problems[0].Warning)
	assert.Contains(t, problems[0].Message, "word rules are terms")

	top, err := UnknownKeys([]byte("name: Docs\nvocabulary:\n  forbidden_terms:\n    - term: utilize\nterms:\n  - term: leverage\n"))
	require.NoError(t, err)
	assert.Empty(t, top, "a top-level `vocabulary:` or `terms:` list is read, so neither is unknown")
}
