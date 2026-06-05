package mtprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/neokapi/neokapi/core/httputil"
	"github.com/neokapi/neokapi/core/model"
)

// DefaultMyMemoryBaseURL is the default MyMemory API endpoint.
const DefaultMyMemoryBaseURL = "https://api.mymemory.translated.net"

// MyMemoryConfig holds configuration for the MyMemory provider.
type MyMemoryConfig struct {
	Email   string `schema:"description=Email address for higher API rate limits (optional)"` // Optional; provides higher rate limits when set
	BaseURL string `schema:"description=API base URL override for testing"`                   // Override for testing
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
	cfg    MyMemoryConfig
	client *http.Client
}

// NewMyMemoryProvider creates a new MyMemory MT provider.
func NewMyMemoryProvider(cfg MyMemoryConfig) *MyMemoryProvider {
	return &MyMemoryProvider{cfg: cfg, client: httputil.NewResilientClient()}
}

func (p *MyMemoryProvider) Name() ProviderID { return MyMemory }

func (p *MyMemoryProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	if req.SourceLocale.IsEmpty() {
		return nil, errors.New("mymemory: SourceLocale is required")
	}
	if req.TargetLocale.IsEmpty() {
		return nil, errors.New("mymemory: TargetLocale is required")
	}

	langPair := fmt.Sprintf("%s|%s", string(req.SourceLocale), string(req.TargetLocale))

	params := url.Values{}
	params.Set("q", req.Source)
	params.Set("langpair", langPair)
	if p.cfg.Email != "" {
		params.Set("de", p.cfg.Email)
	}

	apiURL := fmt.Sprintf("%s/get?%s", p.cfg.baseURL(), params.Encode())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	respBody, err := sendAndRead(p.client, httpReq)
	if err != nil {
		return nil, err
	}

	var result myMemoryResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.ResponseStatus != 200 {
		return nil, fmt.Errorf("api response status %d", result.ResponseStatus)
	}

	return &TranslateResponse{
		Translation: result.ResponseData.TranslatedText,
	}, nil
}

func (p *MyMemoryProvider) Close() error { return nil }

// MyMemoryToolConfig holds configuration for the MyMemory MT tool (provider config + locale overrides).
type MyMemoryToolConfig struct {
	MyMemoryConfig
	SourceLocale model.LocaleID `schema:"description=Source locale of the content"`
	TargetLocale model.LocaleID `schema:"description=Target locale for processing"`
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
		return errors.New("mymemory: SourceLocale is required")
	}
	if c.TargetLocale.IsEmpty() {
		return errors.New("mymemory: TargetLocale is required")
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
