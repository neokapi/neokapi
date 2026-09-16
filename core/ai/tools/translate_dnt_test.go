package tools_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// systemTurn is the instruction half of a recorded call: what the model was
// told, as opposed to the content it was asked to translate. Asserting on the
// whole prompt would pass on a term the source text happens to contain.
func systemTurn(t *testing.T, msgs []aiprovider.Message) string {
	t.Helper()
	require.NotEmpty(t, msgs)
	return msgs[0].Text()
}

// A concept the terms store marks do-not-translate reaches the drafter. Before
// this, the rules were resolved, rendered through the term map, and dropped
// there for want of a replacement: the check failed a translated product name
// that the prompt had never asked anyone to keep.
func TestTranslateAsksTheModelToKeepADoNotTranslateTerm(t *testing.T) {
	t.Parallel()

	cfg := baseConfig()
	cfg.TermRules = []coreprofile.TermRule{{Term: "kapi", DoNotTranslate: true, ConceptID: "c-kapi"}}

	p := &recordingProvider{}
	runTranslate(t, cfg, p, blockNamed("a", "Open kapi to begin"))

	require.Len(t, p.prompts, 1)
	system := systemTurn(t, p.prompts[0])
	assert.Contains(t, strings.ToLower(system), "do not translate")
	assert.Contains(t, system, "kapi")
}

// The batched path carries it as well, so protection does not depend on how
// many blocks a run packed into one call.
func TestTranslateBatchAsksTheModelToKeepADoNotTranslateTerm(t *testing.T) {
	t.Parallel()

	cfg := baseConfig()
	cfg.BatchSize = 2
	cfg.BatchConcurrency = 1
	cfg.TermRules = []coreprofile.TermRule{{Term: "kapi", DoNotTranslate: true}}

	p := &recordingProvider{}
	runTranslate(t, cfg, p,
		blockNamed("a", "Open kapi to begin"),
		blockNamed("b", "Save the file in kapi"),
	)

	require.Len(t, p.prompts, 1, "a store's do-not-translate concept does not un-batch the run")
	assert.Contains(t, strings.ToLower(systemTurn(t, p.prompts[0])), "do not translate")
}

// Only the terms the text can use are sent, as with every other rule: what
// reaches the model is what the model needs.
func TestTranslateSendsOnlyTheDoNotTranslateTermsTheTextUses(t *testing.T) {
	t.Parallel()

	cfg := baseConfig()
	cfg.TermRules = []coreprofile.TermRule{
		{Term: "kapi", DoNotTranslate: true},
		{Term: "Bowrain", DoNotTranslate: true},
	}

	p := &recordingProvider{}
	runTranslate(t, cfg, p, blockNamed("a", "Open kapi to begin"))

	require.Len(t, p.prompts, 1)
	system := systemTurn(t, p.prompts[0])
	assert.Contains(t, system, "kapi")
	assert.NotContains(t, system, "Bowrain", "this block cannot use it")
}

// A term inside a command is still protected. term-check demands it there, and
// a prompt that dropped it would ask for content the gate then fails.
func TestTranslateProtectsATermInsideACommand(t *testing.T) {
	t.Parallel()

	cfg := baseConfig()
	cfg.TermRules = []coreprofile.TermRule{{Term: "kapi", DoNotTranslate: true}}

	p := &recordingProvider{}
	runTranslate(t, cfg, p, blockNamed("a", "Run `kapi up` to converge."))

	require.Len(t, p.prompts, 1)
	assert.Contains(t, systemTurn(t, p.prompts[0]), "kapi")
}
