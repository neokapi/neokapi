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
// external tests can assert which one a config factory chose. It answers nil
// for a tool that holds none (an MT engine, the deterministic check, the local
// NER model).
func ToolProvider(t any) aiprovider.LLMProvider {
	switch v := t.(type) {
	case *AITranslateTool:
		return v.provider
	case *AIReviewTool:
		return v.provider
	case *AICheckTool:
		return v.provider
	case *VoiceCheckTool:
		return v.provider
	case *VoiceInferTool:
		return v.provider
	case *AITerminologyTool:
		return v.provider
	case *MediaRefineTool:
		return v.provider
	case *AIEntityExtractTool:
		return v.llm
	default:
		return nil
	}
}
