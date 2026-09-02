package mcptools

// MCP review tools — agent parity for the review workflow (issue #1077,
// phase 4). Agents get the same verbs the desktop Review page and `kapi
// status --review` / `kapi apply` offer, routed through the exact same CLI
// layer (cli.ProjectConvergence, cli.ReviewUnitWithContext,
// cli.ApplyReviewDecisionAs),
// so a decision made over MCP is indistinguishable in the state store from
// one made in the desktop — except for its identity: MCP decisions record
// Decision.By as "agent/<client>" (the MCP client's declared name) or
// "agent" when the client did not introduce itself. Autonomous AI approvals
// (the desktop pre-review) use "ai/<model>" instead; gates treat only the
// "ai/" prefix specially (core/gate approver classes).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/cli"
)

func init() {
	cli.RegisterMCPToolFactory(registerReviewTools)
}

// registerReviewTools registers the review-workflow MCP tools.
func registerReviewTools(server *mcp.Server, a *cli.App) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "review_queue",
		Description: "List the review queue: every unit awaiting a person, addressed by (file, key, locale). One queue holds every language, the project's source language among them: a translated unit not yet approved is one row, and a source unit the project's source gate is waiting on is another, marked `isSource`. The result also carries `languages`, the pending count per language. Filter with language, locale and/or collection. Read-only, derived from the content files and the project state store; units annotated by an AI pre-review carry their score. Lean by design: call review_unit for a unit's context (the point governing it, its neighbourhood, its prior version, its findings).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewQueueInput) (*mcp.CallToolResult, ReviewQueueOutput, error) {
		return handleReviewQueue(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:         "review_unit",
		Description:  "Fetch one review-queue unit's full picture: source and target text, ladder status, the last recorded state (with identity), and the context the decision is made in: the point governing the file (voice guidance, term rules, coordinates), the blocks before and after it as run sequences, the prior approved version and the content-memory match with its wording, the check findings with their run anchors, and the AI pre-review score. A unit in the project's source language is read the same way, from its source file, and returns its authoring rung with no target half. The read leg before approve_unit / reject_unit / sign_off_unit.",
		OutputSchema: reviewUnitOutputSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewUnitInput) (*mcp.CallToolResult, ReviewUnitOutput, error) {
		return handleReviewUnit(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "approve_unit",
		Description: "Approve one review-queue unit (→ reviewed). The unit state is recorded in the project store, bound to the current translation's content hash, with identity \"agent/<client>\".",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewDecisionInput) (*mcp.CallToolResult, ReviewDecisionOutput, error) {
		return handleReviewDecision(ctx, a, input, cli.ReviewDecisionApproved, agentIdentity(req))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reject_unit",
		Description: "Reject one review-queue unit (→ draft, back to the work queue) with a note explaining why. Recorded with identity \"agent/<client>\"; retranslating the unit re-enters it in review.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewDecisionInput) (*mcp.CallToolResult, ReviewDecisionOutput, error) {
		return handleReviewDecision(ctx, a, input, cli.ReviewDecisionRejected, agentIdentity(req))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sign_off_unit",
		Description: "Sign off one review-queue unit (→ signed-off, the top ladder rung). Recorded with identity \"agent/<client>\".",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReviewDecisionInput) (*mcp.CallToolResult, ReviewDecisionOutput, error) {
		return handleReviewDecision(ctx, a, input, cli.ReviewDecisionSignedOff, agentIdentity(req))
	})
}

// agentIdentity derives the decision identity for an MCP call: "agent/<name>"
// from the client's declared implementation name, or the bare "agent" when the
// session carries none. Never "ai/…" — an MCP agent acts on a person's behalf,
// so its approvals count as human-class for gates.
func agentIdentity(req *mcp.CallToolRequest) string {
	if req != nil && req.Session != nil {
		if ip := req.Session.InitializeParams(); ip != nil && ip.ClientInfo != nil && ip.ClientInfo.Name != "" {
			return "agent/" + ip.ClientInfo.Name
		}
	}
	return "agent"
}

// resolveReviewProject resolves the target project recipe: the explicit
// `project` input when given, else the ambient project (KAPI_PROJECT / upward
// walk, honoring KAPI_NO_PROJECT).
func resolveReviewProject(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	path, err := cli.ResolveProjectPath(nil)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("no kapi project found. Pass the project input (path to the kapi.yaml recipe)")
	}
	return path, nil
}

// --- Input/Output types ---

type ReviewQueueInput struct {
	Project    string `json:"project,omitempty" jsonschema:"Path to the .kapi project file (default: the ambient project)"`
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
	Project string `json:"project,omitempty" jsonschema:"Path to the .kapi project file (default: the ambient project)"`
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

type ReviewDecisionInput struct {
	Project string `json:"project,omitempty" jsonschema:"Path to the .kapi project file (default: the ambient project)"`
	Locale  string `json:"locale" jsonschema:"Target locale, as listed by review_queue"`
	File    string `json:"file" jsonschema:"Target file path, as listed by review_queue"`
	Key     string `json:"key" jsonschema:"Unit key, as listed by review_queue"`
	Note    string `json:"note,omitempty" jsonschema:"Reviewer note (the reason, for reject_unit)"`
}

type ReviewDecisionOutput struct {
	Decision string `json:"decision"`
	// Changed is false when the unit already carried this exact decision for
	// this exact translation (a redundant call is a no-op, not an error).
	Changed bool `json:"changed"`
	// By is the identity the decision was recorded with ("agent/<client>").
	By string `json:"by"`
}

// --- Handlers ---

func handleReviewQueue(ctx context.Context, a *cli.App, input ReviewQueueInput) (*mcp.CallToolResult, ReviewQueueOutput, error) {
	projectPath, err := resolveReviewProject(input.Project)
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
	projectPath, err := resolveReviewProject(input.Project)
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

func handleReviewDecision(ctx context.Context, a *cli.App, input ReviewDecisionInput, decision, by string) (*mcp.CallToolResult, ReviewDecisionOutput, error) {
	projectPath, err := resolveReviewProject(input.Project)
	if err != nil {
		return nil, ReviewDecisionOutput{}, err
	}
	changed, err := a.ApplyReviewDecisionAs(ctx, projectPath, "", cli.ReviewUnitRef{
		File: input.File, Key: input.Key, Locale: input.Locale,
	}, decision, input.Note, by)
	if err != nil {
		return nil, ReviewDecisionOutput{}, err
	}
	return nil, ReviewDecisionOutput{Decision: decision, Changed: changed, By: by}, nil
}
