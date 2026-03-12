package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// MockProvider implements LLMProvider for testing.
type MockProvider struct {
	ProviderName        string
	TranslateFunc       func(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)
	ChatFunc            func(ctx context.Context, messages []Message) (*ChatResponse, error)
	ChatStructuredFunc  func(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error)
	TranslateCalls      []TranslateRequest
	ChatCalls           [][]Message
	ChatStructuredCalls []struct {
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

func (p *MockProvider) Name() string { return p.ProviderName }

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
			lastMsg = m.Content
		}
	}
	return &ChatResponse{
		Content: "Mock response to: " + truncate(lastMsg, 50),
		Model:   "mock-model",
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
	}, nil
}

func (p *MockProvider) Close() error { return nil }

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
