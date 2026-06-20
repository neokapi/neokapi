package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
)

// Provider names a machine-translation engine that the unified `translate` tool
// can route to via --provider. The id matches a providers/mt ProviderID
// constant; the label mirrors the provider's documented name.
type Provider struct {
	ID    mtprovider.ProviderID
	Label string
}

// Providers is the canonical list of MT engines reachable through
// `kapi translate --provider <id>`. The unified translate tool (core/ai/tools)
// reads this to populate its provider enum and to dispatch by id.
var Providers = []Provider{
	{mtprovider.DeepL, "DeepL"},
	{mtprovider.Google, "Google Translate"},
	{mtprovider.MSFT, "Microsoft Translator"},
	{mtprovider.ModernMT, "ModernMT"},
	{mtprovider.MyMemory, "MyMemory"},
}

// NewMTTranslateFromConfig returns a ToolConfigFactory bound to a specific MT
// engine. The provider is fixed by id; the config map carries credentials
// (resolved by the CLI preprocessor) and the target locale. The unified
// `translate` tool calls this once it has classified --provider as an MT engine.
func NewMTTranslateFromConfig(id mtprovider.ProviderID) registry.ToolConfigFactory {
	return func(config map[string]any, targetLang string) (tool.Tool, error) {
		var cfg MTTranslateConfig
		if err := schema.ApplyConfig(config, &cfg); err != nil {
			return nil, fmt.Errorf("%s translate config: %w", id, err)
		}
		cfg.ToolName = "translate"
		if targetLang != "" {
			cfg.TargetLocale = model.LocaleID(targetLang)
		}

		p, err := mtprovider.NewProviderWithConfig(id, mtprovider.MTConfig{
			APIKey:          cfg.APIKey,
			SubscriptionKey: cfg.SubscriptionKey,
			Region:          cfg.Region,
			Email:           cfg.Email,
			ProjectID:       cfg.ProjectID,
			BaseURL:         cfg.BaseURL,
		})
		if err != nil {
			return nil, err
		}

		return NewMTTranslateTool(p, cfg), nil
	}
}
