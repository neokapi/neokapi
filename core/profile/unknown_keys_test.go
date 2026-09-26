package profile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// misspeltProfile is the known-bad input for UnknownKeys. Every key in it the
// model does not define sits where the lenient load reaches and drops it.
const misspeltProfile = `name: Docs
brand_voice: calm
tone:
  formalty: casual
style:
  prohibited_patterns:
    - regex: '\bsimply\b'
      sevrity: major
channels:
  docs:
    vocab:
      forbidden_terms:
        - term: utilize
    vocabulary:
      forbidden_terms:
        - term: leverage
          replacment: use
examples:
  - before: a
    after: b
    explanaton: c
`

func TestUnknownKeysNamesEachKeyTheLoadDrops(t *testing.T) {
	loaded, err := LoadProfileYAML(strings.NewReader(misspeltProfile))
	require.NoError(t, err, "the lenient load accepts the document, which is how the keys go unnoticed")
	require.Equal(t, "Docs", loaded.Name)

	problems, err := UnknownKeys([]byte(misspeltProfile))
	require.NoError(t, err)
	fields := make([]string, 0, len(problems))
	for _, p := range problems {
		fields = append(fields, p.Field)
		assert.Equal(t, CodeUnknownKey, p.Code)
		assert.True(t, p.Warning, "an unknown key leaves the profile usable")
		if p.Field == "brand_voice" {
			assert.Equal(t, `unknown key "brand_voice" (line 2) is ignored when the profile loads; `+
				"check its spelling and the section it sits under", p.Message)
		}
		if p.Field == "channels.docs.vocabulary" {
			assert.Contains(t, p.Message, "word rules are terms",
				"a channel's word list names where word rules live")
		}
	}
	assert.ElementsMatch(t, []string{
		"brand_voice",
		"tone.formalty",
		"style.prohibited_patterns[0].sevrity",
		"channels.docs.vocab",
		"channels.docs.vocabulary",
		"examples[0].explanaton",
	}, fields)
}

// TestUnknownKeysAgreesWithTheStrictDecoder holds the walk to the decoder that
// defines an unknown key. Over the repository's own profiles and a set of bad
// ones, the walk places every key DecodeProfileStrict refuses, and no other.
func TestUnknownKeysAgreesWithTheStrictDecoder(t *testing.T) {
	docs := map[string][]byte{
		"misspelt":     []byte(misspeltProfile),
		"flow mapping": []byte("name: F\ntone: {formalty: casual, humr: light}\n"),
		"merge key": []byte(`name: M
tone: &tone
  formality: casual
channels:
  docs:
    tone:
      <<: *tone
      humour: light
`),
		"locale and persona": []byte(`name: L
locales:
  nb:
    formalty: formal
personas:
  ana:
    avoid_terms:
      - term: synergy
`),
		"clean": []byte("name: C\ntone:\n  formality: casual\n"),
	}
	packs, err := filepath.Glob(filepath.Join("packs", "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, packs, "the starter packs are part of the agreement")
	for _, rel := range packs {
		data, rerr := os.ReadFile(rel)
		require.NoError(t, rerr)
		docs[rel] = data
	}
	for _, rel := range repoVoicePacks {
		data, rerr := os.ReadFile(filepath.Join(repoRoot(t), rel))
		require.NoError(t, rerr)
		docs[rel] = data
	}

	for name, data := range docs {
		t.Run(name, func(t *testing.T) {
			_, strictErr := DecodeProfileStrict(bytes.NewReader(data))
			refused := 0
			if strictErr != nil {
				refused = strings.Count(strictErr.Error(), "not found in type")
			}
			problems, err := UnknownKeys(data)
			require.NoError(t, err)
			assert.Len(t, problems, refused)
			for _, p := range problems {
				assert.True(t, strings.HasPrefix(p.Message, "unknown key ") || strings.HasPrefix(p.Message, "key "),
					"the walk placed %s itself rather than falling back to the decoder's wording", p.Field)
			}
		})
	}
}

func TestEveryWarningCarriesACode(t *testing.T) {
	p := (&VoiceProfile{
		Name:  "Docs",
		Tone:  ToneProfile{Formality: "calm and matter-of-fact"},
		Style: StyleRules{ProhibitedPatterns: []Pattern{{Regex: `\bsimply\b`}}},
		Channels: map[string]ChannelOverride{"docs": {
			Style: &StyleRules{},
		}},
	}).Carry("test", []TermRule{{Term: "utilize", Replacement: "use"}})
	problems := ValidateProfile(p)
	require.Empty(t, Blocking(problems))
	codes := map[string]bool{}
	for _, w := range Advisory(problems) {
		require.NotEmpty(t, w.Code, "warning on %s", w.Field)
		codes[w.Code] = true
	}
	assert.Equal(t, map[string]bool{
		CodeUnfamiliarValue:      true,
		CodeOverrideDropsPattern: true,
	}, codes)
}

// A retired severity key names the key that decides whether a rule fails.
func TestUnknownKeys_SeverityNamesAdvisory(t *testing.T) {
	found, err := UnknownKeys([]byte("name: X\nvocabulary:\n  forbidden_terms:\n    - term: utilize\n      severity: minor\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Field != "vocabulary.forbidden_terms[0].severity" {
		t.Fatalf("want one problem on the severity key, got %+v", found)
	}
	if !strings.Contains(found[0].Message, "advisory: true") {
		t.Errorf("the message names advisory: %q", found[0].Message)
	}
}
