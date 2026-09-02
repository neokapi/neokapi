package bowrainmcp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/neokapi/neokapi/host/venue/source"
)

// This file adds MCP tools that read the workspace brand knowledge graph
// (Bowrain AD-021) so an AI assistant can consult governed concepts, a
// concept's timeline, and change-set status/blast-radius. Each handler resolves
// the project + workspace-scoped client exactly like the sync MCP tools, via
// source.NewKnowledgeClient.

// knowledgeClient discovers the kapi project and builds a workspace-scoped
// Bowrain client for the knowledge-graph MCP tools.
//
// The project travels back alongside the client because a tool's result is read
// by an assistant reporting to a person, and a change-set id on its own gives
// that person nothing to open. The recipe's server and workspace are what turn
// the id into a review link, and they live on the project, not the client.
func knowledgeClient() (*project.Project, *apiclient.BowrainClient, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, nil, err
	}
	client, err := source.NewKnowledgeClient(proj)
	if err != nil {
		return nil, nil, err
	}
	return proj, client, nil
}

// changesetReviewURL is the deep link to a change-set's review surface in the
// web hub: <server>/<workspace>/context/changes/<id>, the one route the app
// serves for it.
//
// Empty when the recipe names no server or no workspace. A surface that cannot
// build the link reports the id alone rather than a link assembled from a
// guessed host: a wrong link is worse than none, because it reads as a page
// that has gone missing rather than as a hub the caller has not connected to.
func changesetReviewURL(proj *project.Project, changesetID string) string {
	if proj == nil || proj.Recipe == nil || proj.Recipe.Server == nil || changesetID == "" {
		return ""
	}
	server := strings.TrimRight(proj.Recipe.Server.ServerURL(), "/")
	workspace := proj.Recipe.Server.Workspace()
	if server == "" || workspace == "" {
		return ""
	}
	return server + "/" + workspace + "/context/changes/" + changesetID
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
	_, client, err := knowledgeClient()
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

	_, client, err := knowledgeClient()
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
	// ReviewURL is the deep link to this change-set's review surface. Omitted
	// when the recipe names no server or workspace to build one from, so a
	// consumer never receives a link that resolves nowhere.
	ReviewURL string `json:"review_url,omitempty"`
}

type MCPBlastRadius struct {
	TotalBlocks    int `json:"total_blocks"`
	AffectedBlocks int `json:"affected_blocks"`
	NewViolations  int `json:"new_violations"`
	Resolved       int `json:"resolved"`
	Words          int `json:"words"`

	// Partial says the server's walk ran out of time before scanning every
	// block, so the counts are lower bounds. It is carried into the tool output
	// because the consumer here is an assistant summarising the change for
	// someone deciding whether to approve it, and "affects 900 blocks" read off
	// a truncated scan is a smaller number than the truth with nothing marking
	// it as such.
	//
	// "Lower bound" understates it, which is why CountsAre says more than the
	// boolean. The server's walk is one sequential pass — projects, then
	// streams, then blocks — and the budget aborts it from inside the innermost
	// loop, so a project it never reached contributes NOTHING: the shortfall is
	// whole projects missing, not every project counted a little low. This
	// summary carries no per-project breakdown (deliberately — see the handler),
	// so there is no list here to mislead; the qualification exists so the
	// scalars are not read as a survey of the whole workspace.
	Partial bool `json:"partial,omitempty"`
	// CountsAre spells the qualification out in words rather than leaving a
	// bare boolean for a reader to interpret.
	CountsAre string `json:"counts_are,omitempty"`
}

type MCPExperimentStatusOutput struct {
	Experiments []MCPExperimentEntry `json:"experiments,omitempty"`
	Experiment  *MCPExperimentEntry  `json:"experiment,omitempty"`
	BlastRadius *MCPBlastRadius      `json:"blast_radius,omitempty"`

	// BlastRadiusError says why the blast radius is absent. Without it, a
	// failed radius call and a change-set that touches nothing produce the
	// identical output — no blast_radius field — and the assistant reading it
	// reports "no impact" for a walk that never completed.
	BlastRadiusError string `json:"blast_radius_error,omitempty"`
}

func handleExperimentStatus(ctx context.Context, input MCPExperimentStatusInput) (*mcp.CallToolResult, MCPExperimentStatusOutput, error) {
	proj, client, err := knowledgeClient()
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
				ReviewURL: changesetReviewURL(proj, detail.ID),
			},
		}
		// Blast radius is a best-effort summary alongside the detail — the
		// change-set detail is still worth returning without it. But
		// best-effort is not silent. A dropped failure makes an absent
		// blast_radius mean either "this change touches nothing" or "we could
		// not find out", with no way to tell which — and the assistant
		// consuming this reports the first reading, the reassuring one.
		impact, brErr := client.GetChangesetBlastRadius(ctx, input.ChangesetID)
		switch {
		case brErr != nil:
			out.BlastRadiusError = brErr.Error()
		default:
			out.BlastRadius = &MCPBlastRadius{
				TotalBlocks:    impact.TotalBlocks,
				AffectedBlocks: impact.AffectedBlocks,
				NewViolations:  impact.NewViolations,
				Resolved:       impact.Resolved,
				Words:          impact.Words,
				Partial:        impact.Partial,
			}
			// The impact's per-project breakdown (impact.Projects) is
			// deliberately NOT carried into this summary, and under a partial
			// walk that omission is load-bearing rather than incidental: the
			// server's pass is sequential and aborts from the innermost loop,
			// so projects it never reached are absent from that list entirely.
			// Rendering it would read as "these are the projects affected"
			// when the truth is "these are the projects examined" — an
			// assistant naming two projects it never looked past is worse than
			// one that names none.
			// Consequence in this surface's voice, cause in the server's
			// field. PartialReason states only why the walk stopped, so this
			// sentence must not restate that: it says what the numbers mean,
			// which is the part the reader needs.
			if impact.Partial {
				out.BlastRadius.CountsAre = "lower bounds: any project the scan did not reach contributes nothing to these totals"
				if impact.PartialReason != "" {
					out.BlastRadius.CountsAre += " (" + impact.PartialReason + ")"
				}
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
			ReviewURL: changesetReviewURL(proj, cs.ID),
		})
	}
	return nil, out, nil
}
