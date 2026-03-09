package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MockProvider implements LLMProvider for testing.
type MockProvider struct {
	ProviderName   string
	TranslateFunc  func(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)
	ChatFunc       func(ctx context.Context, messages []Message) (*ChatResponse, error)
	TranslateCalls []TranslateRequest
	ChatCalls      [][]Message
	mu             sync.Mutex
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

func (p *MockProvider) Close() error { return nil }

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
