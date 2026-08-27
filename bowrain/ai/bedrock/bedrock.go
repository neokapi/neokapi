// Package bedrock implements an aiprovider.LLMProvider backed by AWS Bedrock's
// Converse API. It lives in the bowrain module — not the framework's
// providers/ai — so the aws-sdk-go-v2 dependency never reaches the kapi CLI or
// Kapi Desktop; every framework AI provider is hand-rolled HTTP for exactly that
// reason. bowrain-server and bowrain-worker blank-import this package to register
// the "bedrock" provider in the aiprovider registry, and the platform selects it
// with BOWRAIN_PLATFORM_PROVIDER=bedrock.
//
// Auth is the ambient AWS credential chain — the ECS task role in production —
// so there is no API key. On-demand invocation requires a cross-region INFERENCE
// PROFILE id for the region (e.g. "eu.anthropic.claude-sonnet-4-6"): Bedrock
// rejects a bare foundation-model id with an on-demand-throughput error, so the
// configured model must be a profile id. Because Converse is model-agnostic, the
// same code serves any Bedrock text model; the Anthropic profiles are the
// platform default only.
package bedrock

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brdoc "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// ProviderID is the aiprovider registry name; select it with
// BOWRAIN_PLATFORM_PROVIDER=bedrock.
const ProviderID = aiprovider.ProviderID("bedrock")

// DefaultModel is the cross-region inference profile used when no model is
// configured. Claude Sonnet 4.6 is a strong default for translation; override
// with BOWRAIN_PLATFORM_MODEL (the value must be an inference-profile id, not a
// bare foundation-model id).
const DefaultModel = "eu.anthropic.claude-sonnet-4-6"

func init() {
	aiprovider.RegisterProvider(
		aiprovider.ProviderInfo{
			Name:         ProviderID,
			Label:        "AWS Bedrock",
			DefaultModel: DefaultModel,
			// No ModelPrefixes: Bedrock ids look like "eu.anthropic.claude-*", but
			// "claude"/"anthropic" belong to the direct Anthropic API provider.
			// Bedrock is chosen explicitly, like Azure OpenAI.
		},
		func(cfg aiprovider.Config) aiprovider.LLMProvider { return New(cfg) },
	)
}

// Provider is an aiprovider.LLMProvider (and StreamingLLMProvider) backed by
// Bedrock Converse.
type Provider struct {
	cfg aiprovider.Config

	once    sync.Once
	client  *bedrockruntime.Client
	initErr error
}

// Compile-time checks: Provider satisfies both the base and streaming provider
// interfaces, and declares its model limits so the batch packer can size calls.
var (
	_ aiprovider.LLMProvider          = (*Provider)(nil)
	_ aiprovider.StreamingLLMProvider = (*Provider)(nil)
	_ aiprovider.LimitedProvider      = (*Provider)(nil)
)

// New constructs a Bedrock provider. The AWS SDK client is built lazily on the
// first request, so merely constructing the provider (e.g. to read its metadata)
// never touches the network or the credential chain — and a config-load failure
// surfaces as a request error rather than a panic in the factory, which cannot
// return one.
func New(cfg aiprovider.Config) *Provider {
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	return &Provider{cfg: cfg}
}

// brClient lazily builds the Bedrock client, resolving region and credentials
// from the ambient environment (AWS_REGION + the task role in production).
func (p *Provider) brClient(ctx context.Context) (*bedrockruntime.Client, error) {
	p.once.Do(func() {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
		if err != nil {
			p.initErr = fmt.Errorf("bedrock: load AWS config: %w", err)
			return
		}
		var opts []func(*bedrockruntime.Options)
		// cfg.BaseURL, when set, is an endpoint override (a VPC endpoint or a local
		// test double) — never a region, which comes from AWS_REGION.
		if p.cfg.BaseURL != "" {
			endpoint := p.cfg.BaseURL
			opts = append(opts, func(o *bedrockruntime.Options) { o.BaseEndpoint = aws.String(endpoint) })
		}
		p.client = bedrockruntime.NewFromConfig(awsCfg, opts...)
	})
	return p.client, p.initErr
}

func (p *Provider) Name() aiprovider.ProviderID { return ProviderID }

func (p *Provider) InputModalities() []aiprovider.Modality {
	return []aiprovider.Modality{aiprovider.ModalityImage}
}

func (p *Provider) Close() error { return nil }

// Limits reports what the configured model can emit, so the batch packer sizes
// each call to fit. The model id is a Bedrock inference profile
// ("eu.anthropic.claude-sonnet-4-6"); baseModelID strips the region+vendor
// prefixes to the bare model name the framework's limit table knows. An unknown
// model falls back to a conservative ceiling, yielding smaller batches rather
// than truncated replies.
func (p *Provider) Limits() aiprovider.Limits {
	if l, ok := aiprovider.LimitsForModel(baseModelID(p.cfg.Model)); ok {
		return l
	}
	return aiprovider.Limits{MaxOutputTokens: aiprovider.ConservativeMaxOutputTokens}
}

func (p *Provider) Translate(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	// Mirrors the framework's standardTranslate: render the shared prompt and run
	// it through this provider's own Chat. (standardTranslate is unexported, so
	// the glue is reproduced here; the prompt bytes are identical.)
	msgs := aiprovider.MessagesFromTurns(req.PromptTurns())
	resp, err := p.Chat(ctx, msgs)
	if err != nil {
		return nil, err
	}
	return &aiprovider.TranslateResponse{
		Translation: resp.Content,
		Confidence:  0.85,
		Model:       resp.Model,
		Usage:       resp.Usage,
		Truncated:   resp.Truncated,
	}, nil
}

func (p *Provider) Chat(ctx context.Context, messages []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	client, err := p.brClient(ctx)
	if err != nil {
		return nil, err
	}
	in, err := p.converseInput(messages)
	if err != nil {
		return nil, err
	}
	out, err := client.Converse(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("bedrock: converse: %w", err)
	}
	return &aiprovider.ChatResponse{
		Content:   outputText(out.Output),
		Model:     p.cfg.Model,
		Usage:     toTokenUsage(out.Usage),
		Truncated: out.StopReason == brtypes.StopReasonMaxTokens,
	}, nil
}

func (p *Provider) ChatStructured(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	client, err := p.brClient(ctx)
	if err != nil {
		return nil, err
	}
	in, err := p.converseInput(messages)
	if err != nil {
		return nil, err
	}
	toolName := toolNameOf(schema)
	in.ToolConfig = toolConfig(schema, toolName)

	out, err := client.Converse(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("bedrock: converse (structured): %w", err)
	}
	if input, ok := toolUseInput(out.Output, toolName); ok {
		return &aiprovider.ChatResponse{
			Content:   input,
			Model:     p.cfg.Model,
			Usage:     toTokenUsage(out.Usage),
			Truncated: out.StopReason == brtypes.StopReasonMaxTokens,
		}, nil
	}
	// No tool_use block: either the model hit the output cap mid-reply, or it
	// declined the tool. Distinguish the two — the caller can shrink a truncated
	// batch, but a malformed reply is a model problem.
	if out.StopReason == brtypes.StopReasonMaxTokens {
		return nil, fmt.Errorf("bedrock: %w (max_tokens=%d)", aiprovider.ErrOutputTruncated, p.maxTokens())
	}
	return nil, errors.New("bedrock: no tool_use block in structured response")
}

// ChatStream streams a plain-text chat response.
func (p *Provider) ChatStream(ctx context.Context, messages []aiprovider.Message, onEvent func(aiprovider.ChatStreamEvent)) (*aiprovider.ChatResponse, error) {
	client, err := p.brClient(ctx)
	if err != nil {
		return nil, err
	}
	in, err := p.converseInput(messages)
	if err != nil {
		return nil, err
	}
	out, err := client.ConverseStream(ctx, streamInput(in))
	if err != nil {
		return nil, fmt.Errorf("bedrock: converse stream: %w", err)
	}
	return p.consumeStream(out.GetStream(), onEvent, false)
}

// ChatStructuredStream is ChatStructured with streaming progress.
func (p *Provider) ChatStructuredStream(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema, onEvent func(aiprovider.ChatStreamEvent)) (*aiprovider.ChatResponse, error) {
	client, err := p.brClient(ctx)
	if err != nil {
		return nil, err
	}
	in, err := p.converseInput(messages)
	if err != nil {
		return nil, err
	}
	in.ToolConfig = toolConfig(schema, toolNameOf(schema))

	out, err := client.ConverseStream(ctx, streamInput(in))
	if err != nil {
		return nil, fmt.Errorf("bedrock: converse stream (structured): %w", err)
	}
	return p.consumeStream(out.GetStream(), onEvent, true)
}

// converseInput assembles the shared request: system-role messages lifted into
// the top-level System field (Converse accepts only user/assistant roles),
// max_tokens from the model ceiling, and temperature when set.
func (p *Provider) converseInput(messages []aiprovider.Message) (*bedrockruntime.ConverseInput, error) {
	system, convo := aiprovider.SplitSystem(messages)
	msgs, err := toBedrockMessages(convo)
	if err != nil {
		return nil, err
	}
	in := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(p.cfg.Model),
		Messages: msgs,
		InferenceConfig: &brtypes.InferenceConfiguration{
			MaxTokens: aws.Int32(int32(p.maxTokens())),
		},
	}
	if system != "" {
		in.System = []brtypes.SystemContentBlock{
			&brtypes.SystemContentBlockMemberText{Value: system},
		}
	}
	// Not `> 0`: that spelling made temperature 0, the one an eval wants, the
	// only value you could not ask for.
	if p.cfg.Temperature != nil {
		in.InferenceConfig.Temperature = aws.Float32(float32(*p.cfg.Temperature))
	}
	return in, nil
}

// maxTokens resolves this call's output budget. The batch packer already sizes a
// batch to fit EffectiveMaxOutputTokens and passes its own per-request budget via
// the context — which only aiprovider-internal code can read — so requesting the
// model ceiling is a safe upper bound that never truncates a well-packed batch.
// Bedrock bills on tokens actually produced, so over-requesting max_tokens costs
// nothing. An explicit cfg.MaxTokens (clamped to the ceiling) still wins.
func (p *Provider) maxTokens() int {
	ceiling := p.Limits().EffectiveMaxOutputTokens()
	if want := p.cfg.MaxTokens; want > 0 && want < ceiling {
		return want
	}
	return ceiling
}

// consumeStream drains a Converse event stream, emitting content chunks through
// onEvent and accumulating the final response. When structured is true it
// collects the tool-use input JSON fragments instead of plain text.
func (p *Provider) consumeStream(stream *bedrockruntime.ConverseStreamEventStream, onEvent func(aiprovider.ChatStreamEvent), structured bool) (*aiprovider.ChatResponse, error) {
	defer stream.Close()

	var (
		content    strings.Builder
		toolInput  strings.Builder
		usage      aiprovider.TokenUsage
		stopReason brtypes.StopReason
	)
	for event := range stream.Events() {
		switch e := event.(type) {
		case *brtypes.ConverseStreamOutputMemberContentBlockDelta:
			switch d := e.Value.Delta.(type) {
			case *brtypes.ContentBlockDeltaMemberText:
				content.WriteString(d.Value)
				if onEvent != nil {
					onEvent(aiprovider.ChatStreamEvent{Type: aiprovider.StreamEventContent, Content: d.Value})
				}
			case *brtypes.ContentBlockDeltaMemberToolUse:
				if d.Value.Input != nil {
					toolInput.WriteString(*d.Value.Input)
				}
			}
		case *brtypes.ConverseStreamOutputMemberMetadata:
			if e.Value.Usage != nil {
				usage = toTokenUsage(e.Value.Usage)
			}
		case *brtypes.ConverseStreamOutputMemberMessageStop:
			stopReason = e.Value.StopReason
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("bedrock: stream: %w", err)
	}

	truncated := stopReason == brtypes.StopReasonMaxTokens
	result := content.String()
	if structured {
		result = toolInput.String()
	}
	if onEvent != nil {
		onEvent(aiprovider.ChatStreamEvent{Type: aiprovider.StreamEventDone, Usage: usage, Model: p.cfg.Model})
	}
	if structured && result == "" && truncated {
		return nil, fmt.Errorf("bedrock: %w (max_tokens=%d)", aiprovider.ErrOutputTruncated, p.maxTokens())
	}
	return &aiprovider.ChatResponse{
		Content:   result,
		Model:     p.cfg.Model,
		Usage:     usage,
		Truncated: truncated,
	}, nil
}

// --- helpers ------------------------------------------------------------------

func toolNameOf(schema aiprovider.JSONSchema) string {
	if schema.Name != "" {
		return schema.Name
	}
	return "structured_output"
}

// toolConfig builds a forced single-tool configuration whose input schema is the
// caller's JSON schema — the Converse mechanism for constrained/structured
// output, mirroring the Anthropic provider's tool-use approach.
func toolConfig(schema aiprovider.JSONSchema, toolName string) *brtypes.ToolConfiguration {
	spec := brtypes.ToolSpecification{
		Name:        aws.String(toolName),
		InputSchema: &brtypes.ToolInputSchemaMemberJson{Value: brdoc.NewLazyDocument(schema.Schema)},
	}
	if schema.Description != "" {
		spec.Description = aws.String(schema.Description)
	}
	return &brtypes.ToolConfiguration{
		Tools:      []brtypes.Tool{&brtypes.ToolMemberToolSpec{Value: spec}},
		ToolChoice: &brtypes.ToolChoiceMemberTool{Value: brtypes.SpecificToolChoice{Name: aws.String(toolName)}},
	}
}

// streamInput mirrors a ConverseInput into the identically-shaped
// ConverseStreamInput (the two ops take distinct but field-compatible structs).
func streamInput(in *bedrockruntime.ConverseInput) *bedrockruntime.ConverseStreamInput {
	return &bedrockruntime.ConverseStreamInput{
		ModelId:         in.ModelId,
		Messages:        in.Messages,
		System:          in.System,
		InferenceConfig: in.InferenceConfig,
		ToolConfig:      in.ToolConfig,
	}
}

// outputText concatenates the text content blocks of a Converse output message.
func outputText(out brtypes.ConverseOutput) string {
	msg, ok := out.(*brtypes.ConverseOutputMemberMessage)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, block := range msg.Value.Content {
		if t, ok := block.(*brtypes.ContentBlockMemberText); ok {
			b.WriteString(t.Value)
		}
	}
	return b.String()
}

// toolUseInput returns the named tool-use block's input as JSON bytes. The
// deserialized Input document marshals straight back to the response JSON.
func toolUseInput(out brtypes.ConverseOutput, toolName string) (string, bool) {
	msg, ok := out.(*brtypes.ConverseOutputMemberMessage)
	if !ok {
		return "", false
	}
	for _, block := range msg.Value.Content {
		tu, ok := block.(*brtypes.ContentBlockMemberToolUse)
		if !ok || aws.ToString(tu.Value.Name) != toolName || tu.Value.Input == nil {
			continue
		}
		raw, err := tu.Value.Input.MarshalSmithyDocument()
		if err != nil {
			return "", false
		}
		return string(raw), true
	}
	return "", false
}

func toTokenUsage(u *brtypes.TokenUsage) aiprovider.TokenUsage {
	if u == nil {
		return aiprovider.TokenUsage{}
	}
	return aiprovider.TokenUsage{
		InputTokens:         int(aws.ToInt32(u.InputTokens)),
		OutputTokens:        int(aws.ToInt32(u.OutputTokens)),
		CacheCreationTokens: int(aws.ToInt32(u.CacheWriteInputTokens)),
		CacheReadTokens:     int(aws.ToInt32(u.CacheReadInputTokens)),
	}
}

func toBedrockMessages(messages []aiprovider.Message) ([]brtypes.Message, error) {
	out := make([]brtypes.Message, 0, len(messages))
	for _, m := range messages {
		role, err := toRole(m.Role)
		if err != nil {
			return nil, err
		}
		blocks := make([]brtypes.ContentBlock, 0, len(m.Parts))
		for _, part := range m.Parts {
			switch part.Kind {
			case aiprovider.ContentText:
				blocks = append(blocks, &brtypes.ContentBlockMemberText{Value: part.Text})
			case aiprovider.ContentImage:
				img, err := toImageBlock(part.Media)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, img)
			default:
				return nil, fmt.Errorf("bedrock: unsupported content kind %q (provider accepts text, image)", part.Kind)
			}
		}
		out = append(out, brtypes.Message{Role: role, Content: blocks})
	}
	return out, nil
}

func toRole(role string) (brtypes.ConversationRole, error) {
	switch role {
	case aiprovider.RoleUser:
		return brtypes.ConversationRoleUser, nil
	case aiprovider.RoleAssistant:
		return brtypes.ConversationRoleAssistant, nil
	default:
		// System is lifted out by SplitSystem before we get here; any other role is
		// a caller error.
		return "", fmt.Errorf("bedrock: unsupported message role %q", role)
	}
}

func toImageBlock(m *model.Media) (*brtypes.ContentBlockMemberImage, error) {
	data, mime, err := mediaBytes(m)
	if err != nil {
		return nil, err
	}
	format, err := imageFormat(mime)
	if err != nil {
		return nil, err
	}
	return &brtypes.ContentBlockMemberImage{
		Value: brtypes.ImageBlock{
			Format: format,
			Source: &brtypes.ImageSourceMemberBytes{Value: data},
		},
	}, nil
}

func imageFormat(mime string) (brtypes.ImageFormat, error) {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		return brtypes.ImageFormatPng, nil
	case "image/jpeg", "image/jpg":
		return brtypes.ImageFormatJpeg, nil
	case "image/gif":
		return brtypes.ImageFormatGif, nil
	case "image/webp":
		return brtypes.ImageFormatWebp, nil
	default:
		return "", fmt.Errorf("bedrock: unsupported image MIME %q (png, jpeg, gif, webp)", mime)
	}
}

// mediaBytes materializes an image slice to raw bytes + MIME type at the provider
// boundary. It mirrors aiprovider's own resolveMediaBytes (which is unexported):
// inline Data, or a local-file URI. A blob-store-backed or remote slice must be
// materialized by the caller — the MediaSlicer produces inline-Data slices — so
// those return an error rather than coupling this provider to a blob store.
func mediaBytes(m *model.Media) ([]byte, string, error) {
	if m == nil {
		return nil, "", errors.New("bedrock: nil media part")
	}
	switch {
	case len(m.Data) > 0:
		return m.Data, m.MimeType, nil
	case isLocalPath(m.URI):
		b, err := os.ReadFile(m.URI)
		if err != nil {
			return nil, "", fmt.Errorf("bedrock: read media %q: %w", m.URI, err)
		}
		return b, m.MimeType, nil
	case m.BlobKey != "":
		return nil, "", fmt.Errorf("bedrock: media is blob-store-backed (%q); materialize it before the provider call", m.BlobKey)
	default:
		return nil, "", errors.New("bedrock: media has no inline data or local URI to send")
	}
}

// isLocalPath reports whether s is a filesystem path (not an http(s)/data URL).
func isLocalPath(s string) bool {
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return true // not a URL — treat as a path
	}
	return u.Scheme == "" || u.Scheme == "file"
}

// baseModelID strips a Bedrock cross-region inference-profile prefix ("eu.",
// "us.", "apac.", "us-gov.") and the vendor segment ("anthropic.") from a model
// id, yielding the bare model id the framework's limit table understands
// (e.g. "eu.anthropic.claude-sonnet-4-6" -> "claude-sonnet-4-6").
func baseModelID(id string) string {
	s := id
	for _, pfx := range []string{"us-gov.", "eu.", "us.", "apac."} {
		if strings.HasPrefix(s, pfx) {
			s = s[len(pfx):]
			break
		}
	}
	// Drop a leading vendor segment (anthropic., meta., amazon., mistral., ...).
	// Only the first dot-delimited segment is a vendor; model ids like
	// "claude-3-5-sonnet-20240620-v1:0" keep the rest intact.
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[i+1:]
	}
	return s
}
