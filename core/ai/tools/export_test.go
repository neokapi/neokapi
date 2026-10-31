package tools

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// ExportCacheFingerprint exposes the session cache key for tests in the
// external test package. It is the one piece of this tool that must be
// asserted from outside and cannot be observed through a prompt.
func ExportCacheFingerprint(t *AITranslateTool, ctx context.Context, b *model.Block) string {
	return t.cacheFingerprint(ctx, b)
}

// ToolProvider exposes the LLM provider an AI tool was built with, so the
// external tests can assert which one a config factory chose. Every tool
// calls its provider through the redaction wrapper, which this looks past.
// It answers nil for a tool that holds none (an MT engine, the deterministic
// check, the local NER model).
func ToolProvider(t any) aiprovider.LLMProvider {
	var p aiprovider.LLMProvider
	switch v := t.(type) {
	case *AITranslateTool:
		p = v.provider
	case *AIReviewTool:
		p = v.provider
	case *AICheckTool:
		p = v.provider
	case *VoiceCheckTool:
		p = v.provider
	case *VoiceInferTool:
		p = v.provider
	case *AITerminologyTool:
		p = v.provider
	case *MediaRefineTool:
		p = v.provider
	case *AIEntityExtractTool:
		p = v.llm
	default:
		return nil
	}
	if p == nil {
		return nil
	}
	return aiprovider.WithoutRedaction(p)
}
