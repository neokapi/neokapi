package tools

import (
	"github.com/neokapi/neokapi/core/registry"
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
}
