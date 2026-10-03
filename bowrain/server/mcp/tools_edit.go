package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changeschema"
)

// ChangeServiceFactory builds the change service of one stream of a project for
// an agent's change sets: the stream home over its rows, the server's policy
// for the user the agent acts for, its commit check and its recorder. landed
// does what a change set that landed asks of the rest of the server.
type ChangeServiceFactory func(ctx context.Context, userID, projectID, stream string) (svc *change.Service, landed func(context.Context, *change.Result), err error)

// WithChangeService enables the edit tools (read_blocks, apply_edits,
// describe_format), which read and change a stream's content through the
// change service the factory builds. Without it they are not registered.
func WithChangeService(f ChangeServiceFactory) Option {
	return func(s *MCPServer) { s.changes = f }
}

// streamArgs names the stream a call reads or changes.
const (
	argProjectID = "the project ID"
	argStream    = "the stream (defaults to main)"
)

type readBlocksInput struct {
	ProjectID string   `json:"project_id" jsonschema:"the project ID"`
	Stream    string   `json:"stream,omitempty" jsonschema:"the stream (defaults to main)"`
	Doc       string   `json:"doc" jsonschema:"the item to read, by its path"`
	Blocks    []string `json:"blocks,omitempty" jsonschema:"read only these blocks, by key; omitted reads every block"`
	Cursor    string   `json:"cursor,omitempty" jsonschema:"the next value of the page before, to continue a read"`
	Limit     int      `json:"limit,omitempty" jsonschema:"the most blocks a page holds (default 100, at most 1000)"`
}

type describeFormatInput struct {
	ProjectID string `json:"project_id" jsonschema:"the project ID"`
	Stream    string `json:"stream,omitempty" jsonschema:"the stream (defaults to main)"`
	Doc       string `json:"doc,omitempty" jsonschema:"an item, to describe the format it is read in"`
	Format    string `json:"format,omitempty" jsonschema:"a format by name"`
}

// applyEditsInputSchema is the change-set schema with the stream the change
// set is applied to: the project and the stream, which the tool reads and
// removes before decoding the change set.
var applyEditsInputSchema = sync.OnceValue(func() *jsonschema.Schema {
	var s jsonschema.Schema
	if err := json.Unmarshal(changeschema.Schema(), &s); err != nil {
		panic(fmt.Sprintf("apply_edits input schema: %v", err))
	}
	s.Properties["project_id"] = &jsonschema.Schema{Type: "string", Description: argProjectID}
	s.Properties["stream"] = &jsonschema.Schema{Type: "string", Description: argStream}
	s.Required = append(s.Required, "project_id")
	if len(s.PropertyOrder) > 0 {
		s.PropertyOrder = append(s.PropertyOrder, "project_id", "stream")
	}
	return &s
})

// registerEditTools registers the tools that read and change a stream's
// content through the change contract.
func (s *MCPServer) registerEditTools() {
	if s.changes == nil {
		return
	}
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "read_blocks",
		Description: "Read an item's blocks in a stream, a page at a time: the read leg of apply_edits. Each block carries " +
			"ref (doc, block and edition) to copy into an operation's at, rev to send as its if_match, and text with " +
			"inline codes as <x id=\"…\"/> placeholders, which an edit keeps. editions lists the block's translations " +
			"with their revision, text and status; ops lists the operations the block accepts. Pass next back as " +
			"cursor for the following page.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in readBlocksInput) (*mcp.CallToolResult, any, error) {
		res, err := s.readBlocks(ctx, req, in)
		return res, nil, err
	})

	s.server.AddTool(&mcp.Tool{
		Name:        "apply_edits",
		InputSchema: applyEditsInputSchema(),
		Description: "Apply a kapi.change/v1 change set to a stream: the one write verb. Name the project (project_id) " +
			"and the stream; each operation's at.doc is an item's path. Each content operation addresses one edition " +
			"of one block with at, the ref read_blocks reports, and carries if_match, the rev you read. set_content " +
			"replaces the text (keep the <x id=\"…\"/> placeholders; with if_match \"absent\" and an edition it creates " +
			"that translation), replace_text changes part of it by find, by start and end, or by range, remove_edition " +
			"drops a translation, and annotate and unannotate add and remove a note or an entity. A refused change set " +
			"writes nothing: an edition that moved since you read it is refused as stale with its current revision and " +
			"text, an edit that drops, invents or unbalances an inline code is refused as guard, an edit that brings in " +
			"a failing finding is refused as gate_failed with the findings, and every other operation reports " +
			"not_applied. Each refusal carries a code and the field at fault: re-read, fix the operation and resend. " +
			"mode preview checks the change set and writes nothing. Every operation is recorded as yours, the calling " +
			"agent's, acting for the signed-in user. A review decision is a person's: decide records only a pre-review, " +
			"outcome advise with a score from 0 to 100 and its reasons, which the review queue shows beside the " +
			"translation while it stands at the revision you read.",
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := s.applyEdits(ctx, req)
		if err != nil {
			res = &mcp.CallToolResult{}
			res.SetError(err)
		}
		return res, nil
	})

	mcp.AddTool(s.server, &mcp.Tool{
		Name: "describe_format",
		Description: "Say what apply_edits can do in an item's format: for each content operation what it accepts, or " +
			"null where the stream refuses it as unsupported. Name a format, or an item to describe the format it is " +
			"read in.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in describeFormatInput) (*mcp.CallToolResult, any, error) {
		res, err := s.describeFormat(ctx, req, in)
		return res, nil, err
	})
}

// changeService is the change service of the stream a call names, after the
// caller's access to the project is proved.
func (s *MCPServer) changeService(ctx context.Context, req *mcp.CallToolRequest, project, stream string) (*change.Service, func(context.Context, *change.Result), error) {
	projectID, err := s.authorizeProject(ctx, req, project)
	if err != nil {
		return nil, nil, err
	}
	if s.changes == nil {
		return nil, nil, errors.New("this server reads and changes no content through the change service")
	}
	if stream == "" {
		stream = "main"
	}
	return s.changes(ctx, callerID(req), projectID, stream)
}

func (s *MCPServer) readBlocks(ctx context.Context, req *mcp.CallToolRequest, in readBlocksInput) (*mcp.CallToolResult, error) {
	svc, _, err := s.changeService(ctx, req, in.ProjectID, in.Stream)
	if err != nil {
		return nil, err
	}
	page, err := svc.Read(ctx, change.ReadRequest{Doc: in.Doc, Blocks: in.Blocks, Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return changeError(err)
	}
	return jsonToolResult(page, false)
}

func (s *MCPServer) describeFormat(ctx context.Context, req *mcp.CallToolRequest, in describeFormatInput) (*mcp.CallToolResult, error) {
	svc, _, err := s.changeService(ctx, req, in.ProjectID, in.Stream)
	if err != nil {
		return nil, err
	}
	d, err := svc.Describe(ctx, change.DescribeRequest{Format: in.Format, Doc: in.Doc})
	if err != nil {
		return changeError(err)
	}
	return jsonToolResult(d, false)
}

// applyEdits decodes the change set a call carries and applies it as the
// calling agent. A refused or partial change set is an error result carrying
// the structured result, so a client that reads only isError still learns that
// the change did not land.
func (s *MCPServer) applyEdits(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, stream, body, cerr := splitStreamArgs(req.Params.Arguments)
	if cerr != nil {
		return changeRefusal(cerr)
	}
	svc, landed, err := s.changeService(ctx, req, project, stream)
	if err != nil {
		return nil, err
	}
	set, err := change.Decode(bytes.NewReader(body))
	if err != nil {
		return changeError(err)
	}
	res, err := svc.Apply(ctx, set, change.Actor{Kind: change.ActorAgent, Name: clientName(req), Session: sessionID(req)})
	if err != nil {
		return changeError(err)
	}
	if landed != nil {
		landed(ctx, res)
	}
	return jsonToolResult(res, res.Status == change.SetRefused || res.Status == change.SetPartial)
}

// splitStreamArgs takes the project and the stream out of a call's arguments
// and returns the rest, the change set, as JSON.
func splitStreamArgs(args json.RawMessage) (project, stream string, body []byte, cerr *change.Error) {
	if len(bytes.TrimSpace(args)) == 0 {
		return "", "", nil, &change.Error{Code: change.CodeInvalid, Message: "the call carries no change set; send the envelope with its ops"}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return "", "", nil, &change.Error{Code: change.CodeInvalid, Message: "the arguments are not a change set: " + err.Error()}
	}
	for name, dst := range map[string]*string{"project_id": &project, "stream": &stream} {
		raw, ok := fields[name]
		if !ok {
			continue
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			return "", "", nil, &change.Error{Code: change.CodeInvalid, Pointer: "/" + name, Message: name + " is a string"}
		}
		delete(fields, name)
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return "", "", nil, &change.Error{Code: change.CodeInvalid, Message: err.Error()}
	}
	return project, stream, body, nil
}

// clientName is the name the MCP client gave itself, which an agent's records
// name it by.
func clientName(req *mcp.CallToolRequest) string {
	if req == nil || req.Session == nil {
		return ""
	}
	params := req.Session.InitializeParams()
	if params == nil || params.ClientInfo == nil {
		return ""
	}
	return params.ClientInfo.Name
}

// sessionID is the MCP session a call belongs to.
func sessionID(req *mcp.CallToolRequest) string {
	if req == nil || req.Session == nil {
		return ""
	}
	return req.Session.ID()
}

// changeError is the error result of a refusal, or err as it is.
func changeError(err error) (*mcp.CallToolResult, error) {
	if ce, ok := errors.AsType[*change.Error](err); ok && ce != nil {
		return changeRefusal(ce)
	}
	return nil, err
}

// changeRefusal is an error result naming the refusal, as the structured
// result and as its text: a kapi.change-result/v1 whose error says why.
func changeRefusal(e *change.Error) (*mcp.CallToolResult, error) {
	return jsonToolResult(change.ErrorResult(e), true)
}

// jsonToolResult is a tool result carrying v as its structured content and as
// its text, with HTML escaping off, so the placeholders a block's text holds
// read as written.
func jsonToolResult(v any, isError bool) (*mcp.CallToolResult, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode the result: %w", err)
	}
	raw := strings.TrimRight(buf.String(), "\n")
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: raw}},
		StructuredContent: json.RawMessage(raw),
		IsError:           isError,
	}, nil
}
