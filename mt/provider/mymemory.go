package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/gokapi/gokapi/model"
)

// DefaultMyMemoryBaseURL is the default MyMemory API endpoint.
const DefaultMyMemoryBaseURL = "https://api.mymemory.translated.net"

// MyMemoryConfig holds configuration for the MyMemory provider.
type MyMemoryConfig struct {
	Email   string // Optional; provides higher rate limits when set
	BaseURL string // Override for testing
}

// Validate checks configuration validity.
func (c *MyMemoryConfig) Validate() error {
	return nil
}

func (c *MyMemoryConfig) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultMyMemoryBaseURL
}

// MyMemoryProvider implements MTProvider using the MyMemory free translation API.
type MyMemoryProvider struct {
	cfg MyMemoryConfig
}

// NewMyMemoryProvider creates a new MyMemory MT provider.
func NewMyMemoryProvider(cfg MyMemoryConfig) *MyMemoryProvider {
	return &MyMemoryProvider{cfg: cfg}
}

func (p *MyMemoryProvider) Name() string { return "mymemory" }

func (p *MyMemoryProvider) Translate(_ context.Context, req TranslateRequest) (*TranslateResponse, error) {
	if req.SourceLocale.IsEmpty() {
		return nil, fmt.Errorf("mymemory: SourceLocale is required")
	}
	if req.TargetLocale.IsEmpty() {
		return nil, fmt.Errorf("mymemory: TargetLocale is required")
	}

	langPair := fmt.Sprintf("%s|%s", string(req.SourceLocale), string(req.TargetLocale))

	params := url.Values{}
	params.Set("q", req.Source)
	params.Set("langpair", langPair)
	if p.cfg.Email != "" {
		params.Set("de", p.cfg.Email)
	}

	apiURL := fmt.Sprintf("%s/get?%s", p.cfg.baseURL(), params.Encode())
	resp, err := http.Get(apiURL)
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

	var result myMemoryResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.ResponseStatus != 200 {
		return nil, fmt.Errorf("API response status %d", result.ResponseStatus)
	}

	return &TranslateResponse{
		Translation: result.ResponseData.TranslatedText,
	}, nil
}

func (p *MyMemoryProvider) Close() error { return nil }

// MyMemoryToolConfig holds configuration for the MyMemory MT tool (provider config + locale overrides).
type MyMemoryToolConfig struct {
	MyMemoryConfig
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
}

// ToolName returns the tool name this config applies to.
func (c *MyMemoryToolConfig) ToolName() string { return "mymemory-translate" }

// Reset restores default values.
func (c *MyMemoryToolConfig) Reset() {
	c.Email = ""
	c.SourceLocale = ""
	c.TargetLocale = ""
	c.BaseURL = ""
}

// Validate checks configuration validity.
func (c *MyMemoryToolConfig) Validate() error {
	if c.SourceLocale.IsEmpty() {
		return fmt.Errorf("mymemory: SourceLocale is required")
	}
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("mymemory: TargetLocale is required")
	}
	return nil
}

type myMemoryResponse struct {
	ResponseData struct {
		TranslatedText string  `json:"translatedText"`
		Match          float64 `json:"match"`
	} `json:"responseData"`
	ResponseStatus int `json:"responseStatus"`
}
