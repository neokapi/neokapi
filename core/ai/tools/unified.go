package tools

import (
	"slices"
	"sort"

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

// Translate is a two-level group: an engine discriminator (AI LLM vs machine
// translation) selects which providers are offered, and the provider drives
// runtime dispatch (NewTranslateFromConfig keys on provider, deriving the engine
// from it). The engine field exists for the cascading UI — it filters the
// provider list — and as the group discriminator that gates the MT credentials.
const (
	translateEngineField = "engine"
	translateEngineLLM   = "llm"
	translateEngineMT    = "mt"
)

// mtProviderExtraFields lists the credential fields each MT provider needs
// beyond the shared apiKey. Providers absent here (deepl, modernmt) contribute
// only a selector option — their config is the common apiKey/model already on
// the base schema.
var mtProviderExtraFields = map[string][]string{
	"microsoft": {"subscriptionKey", "region"},
	"google":    {"projectId"},
	"mymemory":  {"email"},
}

// translateCommonSchema is the translate group's shared config: the engine
// selector, the provider selector (its options cascade off the engine), and the
// apiKey/model/batch options common to every provider.
func translateCommonSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&AITranslateConfig{}, schema.ToolMeta{
		ID:                    "translate",
		Category:              schema.CategoryTranslation,
		DisplayName:           "Translate",
		Description:           "Translate content with an LLM or machine-translation provider (select an engine, then a provider)",
		Tags:                  []string{"translation", schema.TagL10n},
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresTargetLanguage, schema.RequiresCredentials},
		Cardinality:           schema.Bilingual,
		Produces:              []schema.IOPort{{Type: schema.PortTarget, Side: model.SideTarget}},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	})

	// Engine selector (the group discriminator).
	engineOrder := 0
	s.Properties[translateEngineField] = schema.PropertySchema{
		Type:        "string",
		Title:       "Engine",
		Description: "Translation engine: an AI model or a machine-translation provider",
		Default:     translateEngineLLM,
		Widget:      "select",
		Order:       &engineOrder,
		Options: []schema.OptionItem{
			{Value: translateEngineLLM, Label: "AI (LLM)"},
			{Value: translateEngineMT, Label: "Machine translation"},
		},
	}

	// Provider selector: the flat union of all providers (for CLI/docs), plus
	// cascading option-sets so the UI offers only the providers for the selected
	// engine.
	if p, ok := s.Properties["provider"]; ok {
		p.Widget = "select"
		p.Options = allTranslateProviders()
		p.OptionSets = []schema.ConditionalOptions{
			{When: &schema.ConditionExpr{Field: translateEngineField, Eq: translateEngineLLM}, Options: aiProviderOptions()},
			{When: &schema.ConditionExpr{Field: translateEngineField, Eq: translateEngineMT}, Options: mtProviderOptions()},
		}
		s.Properties["provider"] = p
	}

	// Put the engine selector at the front of the provider group so the member
	// sections (MT credentials) insert directly beneath the selectors.
	for i := range s.Groups {
		if slices.Contains(s.Groups[i].Fields, "provider") {
			s.Groups[i].Fields = append([]string{translateEngineField}, s.Groups[i].Fields...)
			break
		}
	}
	return s
}

// translateMembers are the two engines. The LLM engine adds no fields beyond the
// common apiKey/model; the MT engine carries every MT provider's extra
// credentials, each gated on its provider so only the selected provider's fields
// show.
func translateMembers() []registry.ToolGroupMember {
	mt := schema.FromStruct(&mttools.MTTranslateConfig{}, schema.ToolMeta{ID: "translate-mt"})
	provs := make([]string, 0, len(mtProviderExtraFields))
	for prov := range mtProviderExtraFields {
		provs = append(provs, prov)
	}
	sort.Strings(provs)

	props := make(map[string]schema.PropertySchema)
	order := 0
	for _, prov := range provs {
		gate := &schema.ConditionExpr{Field: "provider", Eq: prov}
		for _, f := range mtProviderExtraFields[prov] {
			p := mt.Properties[f]
			order += 10
			ord := order
			p.Order = &ord
			p.Visible = gate
			props[f] = p
		}
	}

	return []registry.ToolGroupMember{
		{Name: translateEngineLLM, Label: "AI (LLM)", Description: "Translate with a large language model."},
		{
			Name:        translateEngineMT,
			Label:       "Machine translation",
			Description: "Translate with a dedicated machine-translation provider.",
			Schema:      &schema.ComponentSchema{Type: "object", Properties: props},
		},
	}
}

// translateGroup is the translate tool group: an engine discriminator (LLM / MT,
// LLM default) whose provider list cascades off the engine, with each MT
// provider's extra credentials shown only when that provider is selected.
func translateGroup() registry.ToolGroupDef {
	return registry.ToolGroupDef{
		Name:          "translate",
		Discriminator: translateEngineField,
		Default:       translateEngineLLM,
		Common:        translateCommonSchema(),
		Members:       translateMembers(),
		ConfigFactory: NewTranslateFromConfig,
		Resolver:      ResolveAIEgressContract,
	}
}

// TranslateSchema returns the composed (flat) projection of the translate group.
func TranslateSchema() *schema.ComponentSchema {
	return registry.ComposeGroupSchema(translateGroup())
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
		Tags:                  []string{"quality", schema.TagL10n},
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresTargetLanguage, schema.RequiresCredentials},
		Cardinality:           schema.Bilingual,
		Consumes:              []schema.IOPort{schema.Port(schema.PortTarget, model.SideTarget)},
		Produces:              []schema.IOPort{schema.Port(model.OverlayQA, model.SideTarget)},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	}
}

// qaCommonSchema is the qa group's shared config: just the mode selector.
func qaCommonSchema() *schema.ComponentSchema {
	meta := qaToolMeta()
	return &schema.ComponentSchema{
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
}

// qaMembers are the two QA backends: deterministic rules and an LLM judge.
func qaMembers() []registry.ToolGroupMember {
	rules := schema.FromStruct(libtools.NewQACheckConfig(model.LocaleEnglish), schema.ToolMeta{ID: "qa-rules"})
	ai := schema.FromStruct(&AIQAConfig{}, schema.ToolMeta{ID: "qa-ai"})
	setProviderOptions(ai, aiProviderOptions())
	return []registry.ToolGroupMember{
		{Name: qaModeRules, Label: "Deterministic rules", Description: "Local rule-based checks — no credentials, no network.", Schema: rules},
		{Name: qaModeAI, Label: "AI review", Description: "LLM-judged quality review via an AI provider.", Schema: ai},
	}
}

// qaGroup is the qa tool group: a `mode` selector (rules / AI), rules as the
// default so qa needs no credentials unless AI is selected.
func qaGroup() registry.ToolGroupDef {
	return registry.ToolGroupDef{
		Name:          "qa",
		Discriminator: qaModeField,
		Default:       qaModeRules,
		Common:        qaCommonSchema(),
		Members:       qaMembers(),
		ConfigFactory: NewQAFromConfig,
		Resolver:      ResolveQAContract,
	}
}

// QASchema returns the composed (flat) projection of the qa group.
func QASchema() *schema.ComponentSchema {
	return registry.ComposeGroupSchema(qaGroup())
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

// mtProviderOptions lists every registered MT provider as enum options.
func mtProviderOptions() []schema.OptionItem {
	opts := make([]schema.OptionItem, 0, len(mttools.Providers))
	for _, p := range mttools.Providers {
		opts = append(opts, schema.OptionItem{Value: string(p.ID), Label: p.Label})
	}
	return opts
}

// allTranslateProviders lists every LLM provider followed by every MT engine —
// the flat union used by CLI flags and docs (the UI uses the engine-cascading
// option-sets instead).
func allTranslateProviders() []schema.OptionItem {
	return append(aiProviderOptions(), mtProviderOptions()...)
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
