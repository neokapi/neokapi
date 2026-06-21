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

// mtProviderExtraFields lists the credential fields each MT provider needs
// beyond the shared apiKey. Providers absent here (deepl, modernmt, and every
// LLM provider) contribute only a selector option — their config is the common
// apiKey/model already on the base schema.
var mtProviderExtraFields = map[string][]string{
	"microsoft": {"subscriptionKey", "region"},
	"google":    {"projectId"},
	"mymemory":  {"email"},
}

// TranslateSchema returns the schema for the unified `translate` tool: a
// `provider` selector spanning every LLM and MT provider, where choosing a
// provider reveals only that provider's extra credentials (Azure key/region,
// Google project id, MyMemory email) instead of showing them all at once. The
// shared apiKey/model and batch options stay common.
func TranslateSchema() *schema.ComponentSchema {
	base := schema.FromStruct(&AITranslateConfig{}, schema.ToolMeta{
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

	// Source the per-provider extra credential fields from MTTranslateConfig.
	mt := schema.FromStruct(&mttools.MTTranslateConfig{}, schema.ToolMeta{ID: "translate-mt"})
	variants := make([]schema.Variant, 0)
	for _, opt := range allTranslateProviders() {
		name, _ := opt.Value.(string)
		v := schema.Variant{Name: name, Label: opt.Label}
		if fields, ok := mtProviderExtraFields[name]; ok {
			props := make(map[string]schema.PropertySchema, len(fields))
			for i, f := range fields {
				p := mt.Properties[f]
				ord := (i + 1) * 10
				p.Order = &ord
				props[f] = p
			}
			v.Params = &schema.ComponentSchema{Type: "object", Properties: props}
		}
		variants = append(variants, v)
	}
	return schema.ComposeVariants(base, "provider", "anthropic", variants)
}

// qaToolMeta is the QA tool metadata shared by the schema and contract resolver.
// The contract is the AI-path superset (credentials + API egress); rule mode
// drops those at runtime via ResolveQAContract.
func qaToolMeta() schema.ToolMeta {
	return schema.ToolMeta{
		ID:                    "qa",
		Category:              schema.CategoryQuality,
		DisplayName:           "Quality Check",
		Description:           "Check translation quality with deterministic rules or an LLM (select with --mode)",
		Tags:                  []string{"quality"},
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresTargetLanguage, schema.RequiresCredentials},
		Cardinality:           schema.Bilingual,
		Consumes:              []schema.IOPort{schema.Port(schema.PortTarget, model.SideTarget)},
		Produces:              []schema.IOPort{schema.Port(model.OverlayQA, model.SideTarget)},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	}
}

// QASchema returns the schema for the unified `qa` tool: a `mode` selector
// (Deterministic rules / AI review) whose choice reveals only that backend's
// config — the rule-check toggles for rules, the provider/model fields for AI.
// Default mode is rules, so `qa` needs no credentials unless AI is selected.
func QASchema() *schema.ComponentSchema {
	meta := qaToolMeta()
	base := &schema.ComponentSchema{
		ID:          "qa",
		Title:       meta.DisplayName,
		Description: meta.Description,
		Type:        "object",
		ToolMeta:    &meta,
		Properties: map[string]schema.PropertySchema{
			qaModeField: {
				Type:        "string",
				Title:       "QA Mode",
				Description: "How to check quality: deterministic local rules, or an AI provider's review",
				Default:     qaModeRules,
			},
		},
		Groups: []schema.ParameterGroup{{ID: "qa", Label: "Quality check", Fields: []string{qaModeField}}},
	}
	rules := schema.FromStruct(libtools.NewQACheckConfig(model.LocaleEnglish), schema.ToolMeta{ID: "qa-rules"})
	ai := schema.FromStruct(&AIQAConfig{}, schema.ToolMeta{ID: "qa-ai"})
	setProviderOptions(ai, aiProviderOptions())
	return schema.ComposeVariants(base, qaModeField, qaModeRules, []schema.Variant{
		{Name: qaModeRules, Label: "Deterministic rules", Description: "Local rule-based checks — no credentials, no network.", Params: rules},
		{Name: qaModeAI, Label: "AI review", Description: "LLM-judged quality review via an AI provider.", Params: ai},
	})
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

// QA mode discriminator: the `qa` tool selects its backend with `mode`
// (Deterministic rules vs AI review), replacing the old "provider-presence"
// heuristic. The values double as the ComposeVariants variant names.
const (
	qaModeField = "mode"
	qaModeRules = "rules"
	qaModeAI    = "ai"
)

// qaUsesAI reports whether a qa config selects the AI backend. The explicit
// `mode` wins; when it is unset (older recipes / flags), it falls back to the
// historical rule: a set `provider` means AI.
func qaUsesAI(config map[string]any) bool {
	switch mode, _ := config[qaModeField].(string); mode {
	case qaModeAI:
		return true
	case qaModeRules:
		return false
	default:
		provider, _ := config["provider"].(string)
		return provider != ""
	}
}

// NewQAFromConfig builds the deterministic rule-based checker in rules mode, or
// the LLM-judged checker in AI mode.
func NewQAFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	if qaUsesAI(config) {
		return NewAIQAFromConfig(config, targetLang)
	}
	return libtools.NewQACheckFromConfig(config, targetLang)
}

// ResolveQAContract refines `qa`'s contract from its config: in rules mode the
// tool runs local rule checks — no credentials, no API call, no egress. In AI
// mode it resolves like the other AI tools (local providers drop the egress
// effect; cloud providers keep the full contract).
func ResolveQAContract(config map[string]any, base registry.ToolInfo) registry.ToolInfo {
	if qaUsesAI(config) {
		return ResolveAIEgressContract(config, base)
	}
	base.Requires = removeStrings(base.Requires, schema.RequiresCredentials)
	base.SideEffects = removeEffects(base.SideEffects, schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress)
	return base
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
		opts = append(opts, schema.OptionItem{Value: string(p.Name), Label: p.Label})
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
