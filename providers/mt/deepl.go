package mtprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/neokapi/neokapi/core/httputil"
	"github.com/neokapi/neokapi/core/model"
)

// DefaultDeepLBaseURL is the default DeepL free API endpoint.
const DefaultDeepLBaseURL = "https://api-free.deepl.com"

// Formality represents a DeepL formality level.
type Formality string

// DeepL formality levels.
const (
	FormalityDefault    Formality = "default"
	FormalityMore       Formality = "more"
	FormalityLess       Formality = "less"
	FormalityPreferMore Formality = "prefer_more"
	FormalityPreferLess Formality = "prefer_less"
)

// DeepLConfig holds configuration for the DeepL provider.
type DeepLConfig struct {
	APIKey    string    `schema:"description=DeepL API authentication key,widget=password"`
	Formality Formality `schema:"description=Formality level for translations,enum=default|more|less|prefer_more|prefer_less,default=default"`
	BaseURL   string    `schema:"description=API base URL override for testing"` // Override for testing
}

// Validate checks configuration validity.
func (c *DeepLConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("deepl: APIKey is required")
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

func (p *DeepLProvider) Name() ProviderID { return DeepL }

func (p *DeepLProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	form := url.Values{}
	form.Set("text", req.Source)
	form.Set("target_lang", strings.ToUpper(string(req.TargetLocale)))
	if !req.SourceLocale.IsEmpty() {
		form.Set("source_lang", strings.ToUpper(string(req.SourceLocale)))
	}
	if p.cfg.Formality != "" && p.cfg.Formality != FormalityDefault {
		form.Set("formality", string(p.cfg.Formality))
	}

	apiURL := p.cfg.baseURL() + "/v2/translate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "DeepL-Auth-Key "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	respBody, err := sendAndRead(p.client, httpReq)
	if err != nil {
		return nil, err
	}

	var result deepLResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Translations) == 0 {
		return nil, errors.New("no translations returned")
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
		return errors.New("deepl: APIKey is required")
	}
	if c.TargetLocale.IsEmpty() {
		return errors.New("deepl: TargetLocale is required")
	}
	return nil
}

type deepLResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
}
