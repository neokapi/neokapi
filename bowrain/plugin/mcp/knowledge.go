package bowrainmcp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/connector"
)

// This file adds MCP tools that read the workspace brand knowledge graph
// (Bowrain AD-021) so an AI assistant can consult governed concepts, a
// concept's timeline, and change-set status/blast-radius. Each handler resolves
// the project + workspace-scoped client exactly like the sync MCP tools, via
// connector.NewKnowledgeClient.

// knowledgeClient discovers the kapi project and builds a workspace-scoped
// Bowrain client for the knowledge-graph MCP tools.
func knowledgeClient() (*apiclient.BowrainClient, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, err
	}
	return connector.NewKnowledgeClient(proj)
}

// --- concept_search ---

type MCPConceptSearchInput struct {
	Query  string `json:"query,omitempty" jsonschema:"Free-text query against the term text"`
	Status string `json:"status,omitempty" jsonschema:"Filter by term lifecycle status (preferred, admitted, deprecated, forbidden)"`
	Market string `json:"market,omitempty" jsonschema:"Filter by market validity tag"`
	Domain string `json:"domain,omitempty" jsonschema:"Filter by subject-field domain"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of concepts to return (default 50)"`
}

type MCPConceptTerm struct {
	Text   string `json:"text"`
	Locale string `json:"locale"`
	Status string `json:"status,omitempty"`
}

type MCPConceptMatch struct {
	ID         string           `json:"id"`
	Domain     string           `json:"domain,omitempty"`
	Definition string           `json:"definition,omitempty"`
	Terms      []MCPConceptTerm `json:"terms,omitempty"`
}

type MCPConceptSearchOutput struct {
	Concepts   []MCPConceptMatch `json:"concepts"`
	TotalCount int               `json:"total_count"`
}

func handleConceptSearch(ctx context.Context, input MCPConceptSearchInput) (*mcp.CallToolResult, MCPConceptSearchOutput, error) {
	client, err := knowledgeClient()
	if err != nil {
		return nil, MCPConceptSearchOutput{}, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	result, err := client.ListConcepts(ctx, apiclient.ListConceptsParams{
		Query:  input.Query,
		Status: input.Status,
		Market: input.Market,
		Domain: input.Domain,
		Limit:  limit,
	})
	if err != nil {
		return nil, MCPConceptSearchOutput{}, err
	}

	out := MCPConceptSearchOutput{TotalCount: result.TotalCount}
	for _, c := range result.Concepts {
		match := MCPConceptMatch{ID: c.ID, Domain: c.Domain, Definition: c.Definition}
		for _, t := range c.Terms {
			match.Terms = append(match.Terms, MCPConceptTerm{Text: t.Text, Locale: t.Locale, Status: t.Status})
		}
		out.Concepts = append(out.Concepts, match)
	}
	return nil, out, nil
}

// --- concept_story ---

type MCPConceptStoryInput struct {
	ConceptID string `json:"concept_id" jsonschema:"The concept ID whose timeline to fetch"`
}

type MCPConceptStoryEntry struct {
	Kind    string    `json:"kind"`
	At      time.Time `json:"at"`
	Actor   string    `json:"actor,omitempty"`
	Summary string    `json:"summary,omitempty"`
	Ref     string    `json:"ref,omitempty"`
}

type MCPConceptStoryOutput struct {
	ConceptID string                 `json:"concept_id"`
	Entries   []MCPConceptStoryEntry `json:"entries"`
}

func handleConceptStory(ctx context.Context, input MCPConceptStoryInput) (*mcp.CallToolResult, MCPConceptStoryOutput, error) {
	if strings.TrimSpace(input.ConceptID) == "" {
		return nil, MCPConceptStoryOutput{}, errors.New("concept_id is required")
	}

	client, err := knowledgeClient()
	if err != nil {
		return nil, MCPConceptStoryOutput{}, err
	}

	story, err := client.GetConceptStory(ctx, input.ConceptID)
	if err != nil {
		return nil, MCPConceptStoryOutput{}, err
	}

	out := MCPConceptStoryOutput{ConceptID: story.ConceptID}
	for _, e := range story.Entries {
		out.Entries = append(out.Entries, MCPConceptStoryEntry{
			Kind:    e.Kind,
			At:      e.At,
			Actor:   e.Actor,
			Summary: e.Summary,
			Ref:     e.Ref,
		})
	}
	return nil, out, nil
}

// --- experiment_status ---

type MCPExperimentStatusInput struct {
	ChangesetID string `json:"changeset_id,omitempty" jsonschema:"A change-set ID to detail; omit to list all change-sets"`
	Status      string `json:"status,omitempty" jsonschema:"When listing, filter by status (draft, in_review, approved, merged, abandoned)"`
}

type MCPExperimentEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Governed  bool   `json:"governed,omitempty"`
	CreatedBy string `json:"created_by,omitempty"`
}

type MCPBlastRadius struct {
	TotalBlocks    int `json:"total_blocks"`
	AffectedBlocks int `json:"affected_blocks"`
	NewViolations  int `json:"new_violations"`
	Resolved       int `json:"resolved"`
	Words          int `json:"words"`
}

type MCPExperimentStatusOutput struct {
	Experiments []MCPExperimentEntry `json:"experiments,omitempty"`
	Experiment  *MCPExperimentEntry  `json:"experiment,omitempty"`
	BlastRadius *MCPBlastRadius      `json:"blast_radius,omitempty"`
}

func handleExperimentStatus(ctx context.Context, input MCPExperimentStatusInput) (*mcp.CallToolResult, MCPExperimentStatusOutput, error) {
	client, err := knowledgeClient()
	if err != nil {
		return nil, MCPExperimentStatusOutput{}, err
	}

	// A specific change-set: return its detail plus a blast-radius summary.
	if input.ChangesetID != "" {
		detail, err := client.GetChangeset(ctx, input.ChangesetID)
		if err != nil {
			return nil, MCPExperimentStatusOutput{}, err
		}
		out := MCPExperimentStatusOutput{
			Experiment: &MCPExperimentEntry{
				ID:        detail.ID,
				Name:      detail.Name,
				Status:    detail.Status,
				Governed:  detail.Governed,
				CreatedBy: detail.CreatedBy,
			},
		}
		// Blast radius is a best-effort summary alongside the detail.
		if impact, brErr := client.GetChangesetBlastRadius(ctx, input.ChangesetID); brErr == nil {
			out.BlastRadius = &MCPBlastRadius{
				TotalBlocks:    impact.TotalBlocks,
				AffectedBlocks: impact.AffectedBlocks,
				NewViolations:  impact.NewViolations,
				Resolved:       impact.Resolved,
				Words:          impact.Words,
			}
		}
		return nil, out, nil
	}

	// No ID: list the workspace's change-sets.
	changesets, err := client.ListChangesets(ctx, input.Status)
	if err != nil {
		return nil, MCPExperimentStatusOutput{}, err
	}
	out := MCPExperimentStatusOutput{}
	for _, cs := range changesets {
		out.Experiments = append(out.Experiments, MCPExperimentEntry{
			ID:        cs.ID,
			Name:      cs.Name,
			Status:    cs.Status,
			CreatedBy: cs.CreatedBy,
		})
	}
	return nil, out, nil
}
