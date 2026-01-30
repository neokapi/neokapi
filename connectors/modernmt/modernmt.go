// Package modernmt provides a translation connector using the ModernMT API.
package modernmt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// DefaultBaseURL is the default ModernMT API endpoint.
const DefaultBaseURL = "https://api.modernmt.com"

// ModernMTConfig holds configuration for the ModernMT connector.
type ModernMTConfig struct {
	APIKey       string
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	Hints        []int64 // Optional memory IDs to bias translations
	BaseURL      string  // Override for testing
}

// ToolName returns the tool name this config applies to.
func (c *ModernMTConfig) ToolName() string { return "modernmt-translate" }

// Reset restores default values.
func (c *ModernMTConfig) Reset() {
	c.APIKey = ""
	c.SourceLocale = ""
	c.TargetLocale = ""
	c.Hints = nil
	c.BaseURL = ""
}

// Validate checks configuration validity.
func (c *ModernMTConfig) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("modernmt: APIKey is required")
	}
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("modernmt: TargetLocale is required")
	}
	return nil
}

func (c *ModernMTConfig) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

// translateRequest is the JSON body sent to the ModernMT API.
type translateRequest struct {
	Source string  `json:"source,omitempty"`
	Target string  `json:"target"`
	Q      string  `json:"q"`
	Hints  []int64 `json:"hints,omitempty"`
}

// translateResponse is the JSON response from the ModernMT API.
type translateResponse struct {
	Status int `json:"status"`
	Data   struct {
		Translation string `json:"translation"`
	} `json:"data"`
}

// NewModernMTConnector creates a new ModernMT translation connector.
func NewModernMTConnector(cfg *ModernMTConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "modernmt-translate",
		ToolDescription: "Translates text using the ModernMT adaptive neural MT API",
		Cfg:             cfg,
	}
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}
		if !block.Translatable {
			return part, nil
		}

		conf := t.Cfg.(*ModernMTConfig)
		sourceText := block.SourceText()
		if sourceText == "" {
			return part, nil
		}

		translated, err := callModernMTAPI(conf, sourceText)
		if err != nil {
			return nil, fmt.Errorf("modernmt-translate: %w", err)
		}
		block.SetTargetText(conf.TargetLocale, translated)
		return part, nil
	}
	return t
}

func callModernMTAPI(cfg *ModernMTConfig, text string) (string, error) {
	reqBody := translateRequest{
		Target: string(cfg.TargetLocale),
		Q:      text,
	}
	if !cfg.SourceLocale.IsEmpty() {
		reqBody.Source = string(cfg.SourceLocale)
	}
	if len(cfg.Hints) > 0 {
		reqBody.Hints = cfg.Hints
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/translate", cfg.baseURL())
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("MMT-ApiKey", cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result translateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Data.Translation == "" {
		return "", fmt.Errorf("no translation returned")
	}

	return result.Data.Translation, nil
}
