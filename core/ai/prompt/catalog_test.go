package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The catalog is what the generated prompt reference is built from. These tests
// exist so the reference cannot silently stop describing what kapi actually
// sends — the failure mode that made the old hand-written prompt docs wrong.

// A prompt that exists but isn't in the catalog is an LLM call kapi makes with
// the user's content and never documents. This makes that unshippable.
func TestEveryPromptIDIsInTheCatalog(t *testing.T) {
	inCatalog := map[string]bool{}
	for _, e := range Catalog() {
		inCatalog[e.ID] = true
	}

	for _, id := range IDs() {
		assert.True(t, inCatalog[id], "prompt %q is sent but missing from Catalog() — it would not appear in the prompt reference", id)
	}
	assert.Len(t, Catalog(), len(IDs()), "Catalog() and IDs() must describe the same set of prompts")
}

// Every catalog entry must render real prompt text. An entry with no turns is a
// documented prompt whose body nobody can see.
func TestEveryCatalogEntryRendersText(t *testing.T) {
	for _, e := range Catalog() {
		t.Run(e.ID, func(t *testing.T) {
			require.NotEmpty(t, e.Turns, "no turns — the reference would render an empty prompt")
			require.NotEmpty(t, e.Summary, "no summary")
			require.NotEmpty(t, e.Tool, "no owning tool")

			var roles []string
			for _, turn := range e.Turns {
				assert.NotEmpty(t, turn.Sections, "turn %q has no sections", turn.Role)
				assert.NotEmpty(t, strings.TrimSpace(turn.Text), "turn %q renders empty", turn.Role)
				for _, s := range turn.Sections {
					assert.NotEmpty(t, s.Kind, "a section with no kind cannot be attributed to an owner")
					assert.NotEmpty(t, s.Origin, "section %q has no origin — the reference could not say who produced it", s.Kind)
				}
				roles = append(roles, turn.Role)
			}
			assert.Contains(t, roles, RoleUser, "every prompt must put the user's content in a user turn, never in the instructions")
		})
	}
}

// The refusal token is a contract between the media-refine prompt and the tool
// that compares against the reply. If the prompt stops naming it, the tool's
// illegible-detection silently dies.
func TestMediaRefineDemandsTheRefuseToken(t *testing.T) {
	for _, m := range []Modality{ModalityImage, ModalityAudio, ModalityVideo} {
		p := MediaRefine{Modality: m, RefuseToken: RefuseToken}
		turns := p.Turns([]string{"a neighbouring line"})

		system, _ := turns[0], turns[1]
		assert.Equal(t, RoleSystem, system.Role)
		assert.Contains(t, system.Text, RefuseToken, "the %s prompt must tell the model how to refuse", m)
		assert.NotEmpty(t, p.Attachment(), "a media prompt ships binary data; the reference must name it")
	}
}

// A prompt that reorders itself between identical runs breaks caching and makes
// runs irreproducible. entity-extract used to range over a map of known terms.
func TestEntityExtractRendersDeterministically(t *testing.T) {
	p := EntityExtract{
		Locale:     "en",
		KnownTerms: []string{"widget", "account", "dashboard", "Bowrain"},
	}
	blocks := []EntityBlock{{ID: "b1", Text: "Open the dashboard."}}

	first := p.Turns(blocks)[0].Text
	for range 20 {
		assert.Equal(t, first, p.Turns(blocks)[0].Text, "the same terms must render the same prompt every time")
	}
	// Sorted, not insertion-ordered.
	assert.Contains(t, first, "Bowrain, account, dashboard, widget")
}

// Segmentation that rewrites the text cannot be mapped back onto the source
// block, so the verbatim rule is load-bearing rather than stylistic.
func TestSegmentKeepsTheVerbatimRule(t *testing.T) {
	turns := Segment{Language: "en"}.Turns("Hello. World.")
	require.Len(t, turns, 2)

	assert.Contains(t, turns[0].Text, "verbatim")
	assert.Equal(t, "Hello. World.", turns[1].Text, "the text to segment is content, not instruction")
}

// The optional inputs are what a user steers with; they must actually reach the
// prompt, and must be absent when unset.
func TestSegmentOptionalInputsReachThePrompt(t *testing.T) {
	bare := Segment{}.Turns("x")[0].Text
	assert.NotContains(t, bare, "Additional instruction")
	assert.NotContains(t, bare, "no longer than")

	full := Segment{MaxChunkRunes: 200, Instruction: "split on list items"}.Turns("x")[0].Text
	assert.Contains(t, full, "about 200 characters")
	assert.Contains(t, full, "split on list items")
}
