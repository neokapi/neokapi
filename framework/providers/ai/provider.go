package provider

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// LLMProvider defines the interface for LLM service providers.
type LLMProvider interface {
	// Name returns the provider identifier (e.g., "anthropic", "openai").
	Name() string

	// Translate translates text using the LLM.
	Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)

	// Chat sends a general chat message and returns the response.
	Chat(ctx context.Context, messages []Message) (*ChatResponse, error)

	// ChatStructured sends a chat message and constrains the response to match
	// the given JSON schema. The response Content will be valid JSON conforming
	// to the schema. This uses provider-specific mechanisms: OpenAI/Azure use
	// response_format with json_schema, Anthropic uses tool use, Ollama uses
	// the format field.
	ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error)

	// Close releases provider resources.
	Close() error
}

// JSONSchema defines a JSON schema for structured LLM output.
type JSONSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema"`
	Strict      bool           `json:"strict,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"` // Message text
}

// TranslateRequest contains parameters for a translation request.
type TranslateRequest struct {
	Source       string            `json:"source"`
	SourceLanguage model.LocaleID  `json:"source_language"`
	TargetLocale model.LocaleID    `json:"target_locale"`
	Context      string            `json:"context,omitempty"`
	Glossary     map[string]string `json:"glossary,omitempty"`
	Format       string            `json:"format,omitempty"` // e.g., "html", "plain"
}

// TranslateResponse contains the translation result.
type TranslateResponse struct {
	Translation string  `json:"translation"`
	Confidence  float64 `json:"confidence"`
	Model       string  `json:"model"`
}

// ChatResponse contains the chat result.
type ChatResponse struct {
	Content string `json:"content"`
	Model   string `json:"model"`
}

// QAIssue represents a quality assurance issue found in a translation.
type QAIssue struct {
	Type        string `json:"type"`     // "terminology", "fluency", "accuracy", "consistency"
	Severity    string `json:"severity"` // "error", "warning", "info"
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// Config holds common provider configuration.
type Config struct {
	APIKey      string  `json:"api_key"`
	BaseURL     string  `json:"base_url,omitempty"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// Validate checks that the config has required fields.
func (c *Config) Validate() error {
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}
