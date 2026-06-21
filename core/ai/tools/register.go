package tools

import (
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// RegisterAll registers all AI-powered tools in the given ToolRegistry.
// These tools require an LLM provider at runtime (API keys, etc.).
// The registry receives metadata and config factories; actual provider
// injection happens at tool creation time via NewToolFromConfig.
func RegisterAll(reg *registry.ToolRegistry) {
	// Translate — a tool group whose members are the LLM and MT providers;
	// `--provider` selects the backend (see unified.go). Replaces the former
	// ai-translate command and the per-engine <provider>-translate commands.
	reg.RegisterGroup(translateGroup())

	// QA — a tool group with two members: deterministic rule checks (default,
	// no credentials) and LLM-judged review; `--mode` selects (see unified.go).
	reg.RegisterGroup(qaGroup())

	// AI Review
	reg.RegisterWithSchema("ai-review", func() tool.Tool {
		return NewAIReviewTool(aiprovider.NewMockProvider(), AIReviewConfig{})
	}, AIReviewSchema())
	reg.SetConfigFactory("ai-review", NewAIReviewFromConfig)

	// AI Brand Voice Check — score/flag text against a brand voice profile.
	reg.RegisterWithSchema("brand-voice-check", func() tool.Tool {
		return NewBrandVoiceCheckTool(aiprovider.NewMockProvider(), nil)
	}, BrandVoiceCheckSchema())
	reg.SetConfigFactory("brand-voice-check", NewBrandVoiceCheckFromConfig)

	// AI Terminology — extract candidate terminology from content.
	reg.RegisterWithSchema("ai-terminology", func() tool.Tool {
		return NewAITerminologyTool(aiprovider.NewMockProvider(), AITerminologyConfig{})
	}, AITerminologySchema())
	reg.SetConfigFactory("ai-terminology", NewAITerminologyFromConfig)

	// AI Entity Extract — a tool group whose engine members are llm / local NER /
	// hybrid (feeds redaction's "entities" detector and terminology workflows).
	reg.RegisterGroup(entityExtractGroup())

	// Media Refine — re-read low-confidence OCR/ASR lines with a multimodal LLM.
	reg.RegisterWithSchema("media-refine", func() tool.Tool {
		return NewMediaRefineTool(aiprovider.NewMockProvider(), MediaRefineConfig{})
	}, MediaRefineSchema())
	reg.SetConfigFactory("media-refine", NewMediaRefineFromConfig)

	// Every AI tool's remote-source-egress side effect is config-dependent: a
	// local provider (Ollama, the offline demo) keeps content on the machine.
	// The tool groups (translate, qa, ai-entity-extract) carry their own
	// resolver in the group def; the remaining single-backend AI tools share
	// ResolveAIEgressContract.
	for _, name := range []registry.ToolID{
		"ai-review", "brand-voice-check",
		"ai-terminology", "media-refine",
	} {
		reg.SetContractResolver(name, ResolveAIEgressContract)
	}
}

// ResolveEntityExtractContract refines ai-entity-extract's contract from its
// config: with `engine: ner` the tool calls no provider at all — extraction
// runs on the local on-device NER model — so it carries no API call, no
// remote source egress, and needs no credentials. Any other engine resolves
// like the rest of the AI tools (local providers drop the egress effect).
func ResolveEntityExtractContract(config map[string]any, base registry.ToolInfo) registry.ToolInfo {
	engine, _ := config["engine"].(string)
	if engine != EngineNER {
		return ResolveAIEgressContract(config, base)
	}
	effects := make([]schema.SideEffect, 0, len(base.SideEffects))
	for _, e := range base.SideEffects {
		if e == schema.SideEffectRemoteSourceEgress || e == schema.SideEffectAPICall {
			continue
		}
		effects = append(effects, e)
	}
	base.SideEffects = effects
	requires := make([]string, 0, len(base.Requires))
	for _, r := range base.Requires {
		if r == schema.RequiresCredentials {
			continue
		}
		requires = append(requires, r)
	}
	base.Requires = requires
	return base
}

// ResolveAIEgressContract refines an AI tool's side effects from its node
// config (AD-006 placement pass): configured with a local provider (Ollama,
// the offline demo) the tool sends nothing to a remote sink, so the
// remote-source-egress effect is dropped. With no provider key — or any cloud
// provider — the static contract stands, fail-closed: the registered default
// for AI tools is a cloud provider, so an unconfigured node counts as remote.
// Registered via ToolRegistry.SetContractResolver for every AI tool.
func ResolveAIEgressContract(config map[string]any, base registry.ToolInfo) registry.ToolInfo {
	provider, _ := config["provider"].(string)
	if provider == "" || !aiprovider.IsLocalProvider(aiprovider.ProviderID(provider)) {
		return base
	}
	effects := make([]schema.SideEffect, 0, len(base.SideEffects))
	for _, e := range base.SideEffects {
		if e == schema.SideEffectRemoteSourceEgress {
			continue
		}
		effects = append(effects, e)
	}
	base.SideEffects = effects
	return base
}
