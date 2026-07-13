package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func basic() Translate {
	return Translate{SourceLocale: "en", TargetLocale: "fr"}
}

// The content to translate is the whole user turn and nothing else. It is data,
// never instruction: the inline-code path used to pass a fully built instruction
// as the source of another instruction, so the model was handed a prompt to
// translate. The user turn must contain no task framing.
func TestSingleUserTurnIsPureContent(t *testing.T) {
	t.Parallel()

	turns := basic().Single("Click <ph id=\"1\"/> to save", true)
	require.Len(t, turns, 2)

	assert.Equal(t, RoleSystem, turns[0].Role)
	assert.Equal(t, RoleUser, turns[1].Role)

	user := turns[1].Text
	assert.Equal(t, "Click <ph id=\"1\"/> to save", user)
	assert.NotContains(t, user, "Translate", "task framing must not leak into the content")
	assert.NotContains(t, user, "Return ONLY")
}

// The tag rule appears only when the block actually carries inline codes.
func TestSinglePreserveTagsGatesTagRule(t *testing.T) {
	t.Parallel()

	with := basic().Single("x", true)[0].Text
	without := basic().Single("x", false)[0].Text

	assert.Contains(t, with, "XML tags")
	assert.NotContains(t, without, "XML tags")
}

// Rendering is deterministic: the same inputs must produce byte-identical
// prompts, including glossary ordering, or the config fingerprint and any
// cached target drift against each other.
func TestRenderingIsDeterministic(t *testing.T) {
	t.Parallel()

	build := func() Translate {
		return Translate{
			SourceLocale: "en",
			TargetLocale: "fr",
			Instruction:  "Keep it informal.",
			VoiceGuide:   "Warm, direct.",
			Glossary:     map[string]string{"zeta": "zêta", "alpha": "alpha", "mu": "mû"},
		}
	}

	first := build().Single("Hello", false)
	for range 20 {
		assert.Equal(t, first, build().Single("Hello", false))
	}

	// Glossary terms are sorted, not map-ordered.
	sys := first[0].Text
	assert.Less(t, strings.Index(sys, "alpha"), strings.Index(sys, "mu"))
	assert.Less(t, strings.Index(sys, "mu"), strings.Index(sys, "zeta"))
}

// Directives carry instruction, brand voice and glossary into every path.
func TestDirectivesReachBothSingleAndBatch(t *testing.T) {
	t.Parallel()

	p := Translate{
		SourceLocale: "en",
		TargetLocale: "fr",
		Instruction:  "Keep it informal.",
		VoiceGuide:   "Warm, direct.",
		Glossary:     map[string]string{"utilize": "use"},
	}

	for name, sys := range map[string]string{
		"single": p.Single("x", false)[0].Text,
		"batch":  p.Batch([]string{"x"})[0].Text,
	} {
		assert.Contains(t, sys, "Keep it informal.", name)
		assert.Contains(t, sys, "Warm, direct.", name)
		assert.Contains(t, sys, "utilize → use", name)
	}
}

// Batch numbering is the response contract: the structured reply maps back by
// index, so the numbering must be 1-based and contiguous.
func TestBatchNumbersSegments(t *testing.T) {
	t.Parallel()

	turns := basic().Batch([]string{"alpha", "beta", "gamma"})
	require.Len(t, turns, 2)
	assert.Equal(t, "[1] alpha\n[2] beta\n[3] gamma", turns[1].Text)
}

// Empty directives add nothing — an unconfigured run must not ship stray
// headers to the model.
func TestNoDirectivesWhenUnset(t *testing.T) {
	t.Parallel()

	assert.Empty(t, basic().Directives())
	sys := basic().Single("x", false)[0].Text
	assert.NotContains(t, sys, "Glossary")
	assert.NotContains(t, sys, "Brand voice")
	assert.NotContains(t, sys, "Instruction")
}

// Meta carries structured intent so consumers never parse the prompt's text.
func TestMetaCarriesLocales(t *testing.T) {
	t.Parallel()

	m := basic().Meta(IDTranslateSingle)
	assert.Equal(t, IDTranslateSingle, m.ID)
	assert.Equal(t, Version, m.Version)
	assert.Equal(t, "en", m.Param("source_locale"))
	assert.Equal(t, "fr", m.Param("target_locale"))
}
