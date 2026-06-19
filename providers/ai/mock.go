package aiprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// MockProvider implements LLMProvider for testing.
type MockProvider struct {
	ProviderName             string
	TranslateFunc            func(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)
	ChatFunc                 func(ctx context.Context, messages []Message) (*ChatResponse, error)
	ChatStructuredFunc       func(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error)
	ChatStreamFunc           func(ctx context.Context, messages []Message, onEvent func(ChatStreamEvent)) (*ChatResponse, error)
	ChatStructuredStreamFunc func(ctx context.Context, messages []Message, schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error)
	InputModalitiesValue     []Modality
	TranslateCalls           []TranslateRequest
	ChatCalls                [][]Message
	ChatStructuredCalls      []struct {
		Messages []Message
		Schema   JSONSchema
	}
	mu sync.Mutex
}

// NewMockProvider creates a new mock provider with default behavior.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		ProviderName: "mock",
	}
}

func (p *MockProvider) Name() ProviderID { return ProviderID(p.ProviderName) }

// InputModalities reports the modalities the mock accepts. It accepts all of
// them so tests can exercise any media path; override via InputModalitiesValue.
func (p *MockProvider) InputModalities() []Modality {
	if p.InputModalitiesValue != nil {
		return p.InputModalitiesValue
	}
	return []Modality{ModalityImage, ModalityAudio, ModalityVideo}
}

func (p *MockProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	p.mu.Lock()
	p.TranslateCalls = append(p.TranslateCalls, req)
	p.mu.Unlock()
	if p.TranslateFunc != nil {
		return p.TranslateFunc(ctx, req)
	}
	// Default: prefix with target locale
	return &TranslateResponse{
		Translation: fmt.Sprintf("[%s] %s", req.TargetLocale, req.Source),
		Confidence:  0.95,
		Model:       "mock-model",
		Usage:       TokenUsage{InputTokens: 10, OutputTokens: 20},
	}, nil
}

func (p *MockProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	p.mu.Lock()
	p.ChatCalls = append(p.ChatCalls, messages)
	p.mu.Unlock()
	if p.ChatFunc != nil {
		return p.ChatFunc(ctx, messages)
	}
	// Default: echo the last user message
	lastMsg := ""
	for _, m := range messages {
		if m.Role == "user" {
			lastMsg = m.Text()
		}
	}
	return &ChatResponse{
		Content: "Mock response to: " + truncate(lastMsg, 50),
		Model:   "mock-model",
		Usage:   TokenUsage{InputTokens: 10, OutputTokens: 20},
	}, nil
}

func (p *MockProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	p.mu.Lock()
	p.ChatStructuredCalls = append(p.ChatStructuredCalls, struct {
		Messages []Message
		Schema   JSONSchema
	}{Messages: messages, Schema: schema})
	p.mu.Unlock()
	if p.ChatStructuredFunc != nil {
		return p.ChatStructuredFunc(ctx, messages, schema)
	}
	// Default: return an empty JSON object matching common patterns
	content := "{}"
	if schema.Schema != nil {
		if props, ok := schema.Schema["properties"].(map[string]any); ok {
			result := make(map[string]any)
			for key, prop := range props {
				if propMap, ok := prop.(map[string]any); ok {
					if propMap["type"] == "array" {
						result[key] = []any{}
					}
				}
			}
			if b, err := json.Marshal(result); err == nil {
				content = string(b)
			}
		}
	}
	return &ChatResponse{
		Content: content,
		Model:   "mock-model",
		Usage:   TokenUsage{InputTokens: 10, OutputTokens: 20},
	}, nil
}

// ChatStream implements StreamingLLMProvider by delegating to Chat and
// emitting the result as a single content event followed by done.
func (p *MockProvider) ChatStream(ctx context.Context, messages []Message, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	if p.ChatStreamFunc != nil {
		return p.ChatStreamFunc(ctx, messages, onEvent)
	}
	resp, err := p.Chat(ctx, messages)
	if err != nil {
		return nil, err
	}
	onEvent(ChatStreamEvent{Type: StreamEventContent, Content: resp.Content})
	onEvent(ChatStreamEvent{Type: StreamEventDone, Usage: resp.Usage, Model: resp.Model})
	return resp, nil
}

// ChatStructuredStream implements StreamingLLMProvider by delegating to ChatStructured.
func (p *MockProvider) ChatStructuredStream(ctx context.Context, messages []Message, schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	if p.ChatStructuredStreamFunc != nil {
		return p.ChatStructuredStreamFunc(ctx, messages, schema, onEvent)
	}
	resp, err := p.ChatStructured(ctx, messages, schema)
	if err != nil {
		return nil, err
	}
	onEvent(ChatStreamEvent{Type: StreamEventContent, Content: resp.Content})
	onEvent(ChatStreamEvent{Type: StreamEventDone, Usage: resp.Usage, Model: resp.Model})
	return resp, nil
}

func (p *MockProvider) Close() error { return nil }

// Compile-time check that MockProvider implements StreamingLLMProvider.
var _ StreamingLLMProvider = (*MockProvider)(nil)

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
