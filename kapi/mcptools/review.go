package mcptools

// MCP review tools: the review queue, one unit's full picture, and the
// pre-review an agent records. An agent reads what awaits a person and leaves
// a score with its reasons on a unit; it never records a decision. Only a
// person establishes a unit, through the desktop Review page, `kapi apply`, or
// a hosted review session, so an agent's judgement never counts as a
// person's.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/state"
)

func init() {
	cli.RegisterMCPToolFactory(registerReviewTools)
}

// registerReviewTools registers the review-workflow MCP tools.
func registerReviewTools(server *mcp.Server, a *cli.App) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "review_queue",
		Description: "List the review queue: every unit awaiting a person, addressed by (file, key, locale). One queue holds every language, the project's source language among them: a translated unit not yet approved is one row, and a source unit held below the project's translate_after level is another, marked `isSource`. The result also carries `languages`, the pending count per language. Filter with language, locale and/or collection. Read-only, derived from the content files and the project state store; units annotated by an AI pre-review carry their score. Lean by design: call review_unit for a unit's context (the point governing it, its neighbourhood, its prior version, its findings).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewQueueInput) (*mcp.CallToolResult, ReviewQueueOutput, error) {
		return handleReviewQueue(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:         "review_unit",
		Description:  "Fetch one review-queue unit's full picture: source and target text, ladder status, the last recorded state (with identity), and the context the decision is made in: the point governing the file (voice guidance, term rules, coordinates), the blocks before and after it as run sequences, the prior approved version and the content-memory match with its wording, the check findings with their run anchors, and the AI pre-review score. A unit in the project's source language is read the same way, from its source file, and returns its authoring rung with no target half. The read leg before pre_review_unit.",
		OutputSchema: reviewUnitOutputSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewUnitInput) (*mcp.CallToolResult, ReviewUnitOutput, error) {
		return handleReviewUnit(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pre_review_unit",
		Description: "Record your pre-review of one review-queue unit: a score from 0 to 100 and the reasons behind it. It is advisory and bound to the current translation, so an edit drops it. The person reviewing sees it in the queue; it never establishes the unit or sends it back.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PreReviewInput) (*mcp.CallToolResult, PreReviewOutput, error) {
		return handlePreReview(ctx, a, input, agentIdentity(req))
	})
}

// agentIdentity derives the identity a pre-review records: "agent/<name>" from
// the client's declared implementation name, or the bare "agent" when the
// session carries none.
func agentIdentity(req *mcp.CallToolRequest) string {
	if req != nil && req.Session != nil {
		if ip := req.Session.InitializeParams(); ip != nil && ip.ClientInfo != nil && ip.ClientInfo.Name != "" {
			return "agent/" + ip.ClientInfo.Name
		}
	}
	return "agent"
}

// resolveReviewProject resolves the target project recipe through the shared
// per-call seam: the explicit `project` input (a recipe, a project root, or any
// path inside one), else the project the MCP server started in, else the
// ambient project (KAPI_PROJECT / upward walk, honoring KAPI_NO_PROJECT).
func resolveReviewProject(a *cli.App, explicit string) (string, error) {
	return a.RequireMCPCallProject(explicit)
}

// --- Input/Output types ---

type ReviewQueueInput struct {
	Project    string `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
	Language   string `json:"language,omitempty" jsonschema:"Only list units in this language; the project's source language lists its source units"`
	Locale     string `json:"locale,omitempty" jsonschema:"Only list units for this target locale"`
	Collection string `json:"collection,omitempty" jsonschema:"Only list units in this content collection"`
}

type ReviewQueueOutput struct {
	Pending []cli.ReviewQueueItem `json:"pending"`
	Total   int                   `json:"total"`
	// Languages counts the pending units per language over the whole queue,
	// before any filter, so a caller can narrow to a language it knows has work.
	Languages []cli.ReviewLanguage `json:"languages,omitempty"`
}

type ReviewUnitInput struct {
	Project string `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
	Locale  string `json:"locale" jsonschema:"Language of the unit, as listed by review_queue: a target locale, or the project's source language for a source unit"`
	File    string `json:"file" jsonschema:"File path, as listed by review_queue: the target file for a translation, the source file for a source unit"`
	Key     string `json:"key" jsonschema:"Unit key, as listed by review_queue"`
}

type ReviewUnitOutput struct {
	Unit *cli.ReviewUnitInfo `json:"unit"`
}

// reviewUnitOutputSchema declares review_unit's result instead of letting the
// SDK infer it from the Go type.
//
// A block's runs nest: a plural run holds a run sequence per form, so model.Run
// refers to itself. The SDK's inference walks the type graph rather than
// emitting a $ref, and refuses a cycle, so a unit carrying its neighbourhood
// as runs cannot have a schema inferred at all. Declaring the envelope keeps
// the tool registrable and leaves the unit an object the client reads by name;
// the field documentation lives on host.ReviewUnitInfo and host.ReviewContext.
var reviewUnitOutputSchema = json.RawMessage(
	`{"type":"object","properties":{"unit":{"type":"object"}}}`)

type PreReviewInput struct {
	Project string            `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
	Locale  string            `json:"locale" jsonschema:"Target locale, as listed by review_queue"`
	File    string            `json:"file" jsonschema:"Target file path, as listed by review_queue"`
	Key     string            `json:"key" jsonschema:"Unit key, as listed by review_queue"`
	Score   int               `json:"score" jsonschema:"How well the translation stands, from 0 (unusable) to 100 (nothing to change)"`
	Reasons []PreReviewReason `json:"reasons,omitempty" jsonschema:"What you found, one entry per issue; empty when nothing needs saying"`
}

// PreReviewReason is one issue a pre-review found.
type PreReviewReason struct {
	Severity   string `json:"severity,omitempty" jsonschema:"critical, major, minor or info"`
	Message    string `json:"message" jsonschema:"What is wrong, in a sentence"`
	Suggestion string `json:"suggestion,omitempty" jsonschema:"Wording that would fix it"`
}

type PreReviewOutput struct {
	// Recorded is false when the unit is not in the queue as addressed.
	Recorded bool `json:"recorded"`
	// By is the identity the pre-review was recorded with ("agent/<client>").
	By string `json:"by"`
}

// --- Handlers ---

func handleReviewQueue(ctx context.Context, a *cli.App, input ReviewQueueInput) (*mcp.CallToolResult, ReviewQueueOutput, error) {
	projectPath, err := resolveReviewProject(a, input.Project)
	if err != nil {
		return nil, ReviewQueueOutput{}, err
	}
	var langs []string
	if input.Language != "" {
		langs = []string{input.Language}
	}
	queue, err := a.ReviewQueue(ctx, projectPath, "", cli.ReviewQueueOptions{Languages: langs})
	if err != nil {
		return nil, ReviewQueueOutput{}, fmt.Errorf("derive review queue: %w", err)
	}
	pending := make([]cli.ReviewQueueItem, 0, len(queue.Pending))
	for _, it := range queue.Pending {
		if input.Locale != "" && it.Locale != input.Locale {
			continue
		}
		if input.Collection != "" && it.Collection != input.Collection {
			continue
		}
		pending = append(pending, it)
	}
	return nil, ReviewQueueOutput{Pending: pending, Total: len(pending), Languages: queue.Languages}, nil
}

func handleReviewUnit(ctx context.Context, a *cli.App, input ReviewUnitInput) (*mcp.CallToolResult, ReviewUnitOutput, error) {
	projectPath, err := resolveReviewProject(a, input.Project)
	if err != nil {
		return nil, ReviewUnitOutput{}, err
	}
	info, err := a.ReviewUnitWithContext(ctx, projectPath, "", cli.ReviewUnitRef{
		File: input.File, Key: input.Key, Locale: input.Locale,
	})
	if err != nil {
		return nil, ReviewUnitOutput{}, err
	}
	return nil, ReviewUnitOutput{Unit: info}, nil
}

func handlePreReview(ctx context.Context, a *cli.App, input PreReviewInput, by string) (*mcp.CallToolResult, PreReviewOutput, error) {
	if input.Score < 0 || input.Score > 100 {
		return nil, PreReviewOutput{}, fmt.Errorf("score must be between 0 and 100, got %d", input.Score)
	}
	projectPath, err := resolveReviewProject(a, input.Project)
	if err != nil {
		return nil, PreReviewOutput{}, err
	}
	rev := state.AIReview{Score: input.Score, Model: by}
	for _, r := range input.Reasons {
		rev.Findings = append(rev.Findings, state.AIReviewFinding{
			Severity: r.Severity, Message: r.Message, Suggestion: r.Suggestion,
		})
	}
	n, err := a.RecordAIReviews(ctx, projectPath, "", input.Locale, input.File, map[string]state.AIReview{input.Key: rev})
	if err != nil {
		return nil, PreReviewOutput{}, err
	}
	return nil, PreReviewOutput{Recorded: n > 0, By: by}, nil
}
