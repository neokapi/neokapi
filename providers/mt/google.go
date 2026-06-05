package mtprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/neokapi/neokapi/core/httputil"
	"github.com/neokapi/neokapi/core/model"
)

// DefaultGoogleBaseURL is the default Google Translate API endpoint.
const DefaultGoogleBaseURL = "https://translation.googleapis.com"

// GoogleConfig holds configuration for the Google Translate provider.
type GoogleConfig struct {
	APIKey    string `schema:"description=Google Cloud API key,widget=password"`
	ProjectID string `schema:"description=Google Cloud project ID"`
	BaseURL   string `schema:"description=API base URL override for testing"` // Override for testing
}

// Validate checks configuration validity.
func (c *GoogleConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("google: APIKey is required")
	}
	return nil
}

func (c *GoogleConfig) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultGoogleBaseURL
}

// GoogleProvider implements MTProvider using the Google Cloud Translation API v2.
type GoogleProvider struct {
	cfg    GoogleConfig
	client *http.Client
}

// NewGoogleProvider creates a new Google Translate MT provider.
func NewGoogleProvider(cfg GoogleConfig) *GoogleProvider {
	return &GoogleProvider{cfg: cfg, client: httputil.NewResilientClient()}
}

func (p *GoogleProvider) Name() ProviderID { return Google }

func (p *GoogleProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	reqBody := googleTranslateRequest{
		Q:      req.Source,
		Target: string(req.TargetLocale),
		Format: "text",
	}
	if !req.SourceLocale.IsEmpty() {
		reqBody.Source = string(req.SourceLocale)
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Send the API key via the X-Goog-Api-Key header rather than the ?key=
	// query parameter. Keeping the key out of the URL prevents it from leaking
	// into wrapped transport errors (*url.Error includes the full URL) and from
	// being recorded by proxies, access logs, and APM tooling.
	apiURL := p.cfg.baseURL() + "/language/translate/v2"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Goog-Api-Key", p.cfg.APIKey)

	respBody, err := sendAndRead(p.client, httpReq)
	if err != nil {
		return nil, err
	}

	var result googleTranslateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Data.Translations) == 0 {
		return nil, errors.New("no translations returned")
	}

	return &TranslateResponse{
		Translation: result.Data.Translations[0].TranslatedText,
	}, nil
}

func (p *GoogleProvider) Close() error { return nil }

// GoogleToolConfig holds configuration for the Google MT tool (provider config + locale overrides).
type GoogleToolConfig struct {
	GoogleConfig
	SourceLocale model.LocaleID `schema:"description=Source locale of the content"`
	TargetLocale model.LocaleID `schema:"description=Target locale for processing"`
}

// ToolName returns the tool name this config applies to.
func (c *GoogleToolConfig) ToolName() string { return "google-translate" }

// Reset restores default values.
func (c *GoogleToolConfig) Reset() {
	c.APIKey = ""
	c.ProjectID = ""
	c.SourceLocale = ""
	c.TargetLocale = ""
	c.BaseURL = ""
}

// Validate checks configuration validity.
func (c *GoogleToolConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("google: APIKey is required")
	}
	if c.TargetLocale.IsEmpty() {
		return errors.New("google: TargetLocale is required")
	}
	return nil
}

type googleTranslateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source,omitempty"`
	Target string `json:"target"`
	Format string `json:"format"`
}

type googleTranslateResponse struct {
	Data struct {
		Translations []struct {
			TranslatedText string `json:"translatedText"`
		} `json:"translations"`
	} `json:"data"`
}
