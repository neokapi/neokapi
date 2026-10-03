package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// Kapi Desktop changes content through the change service (core/change), as
// kapi apply and MCP do: four bindings that take and return the change
// contract's JSON as strings, so the frontend reads and writes the shapes
// @neokapi/contract-types declares. A string, because an operation is a union
// that marshals itself, and a model Wails generated from the Go struct would
// describe its fields rather than the wire form.
//
//	Read      a page of a document's blocks (change.ReadRequest → change.Page)
//	Apply     a change set (kapi.change/v1 → kapi.change-result/v1)
//	Describe  what a format or a document supports (change.DescribeRequest)
//	History   the recorded changes to one edition (change.HistoryRequest)
//
// Every edit the desktop makes is a change set the person sends with the
// revision they rendered: the review pane, source review, a check's fix and
// the document view. The service checks it at commit, writes it through the
// format's writer, and records it in the project's block history with the
// desktop as its origin.

// changeTimeout bounds one call of a change binding.
const changeTimeout = 60 * time.Second

// desktopActor is who sends a change set from the desktop: the person at the
// keyboard.
var desktopActor = change.Actor{Kind: change.ActorPerson}

// changeServiceFor builds the change service for the project a tab has open,
// with the desktop as the origin it records. editions are the editions the
// call names: the one language among them other than the source is the
// language a bilingual file is read in (a PO catalog's msgstr), as kapi apply
// reads one.
func (a *App) changeServiceFor(ctx context.Context, tabID string, editions []model.EditionKey) (*change.Service, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("project has no recipe loaded")
	}
	source := project.NewProjectContext(op.Project, op.Path).SourceLocale
	return a.hostEngine().ChangeService(ctx, host.ChangeServiceOptions{
		Project:      op.Path,
		Origin:       "desktop",
		SourceLocale: source,
		TargetLocale: host.SoleTargetLocale(editions, source),
	})
}

// Read reads a page of a document's blocks: request is a change.ReadRequest
// and the answer a change.Page, each block with the reference and revision an
// operation names. A document is a project-relative path, or the file of one
// edition of it, whose read names that edition.
func (a *App) Read(tabID, request string) (string, error) {
	var q change.ReadRequest
	if err := decodeStrict(request, &q); err != nil {
		return "", fmt.Errorf("read request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	svc, err := a.changeServiceFor(ctx, tabID, q.Editions)
	if err != nil {
		return "", err
	}
	page, err := svc.Read(ctx, q)
	if err != nil {
		return "", err
	}
	return encodeChange(page)
}

// Apply applies a change set as the person using the desktop and returns the
// result, kapi.change-result/v1. A refusal is a result, never an error: a
// change set that does not decode answers with the result's own error, and a
// refused operation (a stale revision, a failing rule) with its error and, when
// stale, the edition as it now stands. An error means the change set could not
// be applied at all.
func (a *App) Apply(tabID, changeSet string) (string, error) {
	set, err := change.Decode(strings.NewReader(changeSet))
	if err != nil {
		if ce, ok := errors.AsType[*change.Error](err); ok && ce != nil {
			return encodeChange(change.ErrorResult(ce))
		}
		return encodeChange(change.ErrorResult(&change.Error{Code: change.CodeInvalid, Message: err.Error()}))
	}
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	svc, err := a.changeServiceFor(ctx, tabID, host.OpEditions(set))
	if err != nil {
		return "", err
	}
	res, err := svc.Apply(ctx, set, desktopActor)
	if err != nil {
		if ce, ok := errors.AsType[*change.Error](err); ok && ce != nil {
			return encodeChange(change.ErrorResult(ce))
		}
		return "", err
	}
	return encodeChange(res)
}

// Describe says what a format supports, or a document's format as its home
// writes it: request is a change.DescribeRequest and the answer a
// change.Description.
func (a *App) Describe(tabID, request string) (string, error) {
	var q change.DescribeRequest
	if err := decodeStrict(request, &q); err != nil {
		return "", fmt.Errorf("describe request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	svc, err := a.changeServiceFor(ctx, tabID, nil)
	if err != nil {
		return "", err
	}
	d, err := svc.Describe(ctx, q)
	if err != nil {
		return "", err
	}
	return encodeChange(d)
}

// History lists the recorded changes to one edition of a block, most recent
// first: request is a change.HistoryRequest and the answer a change.History,
// read from the project's block history (who changed the edition, through
// which surface, when, and the revisions around each change).
func (a *App) History(tabID, request string) (string, error) {
	var q change.HistoryRequest
	if err := decodeStrict(request, &q); err != nil {
		return "", fmt.Errorf("history request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	svc, err := a.changeServiceFor(ctx, tabID, []model.EditionKey{q.Ref.Edition})
	if err != nil {
		return "", err
	}
	h, err := svc.History(ctx, q)
	if err != nil {
		return "", err
	}
	return encodeChange(h)
}

// decodeStrict decodes one JSON request into v, refusing a field v does not
// declare, as the change service's own decoding does.
func decodeStrict(data string, v any) error {
	dec := json.NewDecoder(strings.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("the request holds more than one JSON value")
	}
	return nil
}

// encodeChange writes v as the contract's JSON, with HTML escaping off so the
// placeholders a block's text holds read as written.
func encodeChange(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("encode the answer: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}
