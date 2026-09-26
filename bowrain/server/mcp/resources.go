package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// registerResources registers voice resource templates on the MCP server.
//
// Resource URIs:
//   - brand://profiles/{id}             — full voice profile
//   - brand://profiles/{id}/vocabulary  — word rules of the profile's workspace terms store
//   - brand://profiles/{id}/examples    — before/after pairs
//   - brand://terminology/{workspace}   — workspace terms
func (s *MCPServer) registerResources() {
	// Full voice profile by ID.
	s.server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			Name:        "voice_profile",
			Description: "Full voice profile including tone, style, patterns, and examples",
			URITemplate: "brand://profiles/{id}",
			MIMEType:    "application/json",
		},
		s.handleReadProfile,
	)

	// Vocabulary rules for a profile.
	s.server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			Name:        "brand_vocabulary",
			Description: "Word rules (forbidden and competitor terms with their replacements) of the terms store in the voice profile's workspace",
			URITemplate: "brand://profiles/{id}/vocabulary",
			MIMEType:    "application/json",
		},
		s.handleReadVocabulary,
	)

	// Before/after examples for a profile.
	s.server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			Name:        "brand_examples",
			Description: "Before/after transformation examples for a voice profile",
			URITemplate: "brand://profiles/{id}/examples",
			MIMEType:    "application/json",
		},
		s.handleReadExamples,
	)

	// Workspace terminology index.
	s.server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			Name:        "brand_terminology",
			Description: "Terminology index: the word-rule counts of a workspace's terms store and the voice profiles that apply them",
			URITemplate: "brand://terminology/{workspace}",
			MIMEType:    "application/json",
		},
		s.handleReadTerminology,
	)
}

func (s *MCPServer) handleReadProfile(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	profileID := extractParam(uri, "brand://profiles/")
	if profileID == "" {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	profile, err := s.voiceStore.GetProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	data, err := json.Marshal(profile)
	if err != nil {
		return nil, fmt.Errorf("marshal profile: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(data)}},
	}, nil
}

func (s *MCPServer) handleReadVocabulary(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	profileID := extractParamBefore(uri, "brand://profiles/", "/vocabulary")
	if profileID == "" {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	profile, err := s.voiceStore.GetProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	if profile == nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	rules, err := s.workspaceWordRules(ctx, profile.Scope, "")
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(struct {
		TermRules []coreprofile.TermRule `json:"term_rules"`
	}{rules})
	if err != nil {
		return nil, fmt.Errorf("marshal vocabulary: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(data)}},
	}, nil
}

func (s *MCPServer) handleReadExamples(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	profileID := extractParamBefore(uri, "brand://profiles/", "/examples")
	if profileID == "" {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	profile, err := s.voiceStore.GetProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	data, err := json.Marshal(profile.Examples)
	if err != nil {
		return nil, fmt.Errorf("marshal examples: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(data)}},
	}, nil
}

func (s *MCPServer) handleReadTerminology(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	workspaceID := extractParam(uri, "brand://terminology/")
	if workspaceID == "" {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	profiles, err := s.voiceStore.ListProfiles(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	rules, err := s.workspaceWordRules(ctx, workspaceID, "")
	if err != nil {
		return nil, err
	}
	type profileEntry struct {
		ProfileID   string `json:"profile_id"`
		ProfileName string `json:"profile_name"`
	}
	index := struct {
		WorkspaceID string         `json:"workspace_id"`
		Preferred   int            `json:"preferred_terms"`
		Forbidden   int            `json:"forbidden_terms"`
		Competitor  int            `json:"competitor_terms"`
		Profiles    []profileEntry `json:"profiles"`
	}{WorkspaceID: workspaceID, Profiles: []profileEntry{}}
	index.Preferred, index.Forbidden, index.Competitor = voicescope.WordRuleCounts(rules)
	for _, p := range profiles {
		index.Profiles = append(index.Profiles, profileEntry{ProfileID: p.ID, ProfileName: p.Name})
	}
	data, err := json.Marshal(index)
	if err != nil {
		return nil, fmt.Errorf("marshal terminology: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(data)}},
	}, nil
}
