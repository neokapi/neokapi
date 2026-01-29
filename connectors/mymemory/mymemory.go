// Package mymemory provides a translation connector using the MyMemory free translation API.
package mymemory

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
)

// DefaultBaseURL is the default MyMemory API endpoint.
const DefaultBaseURL = "https://api.mymemory.translated.net"

// MyMemoryConfig holds configuration for the MyMemory connector.
type MyMemoryConfig struct {
	Email        string // Optional; provides higher rate limits when set
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	BaseURL      string // Override for testing
}

// ToolName returns the tool name this config applies to.
func (c *MyMemoryConfig) ToolName() string { return "mymemory-translate" }

// Reset restores default values.
func (c *MyMemoryConfig) Reset() {
	c.Email = ""
	c.SourceLocale = ""
	c.TargetLocale = ""
	c.BaseURL = ""
}

// Validate checks configuration validity.
func (c *MyMemoryConfig) Validate() error {
	if c.SourceLocale.IsEmpty() {
		return fmt.Errorf("mymemory: SourceLocale is required")
	}
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("mymemory: TargetLocale is required")
	}
	return nil
}

func (c *MyMemoryConfig) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

// translateResponse is the JSON response from the MyMemory API.
type translateResponse struct {
	ResponseData struct {
		TranslatedText string  `json:"translatedText"`
		Match          float64 `json:"match"`
	} `json:"responseData"`
	ResponseStatus int `json:"responseStatus"`
}

// NewMyMemoryConnector creates a new MyMemory translation connector.
func NewMyMemoryConnector(cfg *MyMemoryConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "mymemory-translate",
		ToolDescription: "Translates text using the MyMemory free translation API",
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

		conf := t.Cfg.(*MyMemoryConfig)
		sourceText := block.SourceText()
		if sourceText == "" {
			return part, nil
		}

		translated, err := callMyMemoryAPI(conf, sourceText)
		if err != nil {
			return nil, fmt.Errorf("mymemory-translate: %w", err)
		}
		block.SetTargetText(conf.TargetLocale, translated)
		return part, nil
	}
	return t
}

func callMyMemoryAPI(cfg *MyMemoryConfig, text string) (string, error) {
	langPair := fmt.Sprintf("%s|%s", string(cfg.SourceLocale), string(cfg.TargetLocale))

	params := url.Values{}
	params.Set("q", text)
	params.Set("langpair", langPair)
	if cfg.Email != "" {
		params.Set("de", cfg.Email)
	}

	apiURL := fmt.Sprintf("%s/get?%s", cfg.baseURL(), params.Encode())
	resp, err := http.Get(apiURL)
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

	if result.ResponseStatus != 200 {
		return "", fmt.Errorf("API response status %d", result.ResponseStatus)
	}

	return result.ResponseData.TranslatedText, nil
}
