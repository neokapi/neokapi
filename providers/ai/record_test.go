package aiprovider

import (
	"context"
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
	remove := AddRecorder(func(_ context.Context, e Exchange) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e)
	})
	t.Cleanup(remove)
	return &got
}

// Translate must be captured exactly once. A provider's Translate calls its own
// Chat, bypassing the recording wrapper, so the exchange is recorded inside
// StandardTranslate — and the wrapper must not also record it.
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
	assert.Contains(t, ex.Messages[0].Text(), "Translate the user's text")
	assert.Equal(t, RoleUser, ex.Messages[1].Role)
	assert.Equal(t, "Save", ex.Messages[1].Text())

	assert.NotEmpty(t, ex.Response)
}

// Direct ChatStructured calls (batch translate, voice profile, entity extraction)
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
	p, err := NewProvider(Demo, Config{})
	require.NoError(t, err)

	_, wrapped := p.(*recordingStreamingProvider)
	assert.False(t, wrapped)
}

// Two observers of the same calls. `kapi --explain-prompts` and a desktop
// session's activity log both want every exchange, and a single slot meant
// whichever registered second turned the first one off, leaving a transcript
// that came back empty with nothing to say why.
func TestTwoRecordersBothReceiveAndRemoveIndependently(t *testing.T) {
	var mu sync.Mutex
	var first, second []Exchange

	removeFirst := AddRecorder(func(_ context.Context, e Exchange) {
		mu.Lock()
		defer mu.Unlock()
		first = append(first, e)
	})
	removeSecond := AddRecorder(func(_ context.Context, e Exchange) {
		mu.Lock()
		defer mu.Unlock()
		second = append(second, e)
	})
	t.Cleanup(removeSecond)

	p, err := NewProvider(Demo, Config{})
	require.NoError(t, err)

	req := TranslateRequest{Source: "Save", SourceLanguage: "en", TargetLocale: "fr"}
	_, err = p.Translate(t.Context(), req)
	require.NoError(t, err)

	mu.Lock()
	assert.Len(t, first, 1)
	assert.Len(t, second, 1)
	mu.Unlock()

	// Removing one leaves the other recording.
	removeFirst()
	_, err = p.Translate(t.Context(), req)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, first, 1, "a removed recorder must stop receiving")
	assert.Len(t, second, 2, "the remaining recorder must keep receiving")
}

// The context reaches the recorder, which is how a host attributes an exchange
// to the work that made it (a review action, a convergence run) rather than
// showing an undifferentiated list.
func TestRecorderReceivesTheCallContext(t *testing.T) {
	type scopeKey struct{}

	var mu sync.Mutex
	var seen []string
	remove := AddRecorder(func(ctx context.Context, _ Exchange) {
		mu.Lock()
		defer mu.Unlock()
		if v, ok := ctx.Value(scopeKey{}).(string); ok {
			seen = append(seen, v)
		}
	})
	t.Cleanup(remove)

	p, err := NewProvider(Demo, Config{})
	require.NoError(t, err)

	ctx := context.WithValue(t.Context(), scopeKey{}, "review:fix-findings")
	_, err = p.Translate(ctx, TranslateRequest{
		Source: "Save", SourceLanguage: "en", TargetLocale: "fr",
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"review:fix-findings"}, seen)
}
