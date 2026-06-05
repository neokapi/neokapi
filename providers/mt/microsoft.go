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

// DefaultMicrosoftBaseURL is the default Azure Translator API endpoint.
const DefaultMicrosoftBaseURL = "https://api.cognitive.microsofttranslator.com"

// MicrosoftConfig holds configuration for the Microsoft Translator provider.
type MicrosoftConfig struct {
	SubscriptionKey string `schema:"description=Azure Cognitive Services subscription key,widget=password"`
	Region          string `schema:"description=Azure region for the Translator resource (e.g. westeurope)"`
	BaseURL         string `schema:"description=API base URL override for testing"` // Override for testing
}

// Validate checks configuration validity.
func (c *MicrosoftConfig) Validate() error {
	if c.SubscriptionKey == "" {
		return errors.New("microsoft: SubscriptionKey is required")
	}
	return nil
}

func (c *MicrosoftConfig) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultMicrosoftBaseURL
}

// MicrosoftProvider implements MTProvider using the Azure Cognitive Services Translator API.
type MicrosoftProvider struct {
	cfg    MicrosoftConfig
	client *http.Client
}

// NewMicrosoftProvider creates a new Microsoft Translator MT provider.
func NewMicrosoftProvider(cfg MicrosoftConfig) *MicrosoftProvider {
	return &MicrosoftProvider{cfg: cfg, client: httputil.NewResilientClient()}
}

func (p *MicrosoftProvider) Name() ProviderID { return MSFT }

func (p *MicrosoftProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	body := []msRequestBody{{Text: req.Source}}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/translate?api-version=3.0&to=%s", p.cfg.baseURL(), string(req.TargetLocale))
	if !req.SourceLocale.IsEmpty() {
		apiURL += "&from=" + string(req.SourceLocale)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Ocp-Apim-Subscription-Key", p.cfg.SubscriptionKey)
	httpReq.Header.Set("Content-Type", "application/json")
	if p.cfg.Region != "" {
		httpReq.Header.Set("Ocp-Apim-Subscription-Region", p.cfg.Region)
	}

	respBody, err := sendAndRead(p.client, httpReq)
	if err != nil {
		return nil, err
	}

	var result []msTranslateResponseItem
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result) == 0 || len(result[0].Translations) == 0 {
		return nil, errors.New("no translations returned")
	}

	return &TranslateResponse{
		Translation: result[0].Translations[0].Text,
	}, nil
}

func (p *MicrosoftProvider) Close() error { return nil }

// MicrosoftToolConfig holds configuration for the Microsoft MT tool (provider config + locale overrides).
type MicrosoftToolConfig struct {
	MicrosoftConfig
	SourceLocale model.LocaleID `schema:"description=Source locale of the content"`
	TargetLocale model.LocaleID `schema:"description=Target locale for processing"`
}

// ToolName returns the tool name this config applies to.
func (c *MicrosoftToolConfig) ToolName() string { return "microsoft-translate" }

// Reset restores default values.
func (c *MicrosoftToolConfig) Reset() {
	c.SubscriptionKey = ""
	c.Region = ""
	c.SourceLocale = ""
	c.TargetLocale = ""
	c.BaseURL = ""
}

// Validate checks configuration validity.
func (c *MicrosoftToolConfig) Validate() error {
	if c.SubscriptionKey == "" {
		return errors.New("microsoft: SubscriptionKey is required")
	}
	if c.TargetLocale.IsEmpty() {
		return errors.New("microsoft: TargetLocale is required")
	}
	return nil
}

type msRequestBody struct {
	Text string `json:"Text"`
}

type msTranslateResponseItem struct {
	Translations []struct {
		Text string `json:"text"`
		To   string `json:"to"`
	} `json:"translations"`
}
