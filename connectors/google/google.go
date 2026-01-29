// Package google provides a translation connector using the Google Cloud Translation API v2.
package google

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
)

// DefaultBaseURL is the default Google Translate API endpoint.
const DefaultBaseURL = "https://translation.googleapis.com"

// GoogleConfig holds configuration for the Google Translate connector.
type GoogleConfig struct {
	APIKey       string
	ProjectID    string
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	BaseURL      string // Override for testing
}

// ToolName returns the tool name this config applies to.
func (c *GoogleConfig) ToolName() string { return "google-translate" }

// Reset restores default values.
func (c *GoogleConfig) Reset() {
	c.APIKey = ""
	c.ProjectID = ""
	c.SourceLocale = ""
	c.TargetLocale = ""
	c.BaseURL = ""
}

// Validate checks configuration validity.
func (c *GoogleConfig) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("google: APIKey is required")
	}
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("google: TargetLocale is required")
	}
	return nil
}

func (c *GoogleConfig) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

// translateRequest is the JSON body sent to the Google Translate API.
type translateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source,omitempty"`
	Target string `json:"target"`
	Format string `json:"format"`
}

// translateResponse is the JSON response from the Google Translate API.
type translateResponse struct {
	Data struct {
		Translations []struct {
			TranslatedText string `json:"translatedText"`
		} `json:"translations"`
	} `json:"data"`
}

// NewGoogleTranslateConnector creates a new Google Translate connector.
func NewGoogleTranslateConnector(cfg *GoogleConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "google-translate",
		ToolDescription: "Translates text using Google Cloud Translation API v2",
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

		conf := t.Cfg.(*GoogleConfig)
		sourceText := block.SourceText()
		if sourceText == "" {
			return part, nil
		}

		translated, err := callGoogleAPI(conf, sourceText)
		if err != nil {
			return nil, fmt.Errorf("google-translate: %w", err)
		}
		block.SetTargetText(conf.TargetLocale, translated)
		return part, nil
	}
	return t
}

func callGoogleAPI(cfg *GoogleConfig, text string) (string, error) {
	reqBody := translateRequest{
		Q:      text,
		Target: string(cfg.TargetLocale),
		Format: "text",
	}
	if !cfg.SourceLocale.IsEmpty() {
		reqBody.Source = string(cfg.SourceLocale)
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/language/translate/v2?key=%s", cfg.baseURL(), cfg.APIKey)
	resp, err := http.Post(url, "application/json", bytes.NewReader(bodyBytes))
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

	if len(result.Data.Translations) == 0 {
		return "", fmt.Errorf("no translations returned")
	}

	return result.Data.Translations[0].TranslatedText, nil
}
