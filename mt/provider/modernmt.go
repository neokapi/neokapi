package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gokapi/gokapi/core/model"
)

// DefaultModernMTBaseURL is the default ModernMT API endpoint.
const DefaultModernMTBaseURL = "https://api.modernmt.com"

// ModernMTConfig holds configuration for the ModernMT provider.
type ModernMTConfig struct {
	APIKey  string
	Hints   []int64 // Optional memory IDs to bias translations
	BaseURL string  // Override for testing
}

// Validate checks configuration validity.
func (c *ModernMTConfig) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("modernmt: APIKey is required")
	}
	return nil
}

func (c *ModernMTConfig) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultModernMTBaseURL
}

// ModernMTProvider implements MTProvider using the ModernMT API.
type ModernMTProvider struct {
	cfg ModernMTConfig
}

// NewModernMTProvider creates a new ModernMT provider.
func NewModernMTProvider(cfg ModernMTConfig) *ModernMTProvider {
	return &ModernMTProvider{cfg: cfg}
}

func (p *ModernMTProvider) Name() string { return "modernmt" }

func (p *ModernMTProvider) Translate(_ context.Context, req TranslateRequest) (*TranslateResponse, error) {
	reqBody := modernMTRequest{
		Target: string(req.TargetLocale),
		Q:      req.Source,
	}
	if !req.SourceLocale.IsEmpty() {
		reqBody.Source = string(req.SourceLocale)
	}
	if len(p.cfg.Hints) > 0 {
		reqBody.Hints = p.cfg.Hints
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/translate", p.cfg.baseURL())
	httpReq, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("MMT-ApiKey", p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result modernMTResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Data.Translation == "" {
		return nil, fmt.Errorf("no translation returned")
	}

	return &TranslateResponse{
		Translation: result.Data.Translation,
	}, nil
}

func (p *ModernMTProvider) Close() error { return nil }

// ModernMTToolConfig holds configuration for the ModernMT tool (provider config + locale overrides).
type ModernMTToolConfig struct {
	ModernMTConfig
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
}

// ToolName returns the tool name this config applies to.
func (c *ModernMTToolConfig) ToolName() string { return "modernmt-translate" }

// Reset restores default values.
func (c *ModernMTToolConfig) Reset() {
	c.APIKey = ""
	c.SourceLocale = ""
	c.TargetLocale = ""
	c.Hints = nil
	c.BaseURL = ""
}

// Validate checks configuration validity.
func (c *ModernMTToolConfig) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("modernmt: APIKey is required")
	}
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("modernmt: TargetLocale is required")
	}
	return nil
}

type modernMTRequest struct {
	Source string  `json:"source,omitempty"`
	Target string  `json:"target"`
	Q      string  `json:"q"`
	Hints  []int64 `json:"hints,omitempty"`
}

type modernMTResponse struct {
	Status int `json:"status"`
	Data   struct {
		Translation string `json:"translation"`
	} `json:"data"`
}
