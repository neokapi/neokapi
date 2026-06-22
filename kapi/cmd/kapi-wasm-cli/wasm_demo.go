//go:build js && wasm

package main

import (
	"errors"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/model"
	mttools "github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// mtToolConfig is the wasm mt-translate tool's schema: the source/target locales
// plus a `provider` choice so `--provider browser|demo` is exposed as a flag.
// Resolved by resolveWasmMTProvider — unset/`browser` use the on-device Translator
// API where the page supports it, `demo` (or no Translator API) the keyless stub.
type mtToolConfig struct {
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty" schema:"description=Source locale of the content"`
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"description=Target locale for processing"`
	Provider     string         `json:"provider,omitempty"     schema:"title=MT engine,description=browser (on-device Translator API, the default where supported) or demo (illustrative output)"`
}

func (c *mtToolConfig) ToolName() string { return "mt-translate" }
func (c *mtToolConfig) Reset()           { *c = mtToolConfig{} }
func (c *mtToolConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("mt-translate: TargetLocale is required")
	}
	return nil
}

// registerMT registers an `mt-translate` tool whose engine is resolved per run by
// resolveWasmMTProvider: the browser's built-in on-device Translator API
// (`--provider browser`, the default where the page supports it) or — when that
// API is absent or `--provider demo` is requested — the deterministic, keyless
// demo provider that produces illustrative output with no network.
func registerMT(reg *registry.ToolRegistry) {
	if reg == nil {
		return
	}
	s := schema.FromStruct(&mtToolConfig{}, schema.ToolMeta{
		ID:           "mt-translate",
		Category:     schema.CategoryTranslation,
		DisplayName:  "MT Translate",
		Description:  "Machine-translate content on-device with the browser's Translator API where available, or illustrative demo output otherwise",
		WritesOutput: true,
		Requires:     []string{schema.RequiresTargetLanguage},
		Cardinality:  schema.Bilingual,
		Produces:     []schema.IOPort{{Type: schema.PortTarget, Side: model.SideTarget}},
	})

	reg.RegisterWithSchema("mt-translate", func() tool.Tool {
		return mttools.NewMTTranslateTool(resolveWasmMTProvider(nil), mttools.MTTranslateConfig{})
	}, s)

	reg.SetConfigFactory("mt-translate", func(config map[string]any, targetLang string) (tool.Tool, error) {
		var cfg mtToolConfig
		if err := schema.ApplyConfig(config, &cfg); err != nil {
			return nil, fmt.Errorf("mt-translate config: %w", err)
		}
		if targetLang != "" {
			cfg.TargetLocale = model.LocaleID(targetLang)
		}
		return mttools.NewMTTranslateTool(resolveWasmMTProvider(config), mttools.MTTranslateConfig{
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
	reg.SetConfigPreprocessor(func(_ string, requires []string, config map[string]any) (map[string]any, error) {
		if config == nil {
			config = map[string]any{}
		}
		// Real in-browser providers run via a host JS bridge, not a credentialed
		// network call: `local` (a model via WebLLM/WebGPU, transformers.js
		// fallback) and `browser` (the on-device Translator API). Let either
		// through untouched (drop only a stray key) so the demo coercion below does
		// not override it — the mt-translate config factory then resolves `browser`
		// to the demo provider itself when the page lacks the Translator API.
		if prov, _ := config["provider"].(string); prov == string(localProviderID) || prov == string(browserMTProviderID) {
			delete(config, "apiKey")
			return config, nil
		}
		// A tool that requires credentials is provider-backed (AI/MT). Coerce it
		// to the deterministic demo provider even when no provider was selected —
		// which is the case inside a flow or recipe, where buildFlowTools omits
		// the provider key unless --provider was passed. Without this the tool
		// would fall back to its real default (anthropic) and the browser fetch
		// fails (no key, blocked by CORS). The single-tool path already carries a
		// provider key; coerce that too. An explicit `--provider demo` is a no-op.
		_, hasProvider := config["provider"]
		if hasProvider || slices.Contains(requires, schema.RequiresCredentials) {
			config["provider"] = string(aiprovider.Demo)
			// Drop any model/key so the demo provider reports its own stub model.
			delete(config, "model")
			delete(config, "apiKey")
		}
		return config, nil
	})
}
