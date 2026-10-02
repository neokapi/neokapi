package mcptools

// MCP review tools: the review queue and the review picture of one block. An
// agent reads what awaits a person and records its pre-review through
// apply_edits, as a decide operation with outcome advise bound to the
// revision review_block reports; it never records a decision. Only a person
// establishes a block, through the desktop Review page, `kapi apply`, or a
// hosted review session, so an agent's judgement never counts as a person's.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
)

func init() {
	cli.RegisterMCPToolFactory(registerReviewTools)
}

// registerReviewTools registers the review-workflow MCP tools.
func registerReviewTools(server *mcp.Server, a *cli.App) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "review_queue",
		Description: "List the review queue: every unit awaiting a person, each with ref, the reference review_block reads it by ({doc, block, edition}: the source document, the block key and the translation's language; no edition for a source unit). One queue holds every language, the project's source language among them: a translated unit not yet approved is one row, and a source unit held below the project's translate_after level is another, marked `isSource`. The result also carries `languages`, the pending count per language. Filter with language, locale and/or collection. Read-only, derived from the content files and the project state store; units annotated by an AI pre-review carry their score. Lean by design: call review_block for a unit's context (the point governing it, its neighbourhood, its prior version, its findings).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewQueueInput) (*mcp.CallToolResult, ReviewQueueOutput, error) {
		return handleReviewQueue(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:         "review_block",
		Description:  "Read one block's review picture: ref and rev, the reference and revision of the edition under review, and the unit: source and target text, ladder status, the last recorded decision (with identity), and the context the decision is made in: the point governing the file (voice guidance, term rules, coordinates), the blocks before and after it as run sequences, the prior approved version and the content-memory match with its wording, the check findings with their run anchors, and the AI pre-review score. Address it with the ref a review_queue row carries, or a ref read_blocks reports with the translation's edition. A block in the project's source language is read from its source file and returns its authoring rung with no target half. To record a pre-review, send apply_edits a decide operation with at set to ref, if_match set to rev, outcome advise, a score from 0 to 100 and your reasons: it is bound to the wording you read, a later edit drops it, and it never establishes the block.",
		OutputSchema: reviewBlockOutputSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewBlockInput) (*mcp.CallToolResult, ReviewBlockOutput, error) {
		return handleReviewBlock(ctx, a, input)
	})
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
	Pending []ReviewQueueRow `json:"pending"`
	Total   int              `json:"total"`
	// Languages counts the pending units per language over the whole queue,
	// before any filter, so a caller can narrow to a language it knows has work.
	Languages []cli.ReviewLanguage `json:"languages,omitempty"`
}

// ReviewQueueRow is one unit of the queue with the reference review_block
// reads it by.
type ReviewQueueRow struct {
	cli.ReviewQueueItem
	Ref BlockRef `json:"ref"`
}

// ReviewBlockInput names the edition to read.
type ReviewBlockInput struct {
	Project string   `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
	At      BlockRef `json:"at" jsonschema:"the edition to review, as a review_queue row's ref names it"`
}

// BlockRef is a reference to one edition of one block, in the form
// review_queue reports it and review_block takes it.
type BlockRef struct {
	Doc     string `json:"doc" jsonschema:"the source document, project-relative, or the file of a translation"`
	Block   string `json:"block" jsonschema:"the block key, as review_queue and read_blocks report it"`
	Edition string `json:"edition,omitempty" jsonschema:"the translation's language, such as fr; omitted, or the project's source language, reviews the source"`
}

// ReviewBlockOutput is one edition's review picture.
type ReviewBlockOutput struct {
	Ref  BlockRef            `json:"ref"`
	Rev  string              `json:"rev"`
	Unit *cli.ReviewUnitInfo `json:"unit"`
}

// reviewBlockOutputSchema declares review_block's result instead of letting
// the SDK infer it from the Go type.
//
// A block's runs nest: a plural run holds a run sequence per form, so model.Run
// refers to itself. The SDK's inference walks the type graph rather than
// emitting a $ref, and refuses a cycle, so a unit carrying its neighbourhood
// as runs cannot have a schema inferred at all. Declaring the envelope keeps
// the tool registrable and leaves the unit an object the client reads by name;
// the field documentation lives on host.ReviewUnitInfo and host.ReviewContext.
var reviewBlockOutputSchema = json.RawMessage(`{"type":"object","properties":{` +
	`"ref":{"type":"object","description":"the edition under review: doc, block and edition"},` +
	`"rev":{"type":"string","description":"the edition's revision, the if_match of a decide operation about it"},` +
	`"unit":{"type":"object"}}}`)

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
	pending := make([]ReviewQueueRow, 0, len(queue.Pending))
	for _, it := range queue.Pending {
		if input.Locale != "" && it.Locale != input.Locale {
			continue
		}
		if input.Collection != "" && it.Collection != input.Collection {
			continue
		}
		pending = append(pending, ReviewQueueRow{ReviewQueueItem: it, Ref: blockRef(host.ReviewQueueRef(it))})
	}
	return nil, ReviewQueueOutput{Pending: pending, Total: len(pending), Languages: queue.Languages}, nil
}

func handleReviewBlock(ctx context.Context, a *cli.App, input ReviewBlockInput) (*mcp.CallToolResult, ReviewBlockOutput, error) {
	projectPath, err := resolveReviewProject(a, input.Project)
	if err != nil {
		return nil, ReviewBlockOutput{}, err
	}
	edition, err := model.ParseEditionKey(input.At.Edition)
	if err != nil {
		return nil, ReviewBlockOutput{}, fmt.Errorf("at.edition: %w", err)
	}
	rb, err := a.ReviewBlockAt(ctx, projectPath, change.Ref{Doc: input.At.Doc, Block: input.At.Block, Edition: edition})
	if err != nil {
		return nil, ReviewBlockOutput{}, err
	}
	return nil, ReviewBlockOutput{Ref: blockRef(rb.Ref), Rev: rb.Rev, Unit: rb.Unit}, nil
}

// blockRef is r in the form a review tool reports it.
func blockRef(r change.Ref) BlockRef {
	return BlockRef{Doc: r.Doc, Block: r.Block, Edition: r.EditionText()}
}
