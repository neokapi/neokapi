package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A field a profile leaves unset renders nothing, and a section with nothing
// set renders no heading.
func TestRenderVoiceGuideOmitsUnsetFields(t *testing.T) {
	p := &VoiceProfile{
		Name:        "Fernwell",
		Description: "Plain, for studio owners.",
		Tone:        ToneProfile{Formality: "neutral"},
		Vocabulary:  VocabularyRules{ForbiddenTerms: []TermRule{{Term: "seat", Replacement: "person"}}},
	}
	guide := RenderVoiceGuide(p)
	assert.Contains(t, guide, "- Formality: neutral\n")
	for _, empty := range []string{"Emotion:", "Humor:", "## Style Rules", "Sentence length:", "Point of view:", "Contractions:", "### Preferred Terms"} {
		assert.NotContains(t, guide, empty)
	}
	assert.Contains(t, guide, "### Forbidden Terms\n- ~~seat~~ → use **person**\n")

	assert.Equal(t, "# Voice Guide: Bare\n\n", RenderVoiceGuide(&VoiceProfile{Name: "Bare"}))
}

func TestRenderVoiceBrief(t *testing.T) {
	p := &VoiceProfile{
		Name:        "Fernwell",
		Description: "Plain, for studio owners who are not accountants",
		Tone:        ToneProfile{Formality: "neutral"},
		Style: StyleRules{
			SentenceLength:     "short",
			PersonPOV:          "second",
			ProhibitedPatterns: []Pattern{{Regex: `(?i)\b(?:endpoint|payload)\b`, Description: "implementation vocabulary"}},
		},
		Vocabulary: VocabularyRules{ForbiddenTerms: []TermRule{{Term: "seat", Replacement: "person"}}},
		Examples:   []VoiceExample{{Before: "Utilize the seat", After: "Use the person"}},
	}
	brief := RenderVoiceBrief(p)
	assert.Equal(t,
		"Voice: Fernwell. Plain, for studio owners who are not accountants. Neutral register, short sentences, second person (you).\n"+
			"\nAvoid:\n- implementation vocabulary (such as endpoint, payload)\n"+
			"\nRewrite in this direction:\n- \"Utilize the seat\" becomes \"Use the person\"\n",
		brief)
	assert.NotContains(t, brief, "person**", "vocabulary is left to the caller's merged list")
	assert.Equal(t, "Voice: Bare.\n", RenderVoiceBrief(&VoiceProfile{Name: "Bare"}))
}
