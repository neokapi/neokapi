//go:build js && wasm

package main

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	mttools "github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
)

// registerDemoMT registers an `mt-translate` tool backed by the deterministic
// demo MT provider. The native build wires real MT providers from typed
// credentials; the browser build has no keys, so the demo provider is the only
// MT engine available — it produces illustrative output with no network.
func registerDemoMT(reg *registry.ToolRegistry) {
	if reg == nil {
		return
	}
	s := schema.FromStruct(&mtprovider.DemoToolConfig{}, schema.ToolMeta{
		ID:           "mt-translate",
		Category:     schema.CategoryTranslation,
		DisplayName:  "MT Translate (demo)",
		Description:  "Translate content using the demo MT provider (illustrative output)",
		Inputs:       []string{schema.PartTypeBlock},
		Tags:         []string{"demo"},
		WritesOutput: true,
		Requires:     []string{schema.RequiresTargetLanguage},
		Cardinality:  schema.Bilingual,
		Produces:     []schema.AnnotationType{schema.AnnotationTranslation},
	})

	reg.RegisterWithSchema("mt-translate", func() tool.Tool {
		return mttools.NewMTTranslateTool(mtprovider.NewDemoProvider(), mttools.MTTranslateConfig{})
	}, s)

	reg.SetConfigFactory("mt-translate", func(config map[string]any, targetLang string) (tool.Tool, error) {
		var cfg mtprovider.DemoToolConfig
		if err := schema.ApplyConfig(config, &cfg); err != nil {
			return nil, fmt.Errorf("mt-translate config: %w", err)
		}
		if targetLang != "" {
			cfg.TargetLocale = model.LocaleID(targetLang)
		}
		return mttools.NewMTTranslateTool(mtprovider.NewDemoProvider(), mttools.MTTranslateConfig{
			SourceLocale: cfg.SourceLocale,
			TargetLocale: cfg.TargetLocale,
		}), nil
	})
}

// forceDemoProviders installs a tool-registry config preprocessor that coerces
// AI provider selection to the demo provider. In the browser there are no API
// keys and no network, so the real providers (anthropic, openai, …) cannot
// run; the deterministic demo provider is substituted so AI commands still
// produce illustrative output. An explicit `--provider demo` is already demo,
// so this is a no-op in that case.
//
// This lives entirely in the wasm wiring: it does not change native behavior,
// where the credential-resolution preprocessor set by App.Init remains in
// effect.
func forceDemoProviders(reg *registry.ToolRegistry) {
	if reg == nil {
		return
	}
	reg.SetConfigPreprocessor(func(_ string, _ []string, config map[string]any) (map[string]any, error) {
		if config == nil {
			config = map[string]any{}
		}
		// Only AI tools carry a "provider" key. Coerce any non-demo or empty
		// selection to the demo provider so the command runs offline.
		if _, ok := config["provider"]; ok {
			if p, _ := config["provider"].(string); p != string(aiprovider.Demo) {
				config["provider"] = string(aiprovider.Demo)
			}
			// Drop any model so the demo provider reports its own stub model.
			delete(config, "model")
			delete(config, "apiKey")
		}
		return config, nil
	})
}
