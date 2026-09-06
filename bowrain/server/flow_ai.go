package server

import (
	"context"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// flowAIProvider builds the LLM provider the AI steps of a flow call, for the
// workspace whose content the flow is running over.
//
// It resolves the same upstream the interactive editor and the queued worker
// jobs resolve: the instance's platform AI backend (Bedrock on the hosted
// cloud, called with the task role and no API key), with the workspace's own
// model choice applied when the admin has opened model choice to customers.
// One producer, so a translate step of a flow reaches the model a translation
// started from the editor reaches.
//
// A self-hosted instance with no platform provider configured yields no
// provider and no error, which leaves each step to the provider its own config
// names.
func (s *Server) flowAIProvider(ctx context.Context, workspaceID, requestedModel string) (aiprovider.LLMProvider, error) {
	cfg := s.platformProviderConfigForWorkspace(ctx, workspaceID)
	if cfg.Provider == "" {
		return nil, nil
	}
	prov, _, err := cfg.Build(requestedModel)
	if err != nil {
		return nil, err
	}
	return prov, nil
}

// wireFlowAIProvider grants the flow runner the platform's AI provider and the
// accounting that goes with it, so a flow's translate or review step calls the
// model the workspace is entitled to and spends what it burns. Called after
// the final Services instance exists, because wrapping the content store for
// event emission rebuilds it.
func (s *Server) wireFlowAIProvider() {
	if s.Services == nil || s.Services.Flow == nil {
		return
	}
	s.Services.Flow.SetAIProviderResolver(s.flowAIProvider)
	s.Services.Flow.SetAIAccountant(flowAIAccountant{srv: s})
}
