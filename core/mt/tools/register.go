package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
)

// mtProviders is the canonical list of MT providers that ship a
// <provider>-translate tool. The display labels mirror the providers'
// documented names; the ids match providers/mt's ProviderID constants.
var mtProviders = []struct {
	ID    mtprovider.ProviderID
	Label string
}{
	{mtprovider.DeepL, "DeepL"},
	{mtprovider.Google, "Google Translate"},
	{mtprovider.MSFT, "Microsoft Translator"},
	{mtprovider.ModernMT, "ModernMT"},
	{mtprovider.MyMemory, "MyMemory"},
}

// RegisterAll registers all MT-powered <provider>-translate tools in the given
// ToolRegistry, mirroring how the AI tools register (core/ai/tools).
//
// Each tool requires credentials at runtime; the default factory uses the
// deterministic, offline demo provider so the tool is constructible (and lists
// in `kapi tools`) without any keys. The config factory resolves the real
// provider — keyed by the registered tool name — from the (credential-resolved)
// config map at tool-creation time.
func RegisterAll(reg *registry.ToolRegistry) {
	for _, p := range mtProviders {
		toolName := string(p.ID) + "-translate"
		name := registry.ToolID(toolName)

		// Default factory: a demo-backed tool keeps the registry entry
		// constructible with no credentials (parity with ai tools using
		// the mock provider as the default factory). The registered tool name
		// is pinned via the config so the tool reports e.g. "deepl-translate"
		// even though it is demo-backed.
		defaultFactory := func() tool.Tool {
			return NewMTTranslateTool(mtprovider.NewDemoProvider(), MTTranslateConfig{ToolName: toolName})
		}

		reg.RegisterWithSchema(name, defaultFactory, MTTranslateSchema(p.ID, p.Label))
		reg.SetConfigFactory(name, newMTTranslateFromConfig(p.ID))
	}
}

// MTTranslateSchema returns the schema + metadata for a single
// <provider>-translate tool. Modeled on AITranslateSchema.
func MTTranslateSchema(id mtprovider.ProviderID, label string) *schema.ComponentSchema {
	toolID := string(id) + "-translate"
	return schema.FromStruct(&MTTranslateConfig{}, schema.ToolMeta{
		ID:          toolID,
		Category:    schema.CategoryTranslation,
		DisplayName: label + " Translate",
		Description: "Translate content using " + label,
		Tags:        []string{"mt", "machine-translation"},
		// MT is a network round-trip per block, like ai-translate — parallelism
		// hides API latency. ai-translate defaults to 5; match it.
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresTargetLanguage, schema.RequiresCredentials},
		Cardinality:           schema.Bilingual,
		Produces:              []schema.IOPort{{Type: schema.PortTarget, Side: model.SideTarget}},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall},
	})
}

// newMTTranslateFromConfig returns a ToolConfigFactory bound to a specific MT
// provider id. The provider is fixed by the registered tool name; the config
// map carries credentials (resolved by the CLI preprocessor) and locale.
func newMTTranslateFromConfig(id mtprovider.ProviderID) registry.ToolConfigFactory {
	toolName := string(id) + "-translate"
	return func(config map[string]any, targetLang string) (tool.Tool, error) {
		var cfg MTTranslateConfig
		if err := schema.ApplyConfig(config, &cfg); err != nil {
			return nil, fmt.Errorf("%s config: %w", toolName, err)
		}
		cfg.ToolName = toolName
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
