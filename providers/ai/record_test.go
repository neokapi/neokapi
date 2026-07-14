package aiprovider

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ai/prompt"
)

// recordAll installs a recorder for the duration of the test and returns the
// captured exchanges.
func recordAll(t *testing.T) *[]Exchange {
	t.Helper()

	var mu sync.Mutex
	var got []Exchange
	SetRecorder(func(e Exchange) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e)
	})
	t.Cleanup(func() { SetRecorder(nil) })
	return &got
}

// Translate must be captured exactly once. A provider's Translate calls its own
// Chat, bypassing the recording wrapper, so the exchange is recorded inside
// standardTranslate — and the wrapper must not also record it.
func TestRecorderCapturesTranslateExactlyOnce(t *testing.T) {
	got := recordAll(t)

	p, err := NewProvider(Demo, Config{})
	require.NoError(t, err)

	_, err = p.Translate(t.Context(), TranslateRequest{
		Source: "Save", SourceLanguage: "en", TargetLocale: "fr",
	})
	require.NoError(t, err)

	require.Len(t, *got, 1, "translate must record exactly one exchange, not zero and not two")

	ex := (*got)[0]
	assert.Equal(t, prompt.IDTranslateSingle, ex.Prompt)
	assert.Equal(t, prompt.Version, ex.Version)
	assert.Equal(t, string(Demo), ex.Provider)

	// The recorded messages are what the provider was handed: a system turn with
	// the task framing, and a user turn holding only the content.
	require.Len(t, ex.Messages, 2)
	assert.Equal(t, RoleSystem, ex.Messages[0].Role)
	assert.Contains(t, ex.Messages[0].Text(), "localization specialist")
	assert.Equal(t, RoleUser, ex.Messages[1].Role)
	assert.Equal(t, "Save", ex.Messages[1].Text())

	assert.NotEmpty(t, ex.Response)
}

// Direct ChatStructured calls (batch translate, brand voice, entity extraction)
// go through the wrapper and must be captured with their output schema.
func TestRecorderCapturesStructuredCall(t *testing.T) {
	got := recordAll(t)

	p, err := NewProvider(Demo, Config{})
	require.NoError(t, err)

	pt := prompt.Translate{SourceLocale: "en", TargetLocale: "fr"}
	ctx := prompt.WithMeta(t.Context(), pt.Meta(prompt.IDTranslateBatch))

	_, err = p.ChatStructured(ctx, MessagesFromTurns(pt.Batch(prompt.BatchSegments([]string{"Save", "Cancel"}))),
		JSONSchema{Name: "batch_translations", Schema: map[string]any{"type": "object"}})
	require.NoError(t, err)

	require.Len(t, *got, 1)
	ex := (*got)[0]
	assert.Equal(t, prompt.IDTranslateBatch, ex.Prompt)
	require.NotNil(t, ex.Schema)
	assert.Equal(t, "batch_translations", ex.Schema.Name)
	assert.Contains(t, ex.Messages[1].Text(), `"text": "Save"`)
}

// Wrapping must not cost a provider its streaming support — the translate tool
// type-asserts for StreamingLLMProvider to show live progress.
func TestRecordingPreservesStreamingInterface(t *testing.T) {
	recordAll(t)

	p, err := NewProvider(Demo, Config{})
	require.NoError(t, err)

	_, ok := p.(StreamingLLMProvider)
	assert.True(t, ok, "recording wrapper must preserve StreamingLLMProvider")
}

// With no recorder installed, providers are returned unwrapped — an ordinary run
// pays nothing for the diagnostic.
func TestNoRecorderLeavesProviderUnwrapped(t *testing.T) {
	SetRecorder(nil)

	p, err := NewProvider(Demo, Config{})
	require.NoError(t, err)

	_, wrapped := p.(*recordingStreamingProvider)
	assert.False(t, wrapped)
}
