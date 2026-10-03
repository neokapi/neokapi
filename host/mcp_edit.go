//go:build !js

package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changeschema"
	"github.com/neokapi/neokapi/core/model"
)

// The edit tools of the MCP surface: read_blocks reads a document's blocks
// through the change service, apply_edits sends it a kapi.change/v1 change
// set, and describe_format says what a format supports. They are the agent's
// half of the one edit contract: `kapi inspect` and `kapi apply` are the
// command line's, over the same service, so a reference or a revision one
// reports is one the other takes.
//
// The transport stamps the actor. Every change set apply_edits sends is the
// calling agent's, named by the client's initialize name, in this server's
// session; nothing in a change set can name another sender.
func init() {
	RegisterMCPToolFactory(registerEditMCPTools)
}

// mcpChangeOrigin names the MCP surface in the record of a change.
const mcpChangeOrigin = "mcp"

// mcpProjectArg is the description of the per-call project argument the edit
// tools take, as every project-scoped tool does.
const mcpProjectArg = "the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"

// readBlocksInput names the blocks a read_blocks call returns.
type readBlocksInput struct {
	Doc      string   `json:"doc" jsonschema:"the document: a path inside the project, relative to its root or absolute, or outside a project a path under the server's working directory; container!entry names an archive member; the file of a translation reads that edition of its source"`
	Blocks   []string `json:"blocks,omitempty" jsonschema:"only these blocks, by the block key a read reports; empty reads every block"`
	Editions []string `json:"editions,omitempty" jsonschema:"translations kept in files of their own to show beside each block, such as fr or de; the editions the document holds itself are always shown"`
	Cursor   string   `json:"cursor,omitempty" jsonschema:"continue a read: the next value of the page before"`
	Limit    int      `json:"limit,omitempty" jsonschema:"the most blocks a page holds (default 100, at most 1000)"`
	Project  string   `json:"project,omitempty"`
}

// describeFormatInput names the format a describe_format call describes.
type describeFormatInput struct {
	Format  string `json:"format,omitempty" jsonschema:"a format name, such as html, markdown or po"`
	Doc     string `json:"doc,omitempty" jsonschema:"a document, to describe the format kapi reads it in; give this or format"`
	Project string `json:"project,omitempty"`
}

// readBlocksOutputSchema declares read_blocks' result. A block's codes,
// structures and editions are objects whose fields the tool description
// names; inferring them from the Go types would spell out the run model
// twice in a tool every writing session is offered.
var readBlocksOutputSchema = json.RawMessage(`{"type":"object","properties":{` +
	`"doc":{"type":"string","description":"the document, as references name it"},` +
	`"home":{"type":"string","description":"where the document's text lives: file"},` +
	`"format":{"type":"string"},` +
	`"head":{"type":"string","description":"the digest of the document the page was read from"},` +
	`"blocks":{"type":"array","items":{"type":"object","properties":{` +
	`"ref":{"type":"object","description":"the reference to copy into an operation's at"},` +
	`"rev":{"type":"string","description":"the revision to send as if_match"},` +
	`"text":{"type":"string","description":"the content, inline codes as <x id=\"…\"/> placeholders"},` +
	`"codes":{"type":"object","description":"each inline code by the id its placeholder shows: kind, type, attributes, the attributes set_attribute can change, and equiv and disp, labels naming what the code stands for; an edit keeps a code by its placeholder, never by its label"},` +
	`"structures":{"type":"array","description":"each plural or select, with the path that reaches it and the text of each branch"},` +
	`"editions":{"type":"object","description":"the block's other editions by key: rev, text, status, basis and stale, each one's plurals and selects as structures, and its codes where they differ from the block's"},` +
	`"ops":{"type":"array","items":{"type":"string"},"description":"the operations the block accepts"}}}},` +
	`"next":{"type":"string","description":"the cursor of the next page; absent on the last"}}}`)

// applyEditsOutputSchema declares apply_edits' result, a
// kapi.change-result/v1 document.
var applyEditsOutputSchema = json.RawMessage(`{"type":"object","properties":{` +
	`"schema":{"type":"string","const":"kapi.change-result/v1"},` +
	`"status":{"type":"string","enum":["applied","refused","previewed","partial"],"description":"refused writes nothing; partial names the documents that landed"},` +
	`"record":{"description":"the id of the recorded edit, when one was recorded"},` +
	`"docs":{"type":"array","items":{"type":"object"},"description":"each document: written, the digests before and after, the findings, and in a preview the diff"},` +
	`"ops":{"type":"array","items":{"type":"object"},"description":"each operation by index: status, the revisions before and after, the positions it resolved, the translations it made stale (invalidates), and on a refusal the error and, when stale, the current revision and text"},` +
	`"error":{"type":"object","description":"why the change set could not be read, with the JSON pointer of what is wrong"}}}`)

// describeFormatOutputSchema declares describe_format's result.
var describeFormatOutputSchema = json.RawMessage(`{"type":"object","properties":{` +
	`"format":{"type":"string"},` +
	`"editions":{"type":"string","enum":["in-file","one-per-file"],"description":"whether the format keeps a document's editions in one file"},` +
	`"ops":{"type":"object","description":"each content operation with what the format supports of it, or null where it refuses the operation as unsupported"},` +
	`"native":{"type":"array","items":{"type":"object"},"description":"the format's own operations"}}}`)

// applyEditsInputSchema is the change-set schema with one more property, the
// per-call project, which the tool reads and removes before decoding the
// change set.
var applyEditsInputSchema = sync.OnceValue(func() *jsonschema.Schema {
	var s jsonschema.Schema
	if err := json.Unmarshal(changeschema.Schema(), &s); err != nil {
		panic(fmt.Sprintf("apply_edits input schema: %v", err))
	}
	s.Properties["project"] = &jsonschema.Schema{Type: "string", Description: mcpProjectArg}
	if len(s.PropertyOrder) > 0 {
		s.PropertyOrder = append(s.PropertyOrder, "project")
	}
	return &s
})

func registerEditMCPTools(server *mcp.Server, a *App) {
	readSchema, err := jsonschema.For[readBlocksInput](nil)
	if err != nil {
		panic(fmt.Sprintf("read_blocks input schema: %v", err))
	}
	readSchema.Properties["project"].Description = mcpProjectArg
	mcp.AddTool(server, &mcp.Tool{
		Name:         "read_blocks",
		InputSchema:  readSchema,
		OutputSchema: readBlocksOutputSchema,
		Description: "Read a document's blocks, a page at a time: the read leg of apply_edits. Each block carries ref " +
			"(doc, block and edition) to copy into an operation's at, rev to send as its if_match, and text with inline " +
			"codes as <x id=\"…\"/> placeholders, which an edit keeps. codes lists each code by the id its placeholder shows, " +
			"with its type and attributes; structures lists each plural or select with the path that reaches a branch and " +
			"the branch's text; editions lists the block's translations with their revision, text and status; ops lists the " +
			"operations the block accepts. A page holds up to limit blocks: pass next back as cursor for the following page. " +
			"A cursor from a document that changed since is refused as stale; read it again from the start. Reads the " +
			"document as apply_edits writes it, through the format and configuration the project binds.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in readBlocksInput) (*mcp.CallToolResult, any, error) {
		res, err := a.readBlocksMCP(ctx, in)
		return res, nil, err
	})

	server.AddTool(&mcp.Tool{
		Name:         "apply_edits",
		InputSchema:  applyEditsInputSchema(),
		OutputSchema: applyEditsOutputSchema,
		Description: "Apply a kapi.change/v1 change set: the one write verb. Each content operation addresses one edition " +
			"of one block with at, the ref read_blocks reports, and carries if_match, the rev you read. set_content replaces " +
			"the text (keep the <x id=\"…\"/> placeholders; with if_match \"absent\" and an edition it creates that translation), " +
			"replace_text changes part of it by find, by start and end, or by range, and remove_edition drops a translation. " +
			"In a JSON, YAML or ARB catalog, insert_block adds a key (doc, name, after or before, and its text per language " +
			"in editions) and delete_block removes one with its translations (at, and if_match mapping the catalog's own " +
			"language to the rev you read). " +
			"describe_format says which operations a format supports. A refused change set writes nothing: an edition " +
			"that moved since you read it is refused as stale with its current revision and text, an edit that drops, " +
			"invents or unbalances an inline code or flattens a plural is refused as guard, and every other operation reports " +
			"not_applied. Each refusal carries a code and the field at fault: re-read, fix the operation and resend. Status " +
			"partial means some of it landed: a write interrupted after some documents were written (docs says which), or " +
			"a decision or store operation refused after the content was written, which carries its error. Re-read the " +
			"documents before you send more. mode preview computes and checks the change set and returns a diff per " +
			"document without writing. " +
			"A source edit lists the translations it made stale under invalidates. Every operation is recorded as yours, " +
			"the calling agent's, in this server's session. Writing a term, a content-memory pair or a recipe field and " +
			"deciding a review are a person's: those operations are refused as not_permitted. Record a term rule as a " +
			"suggestion with context_observe, or context_correct for wording you changed, and record a pre-review as decide " +
			"with outcome advise, a score from 0 to 100 and your reasons, at the ref and rev review_block reports. " +
			"Run check_file on each changed file afterwards.",
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := a.applyEditsMCP(ctx, mcpChangeActor(req), req.Params.Arguments)
		if err != nil {
			// A failure is the tool's error result, as the typed tools report
			// one, rather than a protocol error.
			res = &mcp.CallToolResult{}
			res.SetError(err)
		}
		return res, nil
	})

	describeSchema, err := jsonschema.For[describeFormatInput](nil)
	if err != nil {
		panic(fmt.Sprintf("describe_format input schema: %v", err))
	}
	describeSchema.Properties["project"].Description = mcpProjectArg
	mcp.AddTool(server, &mcp.Tool{
		Name:         "describe_format",
		InputSchema:  describeSchema,
		OutputSchema: describeFormatOutputSchema,
		Description: "Say what apply_edits can do in a format: whether the format keeps a document's translations in one " +
			"file or one per file, and for each content operation what it accepts (the content forms of set_content, the " +
			"code types a writer can create, the attributes set_attribute can change per code type), or null where the " +
			"format refuses the operation as unsupported. Name a format, or a document to describe the format kapi reads " +
			"it in.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in describeFormatInput) (*mcp.CallToolResult, any, error) {
		res, err := a.describeFormatMCP(ctx, in)
		return res, nil, err
	})
}

// mcpChangeActor is the calling agent, in this server's session: the sender
// of every change set apply_edits applies.
func mcpChangeActor(req *mcp.CallToolRequest) change.Actor {
	return change.Actor{Kind: change.ActorAgent, Name: mcpClientName(req), Session: MCPSessionID()}
}

// mcpChangeService builds the change service for the project one MCP call
// names, and returns the recipe it resolved ("" outside a project, where the
// service edits the documents under the server's working directory). The
// service reads the documents in that project's source language, so an
// edition in any other language is a translation, whichever project the
// server started in or answered last. editions are the editions the call
// names (callChangeService).
func (a *App) mcpChangeService(ctx context.Context, project string, editions []model.EditionKey) (*change.Service, string, error) {
	return a.callChangeService(ctx, project, mcpChangeOrigin, editions)
}

func (a *App) readBlocksMCP(ctx context.Context, in readBlocksInput) (*mcp.CallToolResult, error) {
	editions, cerr := editionKeys(in.Editions)
	if cerr != nil {
		return changeRefusal(cerr)
	}
	svc, _, err := a.mcpChangeService(ctx, in.Project, editions)
	if err != nil {
		return nil, err
	}
	page, err := svc.Read(ctx, change.ReadRequest{Doc: in.Doc, Blocks: in.Blocks, Editions: editions, Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return changeError(err)
	}
	return jsonToolResult(page, false)
}

func (a *App) describeFormatMCP(ctx context.Context, in describeFormatInput) (*mcp.CallToolResult, error) {
	svc, _, err := a.mcpChangeService(ctx, in.Project, nil)
	if err != nil {
		return nil, err
	}
	d, err := svc.Describe(ctx, change.DescribeRequest{Format: in.Format, Doc: in.Doc})
	if err != nil {
		return changeError(err)
	}
	if d == nil {
		return nil, errors.New("describe_format found no description")
	}
	return jsonToolResult(d, false)
}

// applyEditsMCP decodes the change set the call carries, applies it as actor
// through the change service of the call's project, and returns the result.
// A refused or partial change set is an error result carrying the same
// structured result, so a client that reads only isError still learns that
// the change did not land.
func (a *App) applyEditsMCP(ctx context.Context, actor change.Actor, args json.RawMessage) (*mcp.CallToolResult, error) {
	project, body, cerr := splitProjectArg(args)
	if cerr != nil {
		return changeRefusal(cerr)
	}
	set, err := change.Decode(bytes.NewReader(body))
	if err != nil {
		return changeError(err)
	}
	svc, recipe, err := a.mcpChangeService(ctx, project, OpEditions(set))
	if err != nil {
		return nil, err
	}
	res, err := a.applyCall(ctx, svc, recipe, set, actor)
	if err != nil {
		return changeError(err)
	}
	return jsonToolResult(res, res.Status == change.SetRefused || res.Status == change.SetPartial)
}

// splitProjectArg takes the project argument out of a call's arguments and
// returns the rest, the change set, as JSON.
func splitProjectArg(args json.RawMessage) (string, []byte, *change.Error) {
	if len(bytes.TrimSpace(args)) == 0 {
		return "", nil, &change.Error{Code: change.CodeInvalid, Message: "the call carries no change set; send the envelope with its ops"}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return "", nil, &change.Error{Code: change.CodeInvalid, Message: "the arguments are not a change set: " + err.Error()}
	}
	var project string
	if raw, ok := fields["project"]; ok {
		if err := json.Unmarshal(raw, &project); err != nil {
			return "", nil, &change.Error{Code: change.CodeInvalid, Pointer: "/project", Message: "project is a path"}
		}
		delete(fields, "project")
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return "", nil, &change.Error{Code: change.CodeInvalid, Message: err.Error()}
	}
	return project, body, nil
}

// changeError is the tool result of a refusal the change service returned as
// an error, or the error itself when it is not one of the contract's.
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
// its text. HTML escaping is off, so the placeholders a block's text holds
// read as written rather than as < escapes.
func jsonToolResult(v any, isError bool) (*mcp.CallToolResult, error) {
	raw, err := changeAnswer(v)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(raw)}},
		StructuredContent: json.RawMessage(raw),
		IsError:           isError,
	}, nil
}
