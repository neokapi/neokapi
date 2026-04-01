package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/neokapi/neokapi/core/httputil"
	"github.com/neokapi/neokapi/core/model"
)

// DefaultDeepLBaseURL is the default DeepL free API endpoint.
const DefaultDeepLBaseURL = "https://api-free.deepl.com"

// DeepL formality levels.
const (
	FormalityDefault    = "default"
	FormalityMore       = "more"
	FormalityLess       = "less"
	FormalityPreferMore = "prefer_more"
	FormalityPreferLess = "prefer_less"
)

// DeepLConfig holds configuration for the DeepL provider.
type DeepLConfig struct {
	APIKey    string `schema:"description=DeepL API authentication key,widget=password"`
	Formality string `schema:"description=Formality level for translations,enum=default|more|less|prefer_more|prefer_less,default=default"` // "default", "more", "less", "prefer_more", "prefer_less"
	BaseURL   string `schema:"description=API base URL override for testing"` // Override for testing
}

// Validate checks configuration validity.
func (c *DeepLConfig) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("deepl: APIKey is required")
	}
	return nil
}

func (c *DeepLConfig) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultDeepLBaseURL
}

// DeepLProvider implements MTProvider using the DeepL API.
type DeepLProvider struct {
	cfg    DeepLConfig
	client *http.Client
}

// NewDeepLProvider creates a new DeepL MT provider.
func NewDeepLProvider(cfg DeepLConfig) *DeepLProvider {
	return &DeepLProvider{cfg: cfg, client: httputil.NewResilientClient()}
}

func (p *DeepLProvider) Name() string { return "deepl" }

func (p *DeepLProvider) Translate(_ context.Context, req TranslateRequest) (*TranslateResponse, error) {
	form := url.Values{}
	form.Set("text", req.Source)
	form.Set("target_lang", strings.ToUpper(string(req.TargetLocale)))
	if !req.SourceLocale.IsEmpty() {
		form.Set("source_lang", strings.ToUpper(string(req.SourceLocale)))
	}
	if p.cfg.Formality != "" && p.cfg.Formality != FormalityDefault {
		form.Set("formality", p.cfg.Formality)
	}

	apiURL := fmt.Sprintf("%s/v2/translate", p.cfg.baseURL())
	httpReq, err := http.NewRequest(http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "DeepL-Auth-Key "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(httpReq)
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

	var result deepLResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Translations) == 0 {
		return nil, fmt.Errorf("no translations returned")
	}

	return &TranslateResponse{
		Translation: result.Translations[0].Text,
	}, nil
}

func (p *DeepLProvider) Close() error { return nil }

// DeepLToolConfig holds configuration for the DeepL MT tool (provider config + locale overrides).
type DeepLToolConfig struct {
	DeepLConfig
	SourceLocale model.LocaleID `schema:"description=Source locale of the content"`
	TargetLocale model.LocaleID `schema:"description=Target locale for processing"`
}

// ToolName returns the tool name this config applies to.
func (c *DeepLToolConfig) ToolName() string { return "deepl-translate" }

// Reset restores default values.
func (c *DeepLToolConfig) Reset() {
	c.APIKey = ""
	c.Formality = FormalityDefault
	c.SourceLocale = ""
	c.TargetLocale = ""
	c.BaseURL = ""
}

// Validate checks configuration validity.
func (c *DeepLToolConfig) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("deepl: APIKey is required")
	}
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("deepl: TargetLocale is required")
	}
	return nil
}

type deepLResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
}
