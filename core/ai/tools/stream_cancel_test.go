package tools_test

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/require"
)

// The tools that buffer their whole input still stop when the flow is
// cancelled while upstream has sent nothing and kept its channel open.
func TestBufferingToolsStopOnCancelWithIdleInput(t *testing.T) {
	cases := map[string]func() tool.Tool{
		"translate-batched": func() tool.Tool {
			return tools.NewAITranslateTool(nil, tools.AITranslateConfig{Provider: "demo", BatchSize: 8})
		},
		"entity-extract-batched": func() tool.Tool {
			return tools.NewAIEntityExtractTool(nil, nil, tools.AIEntityExtractConfig{BatchSize: 4})
		},
		"media-refine": func() tool.Tool {
			return tools.NewMediaRefineTool(nil, tools.MediaRefineConfig{})
		},
		"voice-infer": func() tool.Tool {
			return tools.NewVoiceInferTool(nil, tools.VoiceInferConfig{})
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				in := make(chan *model.Part)
				out := make(chan *model.Part, 1)
				done := make(chan error, 1)
				go func() { done <- build().Process(ctx, in, out) }()
				synctest.Wait()
				cancel()
				require.ErrorIs(t, <-done, context.Canceled)
			})
		})
	}
}
