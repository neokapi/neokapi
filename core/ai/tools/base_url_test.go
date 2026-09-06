package tools

import (
	"context"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// endpointRecorder reports whether the provider under test called it, which is
// the whole question: a provider built from a config that names an endpoint
// must send its request there.
//
// The tests point at ollama, whose default endpoint is a local one. A
// regression then reaches localhost rather than putting a test key on the wire
// to a vendor's public API.
func endpointRecorder(t *testing.T) (*httptest.Server, *bool) {
	t.Helper()
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"ok"},"done":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &called
}

// TestEveryAIToolCallsTheCredentialEndpoint: `kapi credentials add --base-url`
// stores an endpoint and the host injects it into a tool's config under
// baseURL. A tool whose config dropped the key built its provider with no
// endpoint and authenticated against the vendor's public API with a key meant
// for a private one.
func TestEveryAIToolCallsTheCredentialEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		config   map[string]any
		build    func(config map[string]any, targetLang string) (tool.Tool, error)
		provider func(t *testing.T, built tool.Tool) aiprovider.LLMProvider
	}{
		{
			name:  "translate",
			build: NewAITranslateFromConfig,
			provider: func(t *testing.T, built tool.Tool) aiprovider.LLMProvider {
				return built.(*AITranslateTool).provider
			},
		},
		{
			name:  "review",
			build: NewAIReviewFromConfig,
			provider: func(t *testing.T, built tool.Tool) aiprovider.LLMProvider {
				return built.(*AIReviewTool).provider
			},
		},
		{
			name:  "voice-check",
			build: NewVoiceCheckFromConfig,
			provider: func(t *testing.T, built tool.Tool) aiprovider.LLMProvider {
				return built.(*VoiceCheckTool).provider
			},
		},
		{
			name:  "voice-infer",
			build: NewVoiceInferFromConfig,
			provider: func(t *testing.T, built tool.Tool) aiprovider.LLMProvider {
				return built.(*VoiceInferTool).provider
			},
		},
		{
			name:  "term-extract",
			build: NewAITerminologyFromConfig,
			provider: func(t *testing.T, built tool.Tool) aiprovider.LLMProvider {
				return built.(*AITerminologyTool).provider
			},
		},
		{
			name:   "entity-extract",
			config: map[string]any{"engine": "llm"},
			build:  NewAIEntityExtractFromConfig,
			provider: func(t *testing.T, built tool.Tool) aiprovider.LLMProvider {
				return built.(*AIEntityExtractTool).llm
			},
		},
		{
			name:  "media-refine",
			build: NewMediaRefineFromConfig,
			provider: func(t *testing.T, built tool.Tool) aiprovider.LLMProvider {
				return built.(*MediaRefineTool).provider
			},
		},
		{
			name:   "qa",
			config: map[string]any{"mode": "ai"},
			build:  NewAICheckFromConfig,
			provider: func(t *testing.T, built tool.Tool) aiprovider.LLMProvider {
				return built.(*AICheckTool).provider
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, called := endpointRecorder(t)
			config := map[string]any{
				"provider": string(aiprovider.Ollama),
				"apiKey":   "k",
				"model":    "m",
				"baseURL":  srv.URL,
			}
			maps.Copy(config, c.config)

			built, err := c.build(config, "fr")
			require.NoError(t, err)
			prov := c.provider(t, built)
			require.NotNil(t, prov)

			_, _ = prov.Chat(context.Background(), []aiprovider.Message{
				aiprovider.TextMessage(aiprovider.RoleUser, "hi"),
			})
			assert.True(t, *called, "the tool built its provider without the credential's endpoint")
		})
	}
}

// TestEveryAIToolConfigDecodesTheEndpoint states the same requirement one level
// down, where a missing json tag is what breaks it: schema.ApplyConfig is a JSON
// round trip, so a field the map cannot reach is a field the host cannot set.
func TestEveryAIToolConfigDecodesTheEndpoint(t *testing.T) {
	const endpoint = "https://llm.internal/v1"
	config := map[string]any{"baseURL": endpoint}

	t.Run("translate", func(t *testing.T) {
		var cfg AITranslateConfig
		require.NoError(t, schema.ApplyConfig(config, &cfg))
		assert.Equal(t, endpoint, cfg.BaseURL)
	})
	t.Run("review", func(t *testing.T) {
		var cfg AIReviewConfig
		require.NoError(t, schema.ApplyConfig(config, &cfg))
		assert.Equal(t, endpoint, cfg.BaseURL)
	})
	t.Run("voice-check", func(t *testing.T) {
		var cfg VoiceCheckConfig
		require.NoError(t, schema.ApplyConfig(config, &cfg))
		assert.Equal(t, endpoint, cfg.BaseURL)
	})
	t.Run("voice-infer", func(t *testing.T) {
		var cfg VoiceInferConfig
		require.NoError(t, schema.ApplyConfig(config, &cfg))
		assert.Equal(t, endpoint, cfg.BaseURL)
	})
	t.Run("term-extract", func(t *testing.T) {
		var cfg AITerminologyConfig
		require.NoError(t, schema.ApplyConfig(config, &cfg))
		assert.Equal(t, endpoint, cfg.BaseURL)
	})
	t.Run("entity-extract", func(t *testing.T) {
		var cfg AIEntityExtractConfig
		require.NoError(t, schema.ApplyConfig(config, &cfg))
		assert.Equal(t, endpoint, cfg.BaseURL)
	})
	t.Run("media-refine", func(t *testing.T) {
		var cfg MediaRefineConfig
		require.NoError(t, schema.ApplyConfig(config, &cfg))
		assert.Equal(t, endpoint, cfg.BaseURL)
	})
	t.Run("qa", func(t *testing.T) {
		var cfg AICheckConfig
		require.NoError(t, schema.ApplyConfig(config, &cfg))
		assert.Equal(t, endpoint, cfg.BaseURL)
	})
	t.Run("segment-llm", func(t *testing.T) {
		var p LLMParams
		require.NoError(t, schema.ApplyConfig(map[string]any{"baseURL": endpoint, "apiKey": "k"}, &p))
		assert.Equal(t, endpoint, p.BaseURL)
		assert.Equal(t, "k", p.APIKey)
	})
}

// TestTheEndpointStaysOffTheForm: the host clears any endpoint a recipe carried
// and sets it only from a resolved credential, so offering it as an authoring
// field would advertise a key the host deletes. The same holds for the
// segmentation engine's resolved key.
func TestTheEndpointStaysOffTheForm(t *testing.T) {
	for name, s := range map[string]*schema.ComponentSchema{
		"translate":      TranslateSchema(),
		"review":         AIReviewSchema(),
		"voice-check":    VoiceCheckSchema(),
		"voice-infer":    VoiceInferSchema(),
		"term-extract":   AITerminologySchema(),
		"media-refine":   MediaRefineSchema(),
		"segment-engine": schema.FromStruct(&LLMParams{}, schema.ToolMeta{ID: "segment-engine-llm"}),
	} {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, s)
			assert.NotContains(t, s.Properties, "baseURL")
		})
	}
}

// TestSegmentLLMEngineCallsTheResolvedEndpoint: the engine's key and endpoint
// are resolved by the host and handed in through the params map, which is the
// only route into them.
func TestSegmentLLMEngineCallsTheResolvedEndpoint(t *testing.T) {
	srv, called := endpointRecorder(t)

	seg, err := segment.Build("llm", segment.BaseConfig{Language: "en"}, map[string]any{
		"provider": string(aiprovider.Ollama),
		"apiKey":   "k",
		"model":    "m",
		"baseURL":  srv.URL,
	})
	require.NoError(t, err)

	llm, ok := seg.(*llmSegmenter)
	require.True(t, ok)
	_, _ = llm.provider.Chat(context.Background(), []aiprovider.Message{
		aiprovider.TextMessage(aiprovider.RoleUser, "hi"),
	})
	assert.True(t, *called, "the segmentation engine ignored the resolved endpoint")
}
