package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
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
//
// The framework ships no classic MT engines — the translation core is
// LLM-first, and the built-in offline demo provider routes through the LLM
// path (providers/ai's demo). Plugins that host MT engines append here (and
// register a config factory via mtprovider.RegisterConfigFactory) to surface
// them in the translate tool.
var Providers = []Provider{}

// NewMTTranslateFromConfig returns a ToolConfigFactory bound to a specific MT
// engine. The provider is fixed by id; the config map carries credentials
// (resolved by the CLI preprocessor) and the target locale. The unified
// `translate` tool calls this once it has classified --provider as an MT engine.
func NewMTTranslateFromConfig(id mtprovider.ProviderID) registry.ToolConfigFactory {
	return func(config map[string]any, targetLang string) (tool.Tool, error) {
		// The voice profile is injected by the flow bindings as a live pointer, not
		// a serializable value, so it is lifted out before the JSON round-trip that
		// ApplyConfig performs. The glossary is a plain map and binds directly.
		var profile *coreprofile.VoiceProfile
		if pf, ok := config["profile"].(*coreprofile.VoiceProfile); ok {
			profile = pf
			delete(config, "profile")
		}

		var cfg MTTranslateConfig
		if err := schema.ApplyConfig(config, &cfg); err != nil {
			return nil, fmt.Errorf("%s translate config: %w", id, err)
		}
		cfg.Profile = profile
		cfg.ToolName = "translate"
		if targetLang != "" {
			cfg.TargetLocale = model.LocaleID(targetLang)
		}

		p, err := mtprovider.NewProviderWithConfig(id, mtprovider.MTConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
		})
		if err != nil {
			return nil, err
		}

		return NewMTTranslateTool(p, cfg), nil
	}
}
