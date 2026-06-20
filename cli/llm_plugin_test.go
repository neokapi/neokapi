//go:build !js

package cli

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// fakeLLMTransport records the last request and returns a canned response.
type fakeLLMTransport struct {
	last   llmWireRequest
	resp   llmWireResponse
	closed bool
}

func (f *fakeLLMTransport) generate(req llmWireRequest) (llmWireResponse, error) {
	f.last = req
	return f.resp, nil
}
func (f *fakeLLMTransport) Close() error { f.closed = true; return nil }

// withFake returns a provider wired to a fake transport (no subprocess).
func withFake(t *fakeLLMTransport, cfg aiprovider.Config) *llmProvider {
	p := newLLMProvider(cfg)
	p.transport = t // ensure()'s once-closure sees a non-nil transport and skips dial
	return p
}

func TestGemmaProviderIdentity(t *testing.T) {
	p := newLLMProvider(aiprovider.Config{})
	assert.Equal(t, gemmaProviderID, p.Name())
	assert.Equal(t, defaultGemmaModel, p.cfg.Model, "default model filled in")
	mods := p.InputModalities()
	assert.Contains(t, mods, aiprovider.ModalityImage)
	assert.Contains(t, mods, aiprovider.ModalityAudio)
}

func TestGemmaRegisteredAsLocalProvider(t *testing.T) {
	// The init() registration makes Gemma selectable, and it must be local.
	assert.Contains(t, aiprovider.ProviderNames(), string(gemmaProviderID))
	assert.True(t, aiprovider.IsLocalProvider(gemmaProviderID))
}

func TestGemmaChat(t *testing.T) {
	fake := &fakeLLMTransport{resp: llmWireResponse{Text: "Hello", InputTokens: 5, OutputTokens: 2}}
	p := withFake(fake, aiprovider.Config{Model: "gemma-4-e2b"})

	resp, err := p.Chat(context.Background(), []aiprovider.Message{
		aiprovider.TextMessage("user", "Bonjour"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello", resp.Content)
	assert.Equal(t, "gemma-4-e2b", resp.Model)
	assert.Equal(t, 5, resp.Usage.InputTokens)
	assert.Equal(t, 2, resp.Usage.OutputTokens)

	require.Len(t, fake.last.Messages, 1)
	assert.Equal(t, "user", fake.last.Messages[0].Role)
	assert.Equal(t, "Bonjour", fake.last.Messages[0].Text)
	assert.Equal(t, "generate", fake.last.Op)
}

func TestGemmaTranslateBuildsPrompt(t *testing.T) {
	fake := &fakeLLMTransport{resp: llmWireResponse{Text: "Hallo"}}
	p := withFake(fake, aiprovider.Config{})

	resp, err := p.Translate(context.Background(), aiprovider.TranslateRequest{
		Source:         "Hello",
		SourceLanguage: "en",
		TargetLocale:   "de",
	})
	require.NoError(t, err)
	assert.Equal(t, "Hallo", resp.Translation)
	assert.InDelta(t, 0.7, resp.Confidence, 0.001)

	require.Len(t, fake.last.Messages, 1)
	prompt := fake.last.Messages[0].Text
	assert.Contains(t, prompt, "Translate")
	assert.Contains(t, prompt, "de")
	assert.Contains(t, prompt, "Hello")
}

func TestGemmaChatStructuredPassesSchema(t *testing.T) {
	fake := &fakeLLMTransport{resp: llmWireResponse{Text: `{"ok":true}`}}
	p := withFake(fake, aiprovider.Config{})

	schema := aiprovider.JSONSchema{
		Name:   "result",
		Schema: map[string]any{"type": "object"},
	}
	_, err := p.ChatStructured(context.Background(), []aiprovider.Message{
		aiprovider.TextMessage("user", "give json"),
	}, schema)
	require.NoError(t, err)
	require.NotEmpty(t, fake.last.Schema)
	var got map[string]any
	require.NoError(t, json.Unmarshal(fake.last.Schema, &got))
	assert.Equal(t, "object", got["type"])
}

func TestGemmaChatRespectsContextCancellation(t *testing.T) {
	fake := &fakeLLMTransport{}
	p := withFake(fake, aiprovider.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Chat(ctx, []aiprovider.Message{aiprovider.TextMessage("user", "hi")})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestToWireMessagesInlineImageWritesTemp(t *testing.T) {
	msgs := []aiprovider.Message{{
		Role: "user",
		Parts: []aiprovider.ContentPart{
			aiprovider.TextPart("what is this?"),
			aiprovider.MediaPart(aiprovider.ContentImage, &model.Media{
				Data:     []byte("\x89PNGfake"),
				MimeType: "image/png",
			}),
		},
	}}
	wire, cleanup, err := toWireMessages(msgs)
	require.NoError(t, err)
	defer cleanup()

	require.Len(t, wire, 1)
	assert.Equal(t, "what is this?", wire[0].Text)
	require.Len(t, wire[0].Media, 1)
	assert.Equal(t, "image", wire[0].Media[0].Kind)
	assert.Equal(t, "image/png", wire[0].Media[0].MIME)

	path := wire[0].Media[0].Path
	b, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "\x89PNGfake", string(b))

	cleanup()
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "temp media file removed by cleanup")
}

func TestMaterializeMediaLocalURIPassThrough(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "img-*.png")
	require.NoError(t, err)
	_ = f.Close()

	path, tmp, err := materializeMedia(&model.Media{URI: f.Name()})
	require.NoError(t, err)
	assert.Equal(t, f.Name(), path)
	assert.Empty(t, tmp, "local URI is used in place, no temp written")
}

func TestMaterializeMediaBlobErrors(t *testing.T) {
	_, _, err := materializeMedia(&model.Media{BlobKey: "abc"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blob-store-backed")
}

func TestIsLocalFilePath(t *testing.T) {
	assert.True(t, isLocalFilePath("/tmp/x.png"))
	assert.True(t, isLocalFilePath("./x.png"))
	assert.True(t, isLocalFilePath("file:///tmp/x.png"))
	assert.False(t, isLocalFilePath("https://example.com/x.png"))
	assert.False(t, isLocalFilePath("data:image/png;base64,AAAA"))
	assert.False(t, isLocalFilePath(""))
}
