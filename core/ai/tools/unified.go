package tools

import (
	"slices"

	"github.com/neokapi/neokapi/core/model"
	mttools "github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// This file defines the consolidated top-level `translate` and `qa` tools.
//
// `translate` is a single command whose `--provider` selects the backend: any
// LLM provider (anthropic, openai, …) routes to the AI translate tool, any MT
// engine (deepl, google, …) routes to the machine-translation tool. There is no
// longer a `<provider>-translate` command per engine, nor a separate
// `ai-translate` — provider is the only dimension.
//
// `qa` mirrors that shape: with no `--provider` it runs the deterministic,
// rule-based quality checks (no credentials needed); with `--provider` set it
// runs LLM-judged QA. Both underlying implementations (core/tools' rule checks
// and core/ai/tools' LLM judge) are reached through one verb.

// isMTProvider reports whether a provider id names a machine-translation engine
// (as opposed to an LLM provider). MT ids route `translate` to the MT tool.
func isMTProvider(id string) bool {
	return slices.ContainsFunc(mttools.Providers, func(p mttools.Provider) bool {
		return string(p.ID) == id
	})
}

// TranslateSchema returns the schema for the unified `translate` tool: the AI
// translate fields plus the MT-only credential fields, with a provider enum
// spanning every LLM and MT provider.
func TranslateSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&AITranslateConfig{}, schema.ToolMeta{
		ID:                    "translate",
		Category:              schema.CategoryTranslation,
		DisplayName:           "Translate",
		Description:           "Translate content with an LLM or machine-translation provider (select with --provider)",
		Tags:                  []string{"translation"},
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresTargetLanguage, schema.RequiresCredentials},
		Cardinality:           schema.Bilingual,
		Produces:              []schema.IOPort{{Type: schema.PortTarget, Side: model.SideTarget}},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	})
	// Fold in the MT-only provider fields (Azure subscription key/region, the
	// MyMemory account email, the Google project id). The shared apiKey/model
	// already come from AITranslateConfig.
	mt := schema.FromStruct(&mttools.MTTranslateConfig{}, schema.ToolMeta{ID: "translate"})
	mergeProperties(s, mt, "subscriptionKey", "region", "email", "projectId")
	setProviderOptions(s, allTranslateProviders())
	s.BuildRawJSON()
	return s
}

// QASchema returns the schema for the unified `qa` tool: every deterministic
// rule-check toggle plus the AI provider fields. The provider default is empty,
// so `qa` runs rule-based checks unless `--provider` opts into LLM judgement.
func QASchema() *schema.ComponentSchema {
	s := schema.FromStruct(libtools.NewQACheckConfig(model.LocaleEnglish), schema.ToolMeta{
		ID:                    "qa",
		Category:              schema.CategoryQuality,
		DisplayName:           "Quality Check",
		Description:           "Check translation quality with rule-based checks, or an LLM when --provider is set",
		Tags:                  []string{"quality"},
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresTargetLanguage, schema.RequiresCredentials},
		Cardinality:           schema.Bilingual,
		Consumes:              []schema.IOPort{schema.Port(schema.PortTarget, model.SideTarget)},
		Produces:              []schema.IOPort{schema.Port(model.OverlayQA, model.SideTarget)},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	})
	ai := schema.FromStruct(&AIQAConfig{}, schema.ToolMeta{ID: "qa"})
	mergeProperties(s, ai, "provider", "apiKey", "model", "checks")
	// LLM QA is opt-in: an empty provider keeps `qa` deterministic.
	if p, ok := s.Properties["provider"]; ok {
		p.Default = ""
		p.Description = "LLM provider for AI-judged QA (omit for rule-based checks)"
		s.Properties["provider"] = p
	}
	setProviderOptions(s, aiProviderOptions())
	s.BuildRawJSON()
	return s
}

// NewTranslateFromConfig builds the translation tool for the configured
// provider: an MT engine when the provider id names one, otherwise the LLM
// translator (which itself defaults to anthropic when no provider is given).
func NewTranslateFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	if provider, _ := config["provider"].(string); isMTProvider(provider) {
		for _, p := range mttools.Providers {
			if string(p.ID) == provider {
				return mttools.NewMTTranslateFromConfig(p.ID)(config, targetLang)
			}
		}
	}
	return NewAITranslateFromConfig(config, targetLang)
}

// NewQAFromConfig builds the deterministic rule-based checker when no provider
// is set, or the LLM-judged checker when `--provider` selects one.
func NewQAFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	if provider, _ := config["provider"].(string); provider != "" {
		return NewAIQAFromConfig(config, targetLang)
	}
	return libtools.NewQACheckFromConfig(config, targetLang)
}

// ResolveQAContract refines `qa`'s contract from its config: with no provider
// the tool runs local rule checks — no credentials, no API call, no egress.
// With a provider it resolves like the other AI tools (local providers drop the
// egress effect; cloud providers keep the full contract).
func ResolveQAContract(config map[string]any, base registry.ToolInfo) registry.ToolInfo {
	provider, _ := config["provider"].(string)
	if provider != "" {
		return ResolveAIEgressContract(config, base)
	}
	base.Requires = removeStrings(base.Requires, schema.RequiresCredentials)
	base.SideEffects = removeEffects(base.SideEffects, schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress)
	return base
}

// mergeProperties copies the named properties (and their group membership) from
// src into dst. Used to fold one config struct's fields into another's schema.
func mergeProperties(dst, src *schema.ComponentSchema, keys ...string) {
	keep := make(map[string]bool, len(keys))
	for _, k := range keys {
		keep[k] = true
	}
	for name, prop := range src.Properties {
		if keep[name] {
			dst.Properties[name] = prop
		}
	}
	for _, g := range src.Groups {
		var fields []string
		for _, f := range g.Fields {
			if keep[f] {
				fields = append(fields, f)
			}
		}
		if len(fields) == 0 {
			continue
		}
		idx := slices.IndexFunc(dst.Groups, func(x schema.ParameterGroup) bool { return x.ID == g.ID })
		if idx == -1 {
			ng := g
			ng.Fields = fields
			dst.Groups = append(dst.Groups, ng)
			continue
		}
		for _, f := range fields {
			if !slices.Contains(dst.Groups[idx].Fields, f) {
				dst.Groups[idx].Fields = append(dst.Groups[idx].Fields, f)
			}
		}
	}
}

// setProviderOptions replaces the "provider" property's enum options.
func setProviderOptions(s *schema.ComponentSchema, opts []schema.OptionItem) {
	if p, ok := s.Properties["provider"]; ok {
		p.Options = opts
		s.Properties["provider"] = p
	}
}

// aiProviderOptions lists every registered LLM provider as enum options.
func aiProviderOptions() []schema.OptionItem {
	var opts []schema.OptionItem
	for _, p := range aiprovider.Providers() {
		opts = append(opts, schema.OptionItem{Value: p.Name, Label: p.Label})
	}
	return opts
}

// allTranslateProviders lists every LLM provider followed by every MT engine.
func allTranslateProviders() []schema.OptionItem {
	opts := aiProviderOptions()
	for _, p := range mttools.Providers {
		opts = append(opts, schema.OptionItem{Value: string(p.ID), Label: p.Label})
	}
	return opts
}

func removeStrings(in []string, drop ...string) []string {
	out := in[:0:0]
	for _, v := range in {
		if !slices.Contains(drop, v) {
			out = append(out, v)
		}
	}
	return out
}

func removeEffects(in []schema.SideEffect, drop ...schema.SideEffect) []schema.SideEffect {
	out := in[:0:0]
	for _, v := range in {
		if !slices.Contains(drop, v) {
			out = append(out, v)
		}
	}
	return out
}
