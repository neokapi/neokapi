// Package deepl provides a translation connector using the DeepL API.
package deepl

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// DefaultBaseURL is the default DeepL free API endpoint.
const DefaultBaseURL = "https://api-free.deepl.com"

// Formality levels supported by DeepL.
const (
	FormalityDefault    = "default"
	FormalityMore       = "more"
	FormalityLess       = "less"
	FormalityPreferMore = "prefer_more"
	FormalityPreferLess = "prefer_less"
)

// DeepLConfig holds configuration for the DeepL connector.
type DeepLConfig struct {
	APIKey       string
	Formality    string // "default", "more", "less", "prefer_more", "prefer_less"
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	BaseURL      string // Override for testing
}

// ToolName returns the tool name this config applies to.
func (c *DeepLConfig) ToolName() string { return "deepl-translate" }

// Reset restores default values.
func (c *DeepLConfig) Reset() {
	c.APIKey = ""
	c.Formality = FormalityDefault
	c.SourceLocale = ""
	c.TargetLocale = ""
	c.BaseURL = ""
}

// Validate checks configuration validity.
func (c *DeepLConfig) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("deepl: APIKey is required")
	}
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("deepl: TargetLocale is required")
	}
	return nil
}

func (c *DeepLConfig) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

// translateResponse is the JSON response from the DeepL API.
type translateResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
}

// NewDeepLConnector creates a new DeepL translation connector.
func NewDeepLConnector(cfg *DeepLConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "deepl-translate",
		ToolDescription: "Translates text using the DeepL API",
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

		conf := t.Cfg.(*DeepLConfig)
		sourceText := block.SourceText()
		if sourceText == "" {
			return part, nil
		}

		translated, err := callDeepLAPI(conf, sourceText)
		if err != nil {
			return nil, fmt.Errorf("deepl-translate: %w", err)
		}
		block.SetTargetText(conf.TargetLocale, translated)
		return part, nil
	}
	return t
}

func callDeepLAPI(cfg *DeepLConfig, text string) (string, error) {
	form := url.Values{}
	form.Set("text", text)
	form.Set("target_lang", strings.ToUpper(string(cfg.TargetLocale)))
	if !cfg.SourceLocale.IsEmpty() {
		form.Set("source_lang", strings.ToUpper(string(cfg.SourceLocale)))
	}
	if cfg.Formality != "" && cfg.Formality != FormalityDefault {
		form.Set("formality", cfg.Formality)
	}

	apiURL := fmt.Sprintf("%s/v2/translate", cfg.baseURL())
	req, err := http.NewRequest(http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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

	if len(result.Translations) == 0 {
		return "", fmt.Errorf("no translations returned")
	}

	return result.Translations[0].Text, nil
}
