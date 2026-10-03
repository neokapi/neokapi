package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
)

// A host that serves calls rather than one command line answers each call
// with the change service of the project the call names: the MCP edit tools
// (read_blocks, apply_edits, describe_format) and the browser engine's
// kapiRead, kapiApply and kapiDescribe. Both build the service here, so a
// reference or a revision one of them reports is one the other, and kapi
// apply, takes.
//
// The browser engine carries the contract as JSON in and JSON out
// (ReadChangesJSON, ApplyChangesJSON, DescribeChangesJSON). Every answer the
// contract defines is JSON: a read page, a kapi.change-result/v1 result, or a
// format's description. A request the service refuses as a whole, and one
// that does not decode, is answered as a kapi.change-result/v1 whose error
// says why (change.ErrorResult), as kapi apply --json and apply_edits answer
// one. An error the contract has no code for, such as a project that names no
// file, is returned as an error.

// ChangeCallOptions is what one call says beside its request.
type ChangeCallOptions struct {
	// Project is the project the call acts on: its kapi.yaml recipe, its root
	// directory, or any path inside it. Empty is the project discovery finds
	// from the working directory, as for a command given no -p.
	Project string `json:"project,omitempty"`
	// Actor is who sends the change set an apply carries: a person or an
	// agent. Empty is who kapi apply records in the same environment (a
	// person, unless KAPI_ACTOR or an agent host's marker says otherwise).
	// A read and a description take none.
	Actor *change.Actor `json:"actor,omitempty"`
}

// callChangeService builds the change service for the project one call names
// and returns the recipe it resolved ("" outside a project, where the service
// edits the documents under the working directory). An empty project is the
// one the MCP server started in, else the one discovery finds
// (ResolveMCPCallProject). The service reads the documents in that project's
// source language, so an edition in any other language is a translation,
// whichever project the host answered last. origin names the surface in the
// record of a change.
//
// editions are the editions the call names: the ones a read asks for, or the
// ones an apply's operations address. The one language among them other than
// the source is the language a bilingual file is read in (a PO catalog's
// msgstr), as kapi apply reads one (targetLocaleOf).
func (a *App) callChangeService(ctx context.Context, project, origin string, editions []model.EditionKey) (*change.Service, string, error) {
	recipe, err := a.ResolveMCPCallProject(project)
	if err != nil {
		return nil, "", err
	}
	source := model.LocaleID(a.mcpCallSourceLocale(recipe))
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: origin,
		SourceLocale: source, TargetLocale: SoleTargetLocale(editions, source)})
	if err != nil {
		return nil, "", err
	}
	return svc, recipe, nil
}

// OpEditions are the editions a change set's operations address.
func OpEditions(set change.Set) []model.EditionKey {
	keys := make([]model.EditionKey, len(set.Ops))
	for i, op := range set.Ops {
		keys[i] = op.At.Edition
	}
	return keys
}

// applyCall applies set as actor through svc, the service of the project at
// recipe, and notes the wording an agent's applied edits wrote against the
// suggestions that prefer one (noteAgentEdits).
func (a *App) applyCall(ctx context.Context, svc *change.Service, recipe string, set change.Set, actor change.Actor) (*change.Result, error) {
	var before map[int]string
	if actor.Kind == change.ActorAgent {
		before = wordingBefore(ctx, svc, set)
	}
	res, err := svc.Apply(ctx, set, actor)
	if err != nil {
		return nil, err
	}
	if res.Status == change.SetApplied || res.Status == change.SetPartial {
		a.noteAgentEdits(ctx, recipe, contextop.Actor{Kind: contextop.ActorKind(actor.Kind), Name: actor.Name, Session: actor.Session},
			appliedWording(set, res, before))
	}
	return res, nil
}

// editWording is what one applied content operation wrote into an edition:
// the wording it put there, and the wording it took the place of, so a count
// of a form in it credits only the uses the edit introduced.
type editWording struct {
	Before, After string
}

// wordingBefore reads the text each set_content of set replaces in a
// document's own edition, by the operation's index, so the wording it writes
// can be weighed against what was there. A block the read does not find is
// left out, and its edit counts as all new.
func wordingBefore(ctx context.Context, svc *change.Service, set change.Set) map[int]string {
	byDoc := map[string][]int{}
	for i, op := range set.Ops {
		if _, ok := op.Body.(*change.SetContent); ok && op.At.Doc != "" && op.At.Block != "" {
			byDoc[op.At.Doc] = append(byDoc[op.At.Doc], i)
		}
	}
	out := map[int]string{}
	for doc, ops := range byDoc {
		keys := make([]string, 0, len(ops))
		for _, i := range ops {
			keys = append(keys, set.Ops[i].At.Block)
		}
		page, err := svc.Read(ctx, change.ReadRequest{Doc: doc, Blocks: keys, Limit: change.MaxReadLimit})
		if err != nil {
			continue
		}
		text := map[string]string{}
		for _, b := range page.Blocks {
			text[b.Ref.Block] = b.Text
		}
		for _, i := range ops {
			if t, ok := text[set.Ops[i].At.Block]; ok {
				out[i] = t
			}
		}
	}
	return out
}

// appliedWording is the wording each applied content operation wrote into a
// document's own edition, in placeholder text, by the document as the result
// names it, beside the wording it took the place of, so noteAgentEdits can
// count the forms a suggestion prefers that the edit introduced at the
// document's point. A replace_text wrote its replacements in place of what
// each edit's find named; a set_content wrote its whole text in place of the
// text before (before, by operation index). A translation is left out: a
// suggestion's preferred form is wording in the source language.
func appliedWording(set change.Set, res *change.Result, before map[int]string) map[string][]editWording {
	out := map[string][]editWording{}
	for i, op := range set.Ops {
		if i >= len(res.Ops) || res.Ops[i].Status != change.OpApplied {
			continue
		}
		at := res.Ops[i].At
		if at == nil || !at.Edition.IsZero() {
			continue
		}
		var texts []editWording
		switch body := op.Body.(type) {
		case *change.SetContent:
			switch {
			case body.Text != nil:
				texts = append(texts, editWording{Before: before[i], After: *body.Text})
			case body.Runs != nil:
				texts = append(texts, editWording{Before: before[i], After: model.RunsEditText(body.Runs)})
			}
		case *change.ReplaceText:
			for _, e := range body.Edits {
				w := editWording{After: e.Text}
				if e.Find != nil {
					w.Before = *e.Find
				}
				texts = append(texts, w)
			}
		}
		if len(texts) > 0 {
			out[at.Doc] = append(out[at.Doc], texts...)
		}
	}
	return out
}

// editionKeys parses the editions a read names, strictly: a language that is
// not one is refused as invalid rather than read as a key nobody holds.
func editionKeys(names []string) ([]model.EditionKey, *change.Error) {
	keys := make([]model.EditionKey, 0, len(names))
	for _, e := range names {
		k, err := model.ParseEditionKey(e)
		if err != nil {
			return nil, &change.Error{Code: change.CodeInvalid, Field: "editions", Message: err.Error()}
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// changeReadCall is a read request as JSON carries it: change.ReadRequest,
// with the editions in their text form.
type changeReadCall struct {
	Doc      string   `json:"doc"`
	Blocks   []string `json:"blocks,omitempty"`
	Editions []string `json:"editions,omitempty"`
	Cursor   string   `json:"cursor,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}

// ReadChangesJSON answers a read sent as JSON: request is a read request
// ({doc, blocks, editions, cursor, limit}, as change.ReadRequest), options a
// ChangeCallOptions or empty. The answer is the page (change.Page), or a
// kapi.change-result/v1 refusal: a document that does not exist, a stale
// cursor, a request that does not decode.
func (a *App) ReadChangesJSON(ctx context.Context, origin string, request, options []byte) ([]byte, error) {
	opts, cerr := decodeCallOptions(options, false)
	if cerr != nil {
		return changeAnswer(change.ErrorResult(cerr))
	}
	var in changeReadCall
	if cerr := decodeCallJSON(request, "read request", &in); cerr != nil {
		return changeAnswer(change.ErrorResult(cerr))
	}
	editions, cerr := editionKeys(in.Editions)
	if cerr != nil {
		return changeAnswer(change.ErrorResult(cerr))
	}
	svc, _, err := a.callChangeService(ctx, opts.Project, origin, editions)
	if err != nil {
		return nil, err
	}
	page, err := svc.Read(ctx, change.ReadRequest{Doc: in.Doc, Blocks: in.Blocks, Editions: editions, Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return changeRefusalAnswer(err)
	}
	return changeAnswer(page)
}

// ApplyChangesJSON applies a kapi.change/v1 change set sent as JSON (an
// object with ops, JSONL, or an array of operations, as kapi apply reads one)
// through the change service of the project options names, and answers the
// kapi.change-result/v1 result. The actor options names sends it; with none,
// it is who kapi apply records in the same environment. A change set that
// does not decode, or is refused, is answered as a result like any other.
func (a *App) ApplyChangesJSON(ctx context.Context, origin string, set, options []byte) ([]byte, error) {
	opts, cerr := decodeCallOptions(options, true)
	if cerr != nil {
		return changeAnswer(change.ErrorResult(cerr))
	}
	cs, err := change.Decode(bytes.NewReader(set))
	if err != nil {
		return changeRefusalAnswer(err)
	}
	var actor change.Actor
	if opts.Actor != nil {
		actor = *opts.Actor
	} else {
		resolved, err := a.commandActor()
		if err != nil {
			return nil, err
		}
		actor = changeActorOf(resolved.Actor)
		cs.Note = resolved.NoteWith(cs.Note)
	}
	svc, recipe, err := a.callChangeService(ctx, opts.Project, origin, OpEditions(cs))
	if err != nil {
		return nil, err
	}
	res, err := a.applyCall(ctx, svc, recipe, cs, actor)
	if err != nil {
		return changeRefusalAnswer(err)
	}
	return changeAnswer(res)
}

// DescribeChangesJSON answers what a format supports, asked as JSON: request
// names a format or a document ({format, doc}, as change.DescribeRequest),
// options a ChangeCallOptions or empty. The answer is the description
// (change.Description), or a kapi.change-result/v1 refusal.
func (a *App) DescribeChangesJSON(ctx context.Context, origin string, request, options []byte) ([]byte, error) {
	opts, cerr := decodeCallOptions(options, false)
	if cerr != nil {
		return changeAnswer(change.ErrorResult(cerr))
	}
	var in change.DescribeRequest
	if cerr := decodeCallJSON(request, "describe request", &in); cerr != nil {
		return changeAnswer(change.ErrorResult(cerr))
	}
	svc, _, err := a.callChangeService(ctx, opts.Project, origin, nil)
	if err != nil {
		return nil, err
	}
	d, err := svc.Describe(ctx, in)
	if err != nil {
		return changeRefusalAnswer(err)
	}
	if d == nil {
		return nil, errors.New("describe: the change service found no description")
	}
	return changeAnswer(d)
}

// decodeCallOptions reads a call's options. Empty input is no options. Only
// an apply names an actor, and only a person or an agent: a tool sends the
// changes of a flow, through the flow.
func decodeCallOptions(raw []byte, actor bool) (ChangeCallOptions, *change.Error) {
	var opts ChangeCallOptions
	if len(bytes.TrimSpace(raw)) == 0 {
		return opts, nil
	}
	if cerr := decodeCallJSON(raw, "options", &opts); cerr != nil {
		return opts, cerr
	}
	if opts.Actor == nil {
		return opts, nil
	}
	if !actor {
		return opts, &change.Error{Code: change.CodeInvalid, Pointer: "/actor", Message: "only an apply names an actor; a read and a description change nothing"}
	}
	switch opts.Actor.Kind {
	case change.ActorPerson, change.ActorAgent:
		return opts, nil
	}
	return opts, &change.Error{Code: change.CodeInvalid, Pointer: "/actor/kind",
		Message: fmt.Sprintf("the actor is %q or %q, not %q", change.ActorPerson, change.ActorAgent, opts.Actor.Kind)}
}

// decodeCallJSON decodes one JSON object into v strictly: a field v does not
// name, or anything after the object, is refused as invalid.
func decodeCallJSON(raw []byte, what string, v any) *change.Error {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return &change.Error{Code: change.CodeInvalid, Message: "the " + what + " is empty; send a JSON object"}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &change.Error{Code: change.CodeInvalid, Message: "the " + what + " is not one: " + err.Error()}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return &change.Error{Code: change.CodeInvalid, Message: "the " + what + " is one JSON object; found more after it"}
	}
	return nil
}

// changeRefusalAnswer is the answer to a refusal the change service returned
// as an error: a kapi.change-result/v1 naming it, or the error itself when it
// is not one of the contract's.
func changeRefusalAnswer(err error) ([]byte, error) {
	if ce, ok := errors.AsType[*change.Error](err); ok && ce != nil {
		return changeAnswer(change.ErrorResult(ce))
	}
	return nil, err
}

// changeAnswer encodes v as one line of JSON with HTML escaping off, so the
// placeholders a block's text holds read as written rather than as <
// escapes.
func changeAnswer(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode the answer: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
