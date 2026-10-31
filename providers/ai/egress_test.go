package aiprovider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/redaction"
)

func egressCtx(t *testing.T) (context.Context, *redaction.SecretAnnotation) {
	t.Helper()
	det, err := redaction.NewRuleDetector([]redaction.Rule{{Term: "Ada Lovelace", Category: "person"}})
	require.NoError(t, err)
	entry := &redaction.SecretAnnotation{
		Policy: &redaction.Policy{Detector: det, Locale: "en"},
		Values: map[string]redaction.RedactedValue{
			"rdx1": {Token: "rdx1", Category: "person", Disp: "[REDACTED:Person]", Original: "Ada Lovelace", Locale: "en"},
		},
	}
	return entry.BindContext(context.Background()), entry
}

// Every text part and section of a chat goes out redacted, and the recorder
// sees the request as it is sent.
func TestRedacting_RedactsEveryMessageOfAChat(t *testing.T) {
	var recorded []Exchange
	remove := AddRecorder(func(_ context.Context, ex Exchange) { recorded = append(recorded, ex) })
	defer remove()

	mock := NewMockProvider()
	p := Redacting(Recording(mock))
	ctx, _ := egressCtx(t)
	turns := []prompt.Turn{
		prompt.System(prompt.Section{Kind: prompt.KindVoice, Origin: "voice profile", Text: "Address Ada Lovelace formally."}),
		prompt.User(prompt.Section{Kind: prompt.KindContent, Origin: "text", Text: "Write to [REDACTED:Person] today."}),
	}
	_, err := p.Chat(ctx, MessagesFromTurns(turns))
	require.NoError(t, err)

	require.Len(t, mock.ChatCalls, 1)
	raw, err := json.Marshal(mock.ChatCalls[0])
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "Lovelace")
	assert.Equal(t, "Address [REDACTED:Person] formally.", mock.ChatCalls[0][0].Text())
	require.Len(t, recorded, 1)
	raw, err = json.Marshal(recorded[0])
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "Lovelace", "the recorder sees what was sent")
}

// A translate request is redacted field by field: the prompt the provider
// renders from it holds no withheld value.
func TestRedacting_RedactsEveryFieldOfATranslateRequest(t *testing.T) {
	mock := NewMockProvider()
	p := Redacting(mock)
	ctx, _ := egressCtx(t)
	req := TranslateRequest{
		Source:         `Write to <x id="rdx1/"/> today.`,
		SourceLanguage: "en",
		TargetLocale:   "fr",
		PreferredTerms: map[string]string{"Ada Lovelace": "Ada Lovelace"},
		DoNotTranslate: []string{"Ada Lovelace"},
		VoiceGuide:     "Ada Lovelace is addressed formally.",
		Instruction:    "Keep Ada Lovelace in English.",
		BlockContext: prompt.Context{
			Key:    "ada_lovelace",
			Before: []string{"Dear Ada Lovelace,"},
			After:  []string{"Regards, Ada Lovelace"},
			Prior:  &prompt.PriorVersion{Source: "Write to Ada Lovelace now.", Target: "Écrivez à Ada Lovelace maintenant."},
		},
	}
	_, err := p.Translate(ctx, req)
	require.NoError(t, err)
	require.Len(t, mock.TranslateCalls, 1)
	sent := mock.TranslateCalls[0]
	raw, err := json.Marshal(sent)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "Lovelace")
	assert.Equal(t, `Write to <x id="rdx1/"/> now.`, sent.BlockContext.Prior.Source,
		"the prior version shows the value as the source does")
	assert.Equal(t, map[string]string{`<x id="rdx1/"/>`: `<x id="rdx1/"/>`}, sent.PreferredTerms)
}

// With no Egress on the context, a request goes out as it was given.
func TestRedacting_LeavesARequestWithNoPolicyAsItIs(t *testing.T) {
	mock := NewMockProvider()
	_, err := Redacting(mock).Chat(context.Background(), []Message{TextMessage(RoleUser, "Write to Ada Lovelace.")})
	require.NoError(t, err)
	assert.Equal(t, "Write to Ada Lovelace.", mock.ChatCalls[0][0].Text())
}

func TestRedacting_WrapsOnceAndKeepsStreaming(t *testing.T) {
	p := Redacting(NewMockProvider())
	assert.Same(t, p, Redacting(p))
	_, ok := p.(StreamingLLMProvider)
	assert.True(t, ok, "the mock streams, so the wrapper does")
	assert.IsType(t, &MockProvider{}, WithoutRedaction(p))
}
