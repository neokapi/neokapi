package tools_test

import (
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inferTestCorpus is a small but signal-rich brand corpus: contractions,
// exclamations, first-person-plural voice, short sentences, and a recurring
// capitalized product name — enough for the deterministic demo provider to
// infer a non-trivial draft.
const inferTestCorpus = `We're building Bowrain for teams that ship in every language.
Bowrain keeps your voice consistent, and we'll never slow you down!
Try Bowrain today. We keep it simple.`

func newInferDemoProvider() aiprovider.LLMProvider {
	aiprovider.SetDemoNoticeWriter(io.Discard)
	return aiprovider.NewDemoProvider(aiprovider.Config{})
}

func TestInferVoiceProfileDemoProviderDraft(t *testing.T) {
	p := newInferDemoProvider()
	ctx := t.Context()

	draft, evidence, err := tools.InferVoiceProfile(ctx, p, inferTestCorpus, tools.InferOptions{ProfileName: "Acme Voice"})
	require.NoError(t, err)
	require.NotNil(t, draft)
	require.NotNil(t, evidence)

	// The draft maps into the real brand.VoiceProfile shape with the caller's name.
	assert.Equal(t, "Acme Voice", draft.Name)
	assert.NotEmpty(t, draft.Description)

	// Tone: non-empty and schema-valid (enum members, not free text).
	assert.NotEmpty(t, draft.Tone.Personality)
	assert.Contains(t, []string{"casual", "neutral", "formal", "technical"}, draft.Tone.Formality)
	assert.Contains(t, []string{"none", "light", "frequent"}, draft.Tone.Humor)
	assert.NotEmpty(t, draft.Tone.Emotion)
	assert.NotEmpty(t, draft.Tone.Guidelines)

	// Style: enum-valid and consistent with the corpus signals — the corpus
	// has contractions, "we", and short sentences, so this is not a
	// trivially-true assertion against neutral/empty output.
	assert.Contains(t, []string{"short", "medium", "varied"}, draft.Style.SentenceLength)
	assert.Equal(t, "first_plural", draft.Style.PersonPOV)
	assert.Contains(t, []string{"always", "sometimes"}, draft.Style.Contractions,
		"corpus is full of contractions; 'never' would mean the corpus was ignored")
	assert.Equal(t, "casual", draft.Tone.Formality)

	// Vocabulary: the recurring capitalized product name surfaces as a
	// preferred term.
	require.NotEmpty(t, draft.Vocabulary.PreferredTerms)
	names := make([]string, 0, len(draft.Vocabulary.PreferredTerms))
	for _, tr := range draft.Vocabulary.PreferredTerms {
		names = append(names, tr.Term)
		assert.NotEmpty(t, tr.Note)
	}
	assert.Contains(t, names, "Bowrain")

	// Examples: at least one before/after pair with all parts populated.
	require.NotEmpty(t, draft.Examples)
	assert.NotEmpty(t, draft.Examples[0].Before)
	assert.NotEmpty(t, draft.Examples[0].After)
	assert.NotEmpty(t, draft.Examples[0].Explanation)
	assert.NotEqual(t, draft.Examples[0].Before, draft.Examples[0].After)

	// Evidence sidecar: every top-level field carries a confidence in (0,1]
	// and a non-empty source note.
	require.NotNil(t, evidence.Fields)
	for _, field := range []string{"tone", "style", "vocabulary", "examples"} {
		fe, ok := evidence.Fields[field]
		require.True(t, ok, "missing evidence for field %q", field)
		assert.Greater(t, fe.Confidence, 0.0, "field %q", field)
		assert.LessOrEqual(t, fe.Confidence, 1.0, "field %q", field)
		assert.NotEmpty(t, fe.Source, "field %q", field)
	}
}

func TestInferVoiceProfileDeterministic(t *testing.T) {
	p := newInferDemoProvider()
	ctx := t.Context()
	opts := tools.InferOptions{ProfileName: "Repeat Voice", Domain: "developer tools"}

	d1, e1, err := tools.InferVoiceProfile(ctx, p, inferTestCorpus, opts)
	require.NoError(t, err)
	d2, e2, err := tools.InferVoiceProfile(ctx, p, inferTestCorpus, opts)
	require.NoError(t, err)

	assert.Equal(t, d1, d2, "demo inference must be deterministic")
	assert.Equal(t, e1, e2, "demo evidence must be deterministic")
}

func TestInferVoiceProfileEmptyCorpus(t *testing.T) {
	p := newInferDemoProvider()
	ctx := t.Context()

	draft, evidence, err := tools.InferVoiceProfile(ctx, p, "   \n\t  ", tools.InferOptions{})
	require.ErrorIs(t, err, tools.ErrEmptyCorpus)
	assert.Nil(t, draft)
	assert.Nil(t, evidence)
}

func TestInferVoiceProfileTinyCorpus(t *testing.T) {
	p := newInferDemoProvider()
	ctx := t.Context()

	// A single short sentence: must not panic or nil-deref, and yields a
	// schema-valid (if low-evidence) draft.
	draft, evidence, err := tools.InferVoiceProfile(ctx, p, "Hello.", tools.InferOptions{})
	require.NoError(t, err)
	require.NotNil(t, draft)
	require.NotNil(t, evidence)

	assert.Equal(t, "Inferred Brand Voice", draft.Name) // default name applied
	assert.NotEmpty(t, draft.Tone.Personality)
	assert.Contains(t, []string{"casual", "neutral", "formal", "technical"}, draft.Tone.Formality)
	assert.Contains(t, []string{"always", "sometimes", "never"}, draft.Style.Contractions)
	assert.Contains(t, []string{"first_plural", "second", "third"}, draft.Style.PersonPOV)
	require.Contains(t, evidence.Fields, "tone")
	assert.Greater(t, evidence.Fields["tone"].Confidence, 0.0)
}

func TestBrandVoiceInferToolProcess(t *testing.T) {
	p := newInferDemoProvider()
	tl := tools.NewBrandVoiceInferTool(p, tools.BrandVoiceInferConfig{ProfileName: "Stream Voice"})

	ctx := t.Context()
	in := make(chan *model.Part, 4)
	out := make(chan *model.Part, 4)

	texts := []string{
		"We're building Bowrain for teams that ship in every language.",
		"Bowrain keeps your voice consistent, and we'll never slow you down!",
		"Try Bowrain today. We keep it simple.",
	}
	for i, text := range texts {
		block := model.NewBlock("tu"+string(rune('1'+i)), text)
		in <- &model.Part{Type: model.PartBlock, Resource: block}
	}
	close(in)

	err := tl.Process(ctx, in, out)
	require.NoError(t, err)

	// All parts pass through unchanged, in order.
	require.Len(t, out, len(texts))
	for _, text := range texts {
		part := <-out
		assert.Equal(t, text, part.Resource.(*model.Block).SourceText())
	}

	// The corpus-level draft is available on the tool after the stream ends.
	draft, evidence := tl.Draft()
	require.NotNil(t, draft)
	require.NotNil(t, evidence)
	assert.Equal(t, "Stream Voice", draft.Name)
	assert.NotEmpty(t, draft.Tone.Personality)
	assert.Contains(t, evidence.Fields, "tone")

	// Provider usage was accounted.
	assert.Positive(t, tl.TotalUsage().TotalTokens())
}

func TestBrandVoiceInferToolEmptyStream(t *testing.T) {
	p := newInferDemoProvider()
	tl := tools.NewBrandVoiceInferTool(p, tools.BrandVoiceInferConfig{})

	ctx := t.Context()
	in := make(chan *model.Part)
	out := make(chan *model.Part, 1)
	close(in)

	// An empty stream is not a flow error — there is just nothing to infer.
	err := tl.Process(ctx, in, out)
	require.NoError(t, err)

	draft, evidence := tl.Draft()
	assert.Nil(t, draft)
	assert.Nil(t, evidence)
}

func TestBrandVoiceInferRegistered(t *testing.T) {
	s := tools.BrandVoiceInferSchema()
	require.NotNil(t, s)
	assert.Equal(t, "brand-voice-infer", s.ID)
	require.NotNil(t, s.ToolMeta)
	assert.Contains(t, s.ToolMeta.Requires, "credentials")
	sideEffects := make([]string, 0, len(s.ToolMeta.SideEffects))
	for _, e := range s.ToolMeta.SideEffects {
		sideEffects = append(sideEffects, string(e))
	}
	assert.Contains(t, sideEffects, "api-call")

	// Config factory path constructs the tool with the demo provider.
	tl, err := tools.NewBrandVoiceInferFromConfig(map[string]any{
		"provider":    "demo",
		"profileName": "Factory Voice",
	}, "")
	require.NoError(t, err)
	assert.Equal(t, "brand-voice-infer", tl.Name())
}
