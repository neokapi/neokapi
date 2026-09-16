package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The terms a project marks do-not-translate reach the model as an instruction
// of their own. They cannot ride in the terminology map, which pairs a term
// with the wording to use instead: a do-not-translate term names no such
// wording, and the map drops it.
func TestSingleCarriesDoNotTranslateTerms(t *testing.T) {
	t.Parallel()

	p := basic()
	p.DoNotTranslate = []string{"kapi", "Bowrain"}

	turns := p.Single("Open the project.", false)
	require.Len(t, turns, 2)
	system := turns[0].Text

	assert.Contains(t, system, "kapi")
	assert.Contains(t, system, "Bowrain")
	assert.Contains(t, strings.ToLower(system), "do not translate")
	assert.Less(t, strings.Index(system, "Bowrain"), strings.Index(system, "kapi"),
		"terms are sorted, so one rule set renders byte-identical on every run")
}

// The batched path carries them too. A term protected on one path and not the
// other would be protected or not depending on how many blocks a run happened
// to pack into a call.
func TestBatchCarriesDoNotTranslateTerms(t *testing.T) {
	t.Parallel()

	p := basic()
	p.DoNotTranslate = []string{"kapi"}

	turns := p.Batch(BatchSegments([]string{"Open the project.", "Save the file."}))
	require.NotEmpty(t, turns)

	assert.Contains(t, turns[0].Text, "kapi")
	assert.Contains(t, strings.ToLower(turns[0].Text), "do not translate")
}

// An unconfigured run ships no stray heading.
func TestNoDoNotTranslateSectionWithoutTerms(t *testing.T) {
	t.Parallel()

	turns := basic().Single("Open the project.", false)
	require.Len(t, turns, 2)

	assert.NotContains(t, strings.ToLower(turns[0].Text), "do not translate")
}

// The prompt fingerprint is derived from the rendered text, so a term entering
// or leaving the protected list invalidates targets produced under the old
// prompt.
func TestFingerprintMovesWithDoNotTranslate(t *testing.T) {
	t.Parallel()

	bare := basic()
	protected := basic()
	protected.DoNotTranslate = []string{"kapi"}

	assert.NotEqual(t, bare.Fingerprint(), protected.Fingerprint())
}

// A do-not-translate term and a preferred rendering are different
// instructions, and the prompt states each as itself.
func TestDoNotTranslateIsNotRenderedAsARendering(t *testing.T) {
	t.Parallel()

	p := basic()
	p.DoNotTranslate = []string{"kapi"}
	p.PreferredTerms = map[string]string{"save": "lagre"}

	turns := p.Single("Open the project.", false)
	require.Len(t, turns, 2)
	system := turns[0].Text

	assert.Contains(t, system, "save → lagre")
	assert.NotContains(t, system, "kapi → ")
}
