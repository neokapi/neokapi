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

// DefaultModernMTBaseURL is the default ModernMT API endpoint.
const DefaultModernMTBaseURL = "https://api.modernmt.com"

// ModernMTConfig holds configuration for the ModernMT provider.
type ModernMTConfig struct {
	APIKey  string  `schema:"description=ModernMT API key,widget=password"`
	Hints   []int64 `schema:"description=Optional memory IDs to bias translations"` // Optional memory IDs to bias translations
	BaseURL string  `schema:"description=API base URL override for testing"`        // Override for testing
}

// Validate checks configuration validity.
func (c *ModernMTConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("modernmt: APIKey is required")
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
	cfg    ModernMTConfig
	client *http.Client
}

// NewModernMTProvider creates a new ModernMT provider.
func NewModernMTProvider(cfg ModernMTConfig) *ModernMTProvider {
	return &ModernMTProvider{cfg: cfg, client: httputil.NewResilientClient()}
}

func (p *ModernMTProvider) Name() ProviderID { return ModernMT }

func (p *ModernMTProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
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

	apiURL := p.cfg.baseURL() + "/translate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("MMT-ApiKey", p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	respBody, err := sendAndRead(p.client, httpReq)
	if err != nil {
		return nil, err
	}

	var result modernMTResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Data.Translation == "" {
		return nil, errors.New("no translation returned")
	}

	return &TranslateResponse{
		Translation: result.Data.Translation,
	}, nil
}

func (p *ModernMTProvider) Close() error { return nil }

// ModernMTToolConfig holds configuration for the ModernMT tool (provider config + locale overrides).
type ModernMTToolConfig struct {
	ModernMTConfig
	SourceLocale model.LocaleID `schema:"description=Source locale of the content"`
	TargetLocale model.LocaleID `schema:"description=Target locale for processing"`
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
		return errors.New("modernmt: APIKey is required")
	}
	if c.TargetLocale.IsEmpty() {
		return errors.New("modernmt: TargetLocale is required")
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
