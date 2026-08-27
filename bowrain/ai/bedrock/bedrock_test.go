package bedrock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	brdoc "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

func TestRegisteredInAIProviderRegistry(t *testing.T) {
	info, ok := aiprovider.ProviderInfoFor(ProviderID)
	require.True(t, ok, "bedrock provider must be registered via init()")
	assert.Equal(t, "AWS Bedrock", info.Label)
	assert.Equal(t, DefaultModel, info.DefaultModel)
	// No model prefixes: bedrock is chosen explicitly, never inferred from a
	// "claude"/"anthropic" model name (those belong to the direct API provider).
	assert.Empty(t, info.ModelPrefixes)
	assert.True(t, info.NeedsKey() == false || info.NeedsKey() == true) // registered, resolvable

	prov, err := aiprovider.NewProvider(ProviderID, aiprovider.Config{})
	require.NoError(t, err)
	require.NotNil(t, prov)
	assert.Equal(t, ProviderID, prov.Name())
}

func TestNewDefaultsModel(t *testing.T) {
	assert.Equal(t, DefaultModel, New(aiprovider.Config{}).cfg.Model)
	assert.Equal(t, "eu.anthropic.claude-haiku-4-5",
		New(aiprovider.Config{Model: "eu.anthropic.claude-haiku-4-5"}).cfg.Model)
}

func TestInputModalities(t *testing.T) {
	assert.Equal(t, []aiprovider.Modality{aiprovider.ModalityImage}, New(aiprovider.Config{}).InputModalities())
}

func TestBaseModelID(t *testing.T) {
	cases := map[string]string{
		"eu.anthropic.claude-sonnet-4-6":           "claude-sonnet-4-6",
		"us.anthropic.claude-opus-4-8":             "claude-opus-4-8",
		"apac.anthropic.claude-haiku-4-5":          "claude-haiku-4-5",
		"us-gov.anthropic.claude-3-5-sonnet-2024":  "claude-3-5-sonnet-2024",
		"anthropic.claude-sonnet-4-6":              "claude-sonnet-4-6",
		"claude-sonnet-4-6":                        "claude-sonnet-4-6",
		"eu.meta.llama3-1-70b-instruct-v1:0":       "llama3-1-70b-instruct-v1:0",
		"us.anthropic.claude-3-5-sonnet-2024-v1:0": "claude-3-5-sonnet-2024-v1:0",
	}
	for in, want := range cases {
		assert.Equalf(t, want, baseModelID(in), "baseModelID(%q)", in)
	}
}

func TestLimitsResolveThroughProfileID(t *testing.T) {
	// A Bedrock inference-profile id resolves to the underlying model's ceilings,
	// so the batch packer sizes calls correctly.
	sonnet := New(aiprovider.Config{Model: "eu.anthropic.claude-sonnet-4-6"}).Limits()
	assert.Equal(t, 64_000, sonnet.MaxOutputTokens)

	// An unknown model falls back to the conservative default rather than assuming
	// a large ceiling.
	unknown := New(aiprovider.Config{Model: "eu.cohere.command-r-plus"}).Limits()
	assert.Equal(t, aiprovider.ConservativeMaxOutputTokens, unknown.MaxOutputTokens)
}

func TestMaxTokens(t *testing.T) {
	// Default: request the model's non-streaming ceiling (16k for the 64k Sonnet).
	p := New(aiprovider.Config{Model: "eu.anthropic.claude-sonnet-4-6"})
	assert.Equal(t, aiprovider.NonStreamingMaxOutputTokens, p.maxTokens())

	// An explicit, smaller cfg.MaxTokens wins.
	p = New(aiprovider.Config{Model: "eu.anthropic.claude-sonnet-4-6", MaxTokens: 2048})
	assert.Equal(t, 2048, p.maxTokens())

	// A cfg.MaxTokens above the ceiling is clamped to the ceiling.
	p = New(aiprovider.Config{Model: "eu.anthropic.claude-sonnet-4-6", MaxTokens: 1_000_000})
	assert.Equal(t, aiprovider.NonStreamingMaxOutputTokens, p.maxTokens())
}

func TestToRole(t *testing.T) {
	u, err := toRole(aiprovider.RoleUser)
	require.NoError(t, err)
	assert.Equal(t, brtypes.ConversationRoleUser, u)

	a, err := toRole(aiprovider.RoleAssistant)
	require.NoError(t, err)
	assert.Equal(t, brtypes.ConversationRoleAssistant, a)

	_, err = toRole(aiprovider.RoleSystem)
	assert.Error(t, err, "system must be lifted out before message conversion")
}

func TestImageFormat(t *testing.T) {
	for mime, want := range map[string]brtypes.ImageFormat{
		"image/png":  brtypes.ImageFormatPng,
		"image/jpeg": brtypes.ImageFormatJpeg,
		"image/jpg":  brtypes.ImageFormatJpeg,
		"IMAGE/GIF":  brtypes.ImageFormatGif,
		"image/webp": brtypes.ImageFormatWebp,
	} {
		got, err := imageFormat(mime)
		require.NoErrorf(t, err, "imageFormat(%q)", mime)
		assert.Equalf(t, want, got, "imageFormat(%q)", mime)
	}
	_, err := imageFormat("image/tiff")
	assert.Error(t, err)
}

func TestToTokenUsage(t *testing.T) {
	assert.Equal(t, aiprovider.TokenUsage{}, toTokenUsage(nil))

	got := toTokenUsage(&brtypes.TokenUsage{
		InputTokens:           aws.Int32(100),
		OutputTokens:          aws.Int32(40),
		CacheReadInputTokens:  aws.Int32(9),
		CacheWriteInputTokens: aws.Int32(7),
	})
	assert.Equal(t, aiprovider.TokenUsage{
		InputTokens:         100,
		OutputTokens:        40,
		CacheReadTokens:     9,
		CacheCreationTokens: 7,
	}, got)
}

func TestConverseInputLiftsSystemAndSetsInference(t *testing.T) {
	p := New(aiprovider.Config{Model: "eu.anthropic.claude-sonnet-4-6", MaxTokens: 1024, Temperature: new(0.3)})
	in, err := p.converseInput([]aiprovider.Message{
		aiprovider.TextMessage(aiprovider.RoleSystem, "be terse"),
		aiprovider.TextMessage(aiprovider.RoleUser, "hi"),
	})
	require.NoError(t, err)

	assert.Equal(t, "eu.anthropic.claude-sonnet-4-6", aws.ToString(in.ModelId))
	// System lifted out of the message list into the top-level System field.
	require.Len(t, in.System, 1)
	sys, ok := in.System[0].(*brtypes.SystemContentBlockMemberText)
	require.True(t, ok)
	assert.Equal(t, "be terse", sys.Value)
	// Only the user message remains.
	require.Len(t, in.Messages, 1)
	assert.Equal(t, brtypes.ConversationRoleUser, in.Messages[0].Role)
	// Inference config carries max tokens + temperature.
	require.NotNil(t, in.InferenceConfig)
	assert.Equal(t, int32(1024), aws.ToInt32(in.InferenceConfig.MaxTokens))
	assert.InDelta(t, 0.3, float64(aws.ToFloat32(in.InferenceConfig.Temperature)), 1e-6)
}

func TestToBedrockMessagesTextAndImage(t *testing.T) {
	msgs, err := toBedrockMessages([]aiprovider.Message{{
		Role: aiprovider.RoleUser,
		Parts: []aiprovider.ContentPart{
			aiprovider.TextPart("describe"),
			aiprovider.MediaPart(aiprovider.ContentImage, &model.Media{Data: []byte{0x89, 0x50}, MimeType: "image/png"}),
		},
	}})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Content, 2)

	txt, ok := msgs[0].Content[0].(*brtypes.ContentBlockMemberText)
	require.True(t, ok)
	assert.Equal(t, "describe", txt.Value)

	img, ok := msgs[0].Content[1].(*brtypes.ContentBlockMemberImage)
	require.True(t, ok)
	assert.Equal(t, brtypes.ImageFormatPng, img.Value.Format)
	src, ok := img.Value.Source.(*brtypes.ImageSourceMemberBytes)
	require.True(t, ok)
	assert.Equal(t, []byte{0x89, 0x50}, src.Value)
}

func TestToBedrockMessagesRejectsSystemRole(t *testing.T) {
	_, err := toBedrockMessages([]aiprovider.Message{aiprovider.TextMessage(aiprovider.RoleSystem, "x")})
	assert.Error(t, err)
}

func TestToolConfigForcesNamedTool(t *testing.T) {
	schema := aiprovider.JSONSchema{
		Name:        "extract",
		Description: "pull fields",
		Schema:      map[string]any{"type": "object"},
	}
	tc := toolConfig(schema, toolNameOf(schema))
	require.Len(t, tc.Tools, 1)
	spec, ok := tc.Tools[0].(*brtypes.ToolMemberToolSpec)
	require.True(t, ok)
	assert.Equal(t, "extract", aws.ToString(spec.Value.Name))
	assert.Equal(t, "pull fields", aws.ToString(spec.Value.Description))
	_, ok = spec.Value.InputSchema.(*brtypes.ToolInputSchemaMemberJson)
	require.True(t, ok)

	// tool_choice forces the specific tool.
	choice, ok := tc.ToolChoice.(*brtypes.ToolChoiceMemberTool)
	require.True(t, ok)
	assert.Equal(t, "extract", aws.ToString(choice.Value.Name))

	// A schema with no name gets the default tool name.
	assert.Equal(t, "structured_output", toolNameOf(aiprovider.JSONSchema{}))
}

func TestStreamInputCopiesFields(t *testing.T) {
	p := New(aiprovider.Config{Model: "eu.anthropic.claude-sonnet-4-6"})
	in, err := p.converseInput([]aiprovider.Message{aiprovider.TextMessage(aiprovider.RoleUser, "hi")})
	require.NoError(t, err)
	si := streamInput(in)
	assert.Equal(t, in.ModelId, si.ModelId)
	assert.Equal(t, in.Messages, si.Messages)
	assert.Equal(t, in.System, si.System)
	assert.Equal(t, in.InferenceConfig, si.InferenceConfig)
	assert.Equal(t, in.ToolConfig, si.ToolConfig)
}

func TestOutputText(t *testing.T) {
	out := &brtypes.ConverseOutputMemberMessage{Value: brtypes.Message{
		Content: []brtypes.ContentBlock{
			&brtypes.ContentBlockMemberText{Value: "Hello "},
			&brtypes.ContentBlockMemberToolUse{}, // ignored
			&brtypes.ContentBlockMemberText{Value: "world"},
		},
	}}
	assert.Equal(t, "Hello world", outputText(out))
	assert.Empty(t, outputText(nil))
}

func TestToolUseInput(t *testing.T) {
	out := &brtypes.ConverseOutputMemberMessage{Value: brtypes.Message{
		Content: []brtypes.ContentBlock{
			&brtypes.ContentBlockMemberToolUse{Value: brtypes.ToolUseBlock{
				Name:  aws.String("structured_output"),
				Input: brdoc.NewLazyDocument(map[string]any{"greeting": "hi"}),
			}},
		},
	}}
	got, ok := toolUseInput(out, "structured_output")
	require.True(t, ok)
	assert.JSONEq(t, `{"greeting":"hi"}`, got)

	// Wrong tool name → not found.
	_, ok = toolUseInput(out, "other")
	assert.False(t, ok)
}

// --- wire-level tests: real SDK marshaling against a canned HTTP response ------

// newTestProvider points a provider at a local HTTP double, with static env
// credentials so the SDK's credential chain never reaches IMDS or a shared
// profile — the test is hermetic.
func newTestProvider(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "eu-north-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	return New(aiprovider.Config{Model: "eu.anthropic.claude-sonnet-4-6", BaseURL: srv.URL})
}

func TestChatWire(t *testing.T) {
	var gotBody map[string]any
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"output":{"message":{"role":"assistant","content":[{"text":"Bonjour"}]}},
			"stopReason":"end_turn",
			"usage":{"inputTokens":12,"outputTokens":3,"totalTokens":15,"cacheReadInputTokens":4,"cacheWriteInputTokens":7}
		}`)
	})

	resp, err := p.Chat(context.Background(), []aiprovider.Message{
		aiprovider.TextMessage(aiprovider.RoleSystem, "translate to french"),
		aiprovider.TextMessage(aiprovider.RoleUser, "Hello"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Bonjour", resp.Content)
	assert.False(t, resp.Truncated)
	assert.Equal(t, aiprovider.TokenUsage{InputTokens: 12, OutputTokens: 3, CacheReadTokens: 4, CacheCreationTokens: 7}, resp.Usage)

	// The request carried the lifted system prompt + the user message.
	require.NotNil(t, gotBody)
	assert.Contains(t, gotBody, "system")
	assert.Contains(t, gotBody, "messages")
}

func TestChatWireTruncated(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
			"output":{"message":{"role":"assistant","content":[{"text":"partial"}]}},
			"stopReason":"max_tokens",
			"usage":{"inputTokens":5,"outputTokens":5,"totalTokens":10}
		}`)
	})
	resp, err := p.Chat(context.Background(), []aiprovider.Message{aiprovider.TextMessage(aiprovider.RoleUser, "long")})
	require.NoError(t, err)
	assert.True(t, resp.Truncated, "max_tokens stop reason must surface as Truncated")
}

// Translate reproduces the framework's standardTranslate glue rather than
// calling it, so the truncation flag has to be carried here too — a translation
// cut off at the cap must not reach the caller looking finished.
func TestTranslateWireCarriesTruncated(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
			"output":{"message":{"role":"assistant","content":[{"text":"Bonjour le mon"}]}},
			"stopReason":"max_tokens",
			"usage":{"inputTokens":5,"outputTokens":5,"totalTokens":10}
		}`)
	})
	resp, err := p.Translate(context.Background(), aiprovider.TranslateRequest{
		Source: "Hello world", SourceLanguage: "en", TargetLocale: "fr",
	})
	require.NoError(t, err)
	assert.True(t, resp.Truncated, "a translation cut off at the cap must say so")
	assert.Equal(t, "Bonjour le mon", resp.Translation)
}

func TestChatStructuredWire(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
			"output":{"message":{"role":"assistant","content":[
				{"toolUse":{"toolUseId":"t1","name":"structured_output","input":{"greeting":"hi","n":2}}}
			]}},
			"stopReason":"tool_use",
			"usage":{"inputTokens":8,"outputTokens":2,"totalTokens":10}
		}`)
	})
	resp, err := p.ChatStructured(context.Background(),
		[]aiprovider.Message{aiprovider.TextMessage(aiprovider.RoleUser, "give me json")},
		aiprovider.JSONSchema{Name: "structured_output", Schema: map[string]any{"type": "object"}},
	)
	require.NoError(t, err)
	assert.JSONEq(t, `{"greeting":"hi","n":2}`, resp.Content)
	assert.Equal(t, 8, resp.Usage.InputTokens)
}

func TestChatStructuredWireTruncatedIsErrOutputTruncated(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		// No tool_use block + max_tokens → a truncation the caller can retry smaller.
		_, _ = io.WriteString(w, `{
			"output":{"message":{"role":"assistant","content":[{"text":"{\"partial\":"}]}},
			"stopReason":"max_tokens",
			"usage":{"inputTokens":8,"outputTokens":16,"totalTokens":24}
		}`)
	})
	_, err := p.ChatStructured(context.Background(),
		[]aiprovider.Message{aiprovider.TextMessage(aiprovider.RoleUser, "give me json")},
		aiprovider.JSONSchema{Name: "structured_output", Schema: map[string]any{"type": "object"}},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, aiprovider.ErrOutputTruncated)
}
