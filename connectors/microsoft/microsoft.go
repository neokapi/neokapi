// Package microsoft provides a translation connector using the Azure Cognitive Services Translator API.
package microsoft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// DefaultBaseURL is the default Azure Translator API endpoint.
const DefaultBaseURL = "https://api.cognitive.microsofttranslator.com"

// MSConfig holds configuration for the Microsoft Translator connector.
type MSConfig struct {
	SubscriptionKey string
	Region          string
	SourceLocale    model.LocaleID
	TargetLocale    model.LocaleID
	BaseURL         string // Override for testing
}

// ToolName returns the tool name this config applies to.
func (c *MSConfig) ToolName() string { return "microsoft-translate" }

// Reset restores default values.
func (c *MSConfig) Reset() {
	c.SubscriptionKey = ""
	c.Region = ""
	c.SourceLocale = ""
	c.TargetLocale = ""
	c.BaseURL = ""
}

// Validate checks configuration validity.
func (c *MSConfig) Validate() error {
	if c.SubscriptionKey == "" {
		return fmt.Errorf("microsoft: SubscriptionKey is required")
	}
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("microsoft: TargetLocale is required")
	}
	return nil
}

func (c *MSConfig) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

// requestBody is a single element in the JSON array sent to the API.
type requestBody struct {
	Text string `json:"Text"`
}

// translateResponseItem is a single element in the JSON response array.
type translateResponseItem struct {
	Translations []struct {
		Text string `json:"text"`
		To   string `json:"to"`
	} `json:"translations"`
}

// NewMicrosoftTranslatorConnector creates a new Microsoft Translator connector.
func NewMicrosoftTranslatorConnector(cfg *MSConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "microsoft-translate",
		ToolDescription: "Translates text using Azure Cognitive Services Translator",
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

		conf := t.Cfg.(*MSConfig)
		sourceText := block.SourceText()
		if sourceText == "" {
			return part, nil
		}

		translated, err := callMicrosoftAPI(conf, sourceText)
		if err != nil {
			return nil, fmt.Errorf("microsoft-translate: %w", err)
		}
		block.SetTargetText(conf.TargetLocale, translated)
		return part, nil
	}
	return t
}

func callMicrosoftAPI(cfg *MSConfig, text string) (string, error) {
	body := []requestBody{{Text: text}}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/translate?api-version=3.0&to=%s", cfg.baseURL(), string(cfg.TargetLocale))
	if !cfg.SourceLocale.IsEmpty() {
		apiURL += fmt.Sprintf("&from=%s", string(cfg.SourceLocale))
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", cfg.SubscriptionKey)
	req.Header.Set("Content-Type", "application/json")
	if cfg.Region != "" {
		req.Header.Set("Ocp-Apim-Subscription-Region", cfg.Region)
	}

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

	var result []translateResponseItem
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result) == 0 || len(result[0].Translations) == 0 {
		return "", fmt.Errorf("no translations returned")
	}

	return result[0].Translations[0].Text, nil
}
