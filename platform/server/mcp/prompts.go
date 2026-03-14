package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerPrompts registers brand voice prompt templates on the MCP server.
//
// Prompts:
//   - write_in_voice   — write content in brand voice
//   - rewrite_in_voice — rewrite text to match brand voice
//   - check_draft      — check draft against guidelines
func (s *MCPServer) registerPrompts() {
	// write_in_voice — write new content in a brand voice.
	s.server.AddPrompt(
		&mcp.Prompt{
			Name:        "write_in_voice",
			Description: "Write new content following a brand voice profile's guidelines.",
			Arguments: []*mcp.PromptArgument{
				{Name: "profile_id", Description: "the brand voice profile ID", Required: true},
				{Name: "topic", Description: "the topic or subject to write about", Required: true},
				{Name: "content_type", Description: "the type of content (e.g., blog post, email, social media)", Required: false},
				{Name: "locale", Description: "target locale for locale-specific voice adjustments", Required: false},
			},
		},
		s.handleWriteInVoice,
	)

	// rewrite_in_voice — rewrite existing text to match a brand voice.
	s.server.AddPrompt(
		&mcp.Prompt{
			Name:        "rewrite_in_voice",
			Description: "Rewrite existing text to match a brand voice profile's tone, style, and vocabulary.",
			Arguments: []*mcp.PromptArgument{
				{Name: "profile_id", Description: "the brand voice profile ID", Required: true},
				{Name: "text", Description: "the text to rewrite", Required: true},
				{Name: "locale", Description: "target locale for locale-specific voice adjustments", Required: false},
				{Name: "channel", Description: "content channel for channel-specific adjustments", Required: false},
			},
		},
		s.handleRewriteInVoicePrompt,
	)

	// check_draft — check a draft against brand voice guidelines.
	s.server.AddPrompt(
		&mcp.Prompt{
			Name:        "check_draft",
			Description: "Check a draft against brand voice guidelines and suggest improvements.",
			Arguments: []*mcp.PromptArgument{
				{Name: "profile_id", Description: "the brand voice profile ID", Required: true},
				{Name: "draft", Description: "the draft text to check", Required: true},
				{Name: "locale", Description: "target locale for locale-specific checks", Required: false},
			},
		},
		s.handleCheckDraft,
	)
}

func (s *MCPServer) handleWriteInVoice(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	profileID := req.Params.Arguments["profile_id"]
	topic := req.Params.Arguments["topic"]
	contentType := req.Params.Arguments["content_type"]
	locale := req.Params.Arguments["locale"]

	profile, err := s.brandStore.GetProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}

	resolved := resolveProfile(profile, locale, "")
	guide := formatVoiceGuide(resolved)

	contentTypeStr := "content"
	if contentType != "" {
		contentTypeStr = contentType
	}

	systemPrompt := fmt.Sprintf("You are a brand voice writer. Follow the brand voice guide below exactly when writing content.\n\n%s", guide)
	userPrompt := fmt.Sprintf("Write a %s about: %s\n\nFollow the brand voice guide precisely. Use the preferred vocabulary, avoid forbidden and competitor terms, and match the specified tone and style.", contentTypeStr, topic)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Write %s in the %q brand voice", contentTypeStr, profile.Name),
		Messages: []*mcp.PromptMessage{
			{Role: "assistant", Content: &mcp.TextContent{Text: systemPrompt}},
			{Role: "user", Content: &mcp.TextContent{Text: userPrompt}},
		},
	}, nil
}

func (s *MCPServer) handleRewriteInVoicePrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	profileID := req.Params.Arguments["profile_id"]
	text := req.Params.Arguments["text"]
	locale := req.Params.Arguments["locale"]
	channel := req.Params.Arguments["channel"]

	profile, err := s.brandStore.GetProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}

	resolved := resolveProfile(profile, locale, channel)
	guide := formatVoiceGuide(resolved)

	systemPrompt := fmt.Sprintf("You are a brand voice editor. Rewrite text to match the brand voice guide below. Preserve the original meaning while adjusting tone, style, and vocabulary.\n\n%s", guide)
	userPrompt := fmt.Sprintf("Rewrite the following text to match the brand voice:\n\n%s\n\nProvide the rewritten text and a brief summary of changes made.", text)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Rewrite text in the %q brand voice", profile.Name),
		Messages: []*mcp.PromptMessage{
			{Role: "assistant", Content: &mcp.TextContent{Text: systemPrompt}},
			{Role: "user", Content: &mcp.TextContent{Text: userPrompt}},
		},
	}, nil
}

func (s *MCPServer) handleCheckDraft(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	profileID := req.Params.Arguments["profile_id"]
	draft := req.Params.Arguments["draft"]
	locale := req.Params.Arguments["locale"]

	profile, err := s.brandStore.GetProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}

	resolved := resolveProfile(profile, locale, "")
	guide := formatVoiceGuide(resolved)

	systemPrompt := fmt.Sprintf("You are a brand voice reviewer. Evaluate text against the brand voice guide below. Check for tone consistency, style rule compliance, vocabulary violations, and overall brand alignment.\n\n%s", guide)
	userPrompt := fmt.Sprintf("Review the following draft against the brand voice guidelines:\n\n%s\n\nProvide:\n1. An overall compliance assessment (score 0-100)\n2. Specific findings grouped by dimension (tone, style, vocabulary, clarity, brand_compliance)\n3. Suggested corrections for each finding\n4. A revised version if changes are needed", draft)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Check draft against %q brand voice", profile.Name),
		Messages: []*mcp.PromptMessage{
			{Role: "assistant", Content: &mcp.TextContent{Text: systemPrompt}},
			{Role: "user", Content: &mcp.TextContent{Text: userPrompt}},
		},
	}, nil
}
