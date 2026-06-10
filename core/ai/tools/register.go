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
	// AI Translate
	reg.RegisterWithSchema("ai-translate", func() tool.Tool {
		return NewAITranslateTool(aiprovider.NewMockProvider(), AITranslateConfig{})
	}, AITranslateSchema())
	reg.SetConfigFactory("ai-translate", NewAITranslateFromConfig)

	// AI QA Check
	reg.RegisterWithSchema("ai-qa", func() tool.Tool {
		return NewAIQACheckTool(aiprovider.NewMockProvider(), AIQAConfig{})
	}, AIQASchema())
	reg.SetConfigFactory("ai-qa", NewAIQAFromConfig)

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

	// AI Entity Extract — detect named entities (feeds redaction's
	// "entities" detector and terminology workflows).
	reg.RegisterWithSchema("ai-entity-extract", func() tool.Tool {
		return NewAIEntityExtractTool(aiprovider.NewMockProvider(), nil, AIEntityExtractConfig{})
	}, AIEntityExtractSchema())
	reg.SetConfigFactory("ai-entity-extract", NewAIEntityExtractFromConfig)

	// Every AI tool's remote-source-egress side effect is config-dependent: a
	// local provider (Ollama, the offline demo) keeps content on the machine.
	for _, name := range []registry.ToolID{
		"ai-translate", "ai-qa", "ai-review", "brand-voice-check",
		"ai-terminology", "ai-entity-extract",
	} {
		reg.SetContractResolver(name, ResolveAIEgressContract)
	}
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
