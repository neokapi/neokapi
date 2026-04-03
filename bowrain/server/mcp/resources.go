package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerResources registers brand voice resource templates on the MCP server.
//
// Resource URIs:
//   - brand://profiles/{id}             — full voice profile
//   - brand://profiles/{id}/vocabulary  — preferred/forbidden/competitor terms
//   - brand://profiles/{id}/examples    — before/after pairs
//   - brand://terminology/{workspace}   — workspace termbase
func (s *MCPServer) registerResources() {
	// Full voice profile by ID.
	s.server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			Name:        "brand_profile",
			Description: "Full brand voice profile including tone, style, vocabulary, and examples",
			URITemplate: "brand://profiles/{id}",
			MIMEType:    "application/json",
		},
		s.handleReadProfile,
	)

	// Vocabulary rules for a profile.
	s.server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			Name:        "brand_vocabulary",
			Description: "Vocabulary rules (preferred, forbidden, competitor terms) for a brand voice profile",
			URITemplate: "brand://profiles/{id}/vocabulary",
			MIMEType:    "application/json",
		},
		s.handleReadVocabulary,
	)

	// Before/after examples for a profile.
	s.server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			Name:        "brand_examples",
			Description: "Before/after transformation examples for a brand voice profile",
			URITemplate: "brand://profiles/{id}/examples",
			MIMEType:    "application/json",
		},
		s.handleReadExamples,
	)

	// Workspace terminology index.
	s.server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			Name:        "brand_terminology",
			Description: "Terminology index listing all brand voice profiles and their term counts in a workspace",
			URITemplate: "brand://terminology/{workspace}",
			MIMEType:    "application/json",
		},
		s.handleReadTerminology,
	)
}

func (s *MCPServer) handleReadProfile(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	profileID := extractParam(uri, "brand://profiles/")
	if profileID == "" {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	profile, err := s.brandStore.GetProfile(bgCtx(), profileID)
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

func (s *MCPServer) handleReadVocabulary(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	profileID := extractParamBefore(uri, "brand://profiles/", "/vocabulary")
	if profileID == "" {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	profile, err := s.brandStore.GetProfile(bgCtx(), profileID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	data, err := json.Marshal(profile.Vocabulary)
	if err != nil {
		return nil, fmt.Errorf("marshal vocabulary: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(data)}},
	}, nil
}

func (s *MCPServer) handleReadExamples(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	profileID := extractParamBefore(uri, "brand://profiles/", "/examples")
	if profileID == "" {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	profile, err := s.brandStore.GetProfile(bgCtx(), profileID)
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

func (s *MCPServer) handleReadTerminology(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	workspaceID := extractParam(uri, "brand://terminology/")
	if workspaceID == "" {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	profiles, err := s.brandStore.ListProfiles(bgCtx(), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	type termEntry struct {
		ProfileID   string `json:"profile_id"`
		ProfileName string `json:"profile_name"`
		Preferred   int    `json:"preferred_terms"`
		Forbidden   int    `json:"forbidden_terms"`
		Competitor  int    `json:"competitor_terms"`
	}
	var entries []termEntry
	for _, p := range profiles {
		entries = append(entries, termEntry{
			ProfileID:   p.ID,
			ProfileName: p.Name,
			Preferred:   len(p.Vocabulary.PreferredTerms),
			Forbidden:   len(p.Vocabulary.ForbiddenTerms),
			Competitor:  len(p.Vocabulary.CompetitorTerms),
		})
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal terminology: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(data)}},
	}, nil
}
