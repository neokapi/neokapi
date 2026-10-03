package projector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workspace"
)

// Edit is what a content.edit operation carries: one applied change to one
// document's content. A change set that touches several documents is one
// operation per document, each naming the same change set.
//
// What a record keeps depends on who wrote it. A person's or an agent's edit
// keeps the runs around each change and the change set as sent, in blobs; a
// flow's edit keeps the revisions and hashes only, because the file holds the
// text and volume is the cost.
type Edit struct {
	Doc EditDoc `json:"doc"`
	// Home is where the document's text lives: file, workspace or
	// stream:<id>.
	Home  string       `json:"home,omitempty"`
	Actor change.Actor `json:"actor"`
	// Origin says which surface applied the change, in By: apply, ksed, mcp,
	// browser, desktop, flow:<name>, merge, pull or observed.
	Origin Origin `json:"origin,omitzero"`
	// Fingerprint is the governance the commit check used.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Note is the change set's one line for history and review.
	Note string `json:"note,omitempty"`
	// DocBefore and DocAfter are the document's digests around the change.
	DocBefore   string           `json:"doc_before,omitempty"`
	DocAfter    string           `json:"doc_after,omitempty"`
	Transitions []EditTransition `json:"transitions"`
	// Overridden are the findings a person chose to land with gate: report.
	Overridden []change.Finding `json:"overridden,omitempty"`
	// ChangeSet names the blob holding the change set as sent
	// ("blob:sha256:…"), for a person's or an agent's edit.
	ChangeSet string `json:"change_set,omitempty"`
	// Blobs lists the address of every blob the edit names, which is how a
	// context backend's push and pull carry them with the operation
	// (workspace.BlobRefs).
	Blobs []string `json:"blobs,omitempty"`

	// SetJSON is the change set as sent. RecordEdit stores it in a blob and
	// names the blob in ChangeSet; nil records none.
	SetJSON []byte `json:"-"`
}

// EditDoc names the document an edit changed.
type EditDoc struct {
	// Key is the document's key (state.Adoption), which survives a rename.
	Key string `json:"key"`
	// Path is where the document was when the edit landed.
	Path string `json:"path,omitempty"`
}

// EditTransition is one edition an edit changed.
type EditTransition struct {
	// Block is the block as the read reported it, and Key its durable key
	// where reconciliation assigned one.
	Block string `json:"block"`
	Key   string `json:"key,omitempty"`
	// Edition is the edition's key in its text form ("en", "fr").
	Edition string `json:"edition"`
	// Before and After are the edition revisions around the change.
	Before string `json:"before"`
	After  string `json:"after"`
	// Basis is the authoritative edition's revision a derived edition was made
	// from, when the change recorded one.
	Basis string `json:"basis,omitempty"`
	// ContentHash and ContextHash are the block's identity signals after the
	// change, which core/reconcile matches a later read against.
	ContentHash string `json:"content_hash,omitempty"`
	ContextHash string `json:"context_hash,omitempty"`
	// Producer is how a tool in a flow produced a derived edition: its
	// provider, model and the governing context it was produced under. A
	// file that holds strings alone keeps none of it, so the record is where
	// the staleness gate finds what governed a translation the loop wrote.
	Producer *model.Origin `json:"producer,omitempty"`
	// RunsBefore and RunsAfter name the blobs holding the edition's runs
	// around the change ("blob:sha256:…"), each the canonical run JSON
	// model.RunsRevision is computed over. Empty for a hash-only record.
	RunsBefore string `json:"runs_before,omitempty"`
	RunsAfter  string `json:"runs_after,omitempty"`

	// BeforeRuns and AfterRuns are the runs RecordEdit stores in RunsBefore
	// and RunsAfter. A nil sequence stores nothing.
	BeforeRuns []model.Run `json:"-"`
	AfterRuns  []model.Run `json:"-"`
}

// blobRef is how a payload names a blob, beside the digests it also carries.
const blobRef = "blob:"

// BlobAddress returns the blob address a "blob:sha256:…" reference names,
// and false for anything else.
func BlobAddress(ref string) (string, bool) {
	if len(ref) <= len(blobRef) || ref[:len(blobRef)] != blobRef {
		return "", false
	}
	return ref[len(blobRef):], true
}

// RecordEdit records an applied edit to one document as a content.edit
// operation and writes it into the block history, and returns the
// operation's id. It is RecordEdits for one edit.
func (p *Projector) RecordEdit(ctx context.Context, e Edit) (string, error) {
	ids, err := p.RecordEdits(ctx, []Edit{e})
	if len(ids) == 0 {
		return "", err
	}
	return ids[0], err
}

// RecordEdits records applied edits, one content.edit operation each, in one
// write to the log, writes them into the block history, and returns the
// operations' ids in order. A change set that touched several documents, or a
// flow's pass over a collection, records its documents together.
//
// Each operation is addressed by the document, the actor and each transition,
// with the address of the operation that left the edition at the revision the
// transition starts from. Addresses are the same in every log, so recording
// one edit twice (a retry, two machines noticing one change made outside
// kapi) is one operation wherever it is recorded, and so is every change that
// extends it. The same change made again after it was undone extends a later
// operation and is another. Within one call, an edit that extends an earlier
// one in the same call is chained to that edit's address. A call that returns
// an edition to a revision it started from therefore records the later
// changes again when it is retried after it landed; a change set changes each
// edition once, and a flow records each document once per pass.
//
// A store with no log (the embedded layout) has no blob store and nothing to
// address against: the records are written straight into the history,
// hash-only, under ids minted here, each its own address.
func (p *Projector) RecordEdits(ctx context.Context, edits []Edit) ([]string, error) {
	for _, e := range edits {
		if e.Doc.Key == "" {
			return nil, errors.New("projector: an edit names no document")
		}
		if len(e.Transitions) == 0 {
			return nil, errors.New("projector: an edit with no transitions records nothing")
		}
	}
	if len(edits) == 0 {
		return nil, nil
	}
	// The blobs are named on copies, so the caller's edits stay as given.
	edits = slices.Clone(edits)
	for i := range edits {
		edits[i].Transitions = slices.Clone(edits[i].Transitions)
		edits[i].Blobs = slices.Clone(edits[i].Blobs)
	}
	hs := p.st.History
	if hs == nil {
		return nil, errNoSubsystem
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	now := time.Now().UTC()
	if p.log == nil {
		ids := make([]string, 0, len(edits))
		var latest string
		for _, e := range edits {
			head, err := hs.DocumentHead(ctx, e.Doc.Key)
			if err != nil {
				return ids, err
			}
			id := workspace.NewOpID(now, max(latest, head))
			if err := hs.Put(ctx, editRows(id, id, now, e)); err != nil {
				return ids, err
			}
			ids, latest = append(ids, id), id
		}
		return ids, nil
	}
	if err := p.catchUpLocked(ctx, nil); err != nil {
		return nil, err
	}

	// reached holds, per document, the address that left each edition at each
	// revision: read from the history for the revisions an edit starts from,
	// and moved on by each edit in this call.
	reached := map[string]map[history.Reach]string{}
	ops := make([]workspace.Op, 0, len(edits))
	for i := range edits {
		e := &edits[i]
		doc := reached[e.Doc.Key]
		if doc == nil {
			doc = map[history.Reach]string{}
			reached[e.Doc.Key] = doc
		}
		var ask []history.Reach
		for _, t := range e.Transitions {
			at := history.Reach{Block: t.Block, Edition: t.Edition, Rev: t.Before}
			if _, known := doc[at]; !known {
				ask = append(ask, at)
			}
		}
		found, err := hs.Reached(ctx, e.Doc.Key, ask)
		if err != nil {
			return nil, err
		}
		for _, at := range ask {
			doc[at] = found[at]
		}
		if err := p.storeEditBlobs(ctx, e); err != nil {
			return nil, err
		}
		op, err := p.encodeEdit(ctx, *e, now)
		if err != nil {
			return nil, err
		}
		op.Address = editAddress(p.key, *e, doc)
		for _, t := range e.Transitions {
			doc[history.Reach{Block: t.Block, Edition: t.Edition, Rev: t.After}] = op.Address
		}
		ops = append(ops, op)
	}
	written, err := p.log.Record(ctx, ops...)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(written))
	mine := make(map[string]pending, len(written))
	for i, op := range written {
		ids[i] = op.ID
		mine[op.ID] = pending{kind: KindEdit, edit: &edits[i]}
	}
	return ids, p.catchUpLocked(ctx, mine)
}

// storeEditBlobs moves the runs and the change set an edit keeps into blobs,
// and names each in the payload.
func (p *Projector) storeEditBlobs(ctx context.Context, e *Edit) error {
	put := func(data []byte) (string, error) {
		address, err := p.log.PutBlob(ctx, data)
		if err != nil {
			return "", fmt.Errorf("projector: store an edit's blob: %w", err)
		}
		if !slices.Contains(e.Blobs, address) {
			e.Blobs = append(e.Blobs, address)
		}
		return blobRef + address, nil
	}
	for i := range e.Transitions {
		t := &e.Transitions[i]
		if t.BeforeRuns != nil {
			ref, err := put(model.CanonicalRunsJSON(t.BeforeRuns))
			if err != nil {
				return err
			}
			t.RunsBefore = ref
		}
		if t.AfterRuns != nil {
			ref, err := put(model.CanonicalRunsJSON(t.AfterRuns))
			if err != nil {
				return err
			}
			t.RunsAfter = ref
		}
	}
	if e.SetJSON != nil {
		ref, err := put(e.SetJSON)
		if err != nil {
			return err
		}
		e.ChangeSet = ref
	}
	return nil
}

// editBlob is the payload of a content.edit operation whose edit is too large
// to carry: the blob holding it, and the blobs the edit names.
type editBlob struct {
	Blob  string   `json:"blob"`
	Blobs []string `json:"blobs,omitempty"`
}

// encodeEdit renders an edit as the operation that records it, moving the
// edit into a blob when it is too large to carry.
func (p *Projector) encodeEdit(ctx context.Context, e Edit, at time.Time) (workspace.Op, error) {
	body, err := json.Marshal(e)
	if err != nil {
		return workspace.Op{}, fmt.Errorf("projector: encode %s: %w", KindEdit, err)
	}
	if len(body) > BlobThreshold {
		address, err := p.log.PutBlob(ctx, body)
		if err != nil {
			return workspace.Op{}, err
		}
		if body, err = json.Marshal(editBlob{Blob: address, Blobs: e.Blobs}); err != nil {
			return workspace.Op{}, fmt.Errorf("projector: encode %s: %w", KindEdit, err)
		}
	}
	return workspace.Op{Project: p.key, Kind: KindEdit, Payload: body, At: at}, nil
}

// decodeEdit reads the edit a content.edit operation carries, from its
// payload or its blob.
func (p *Projector) decodeEdit(ctx context.Context, op workspace.Op) (Edit, error) {
	var ref editBlob
	if err := json.Unmarshal(op.Payload, &ref); err != nil {
		return Edit{}, fmt.Errorf("projector: read %s %s: %w", op.Kind, workspace.ShortOpID(op.ID), err)
	}
	data := op.Payload
	if ref.Blob != "" {
		blob, err := p.log.Blob(ctx, ref.Blob)
		if err != nil {
			return Edit{}, fmt.Errorf("projector: read %s %s: %w", op.Kind, workspace.ShortOpID(op.ID), err)
		}
		data = blob
	}
	var e Edit
	if err := json.Unmarshal(data, &e); err != nil {
		return Edit{}, fmt.Errorf("projector: read %s %s: %w", op.Kind, workspace.ShortOpID(op.ID), err)
	}
	return e, nil
}

// editRows renders an edit as the block-history rows it projects to, under
// the operation's id and content address.
func editRows(op, address string, at time.Time, e Edit) []history.Row {
	rows := make([]history.Row, 0, len(e.Transitions))
	for _, t := range e.Transitions {
		row := history.Row{
			Op: op, Address: address, Doc: e.Doc.Key, Block: t.Block, Key: t.Key, Edition: t.Edition,
			Before: t.Before, After: t.After, Basis: t.Basis,
			ContentHash: t.ContentHash, ContextHash: t.ContextHash,
			Actor: string(e.Actor.Kind), ActorName: e.Actor.Name, Session: e.Actor.Session,
			Origin: e.Origin.By, At: at,
		}
		if t.Producer != nil {
			row.Producer = *t.Producer
		}
		rows = append(rows, row)
	}
	return rows
}

// editAddress is the content address of an edit: the project, the document,
// the actor, and each transition with the address of the operation it
// extends.
func editAddress(key workspace.ProjectKey, e Edit, reached map[history.Reach]string) string {
	parts := []string{string(key), e.Doc.Key, string(e.Actor.Kind), e.Actor.Name, e.Actor.Session}
	for _, t := range e.Transitions {
		parts = append(parts, t.Block, t.Key, t.Edition, t.Before, t.After, t.Basis,
			reached[history.Reach{Block: t.Block, Edition: t.Edition, Rev: t.Before}])
	}
	return "edit:" + string(key) + ":" + digestOf(parts...)
}

// opAddress is the address an operation's block-history rows are keyed by:
// its content address, or its id for one recorded without an address.
func opAddress(op workspace.Op) string {
	if op.Address != "" {
		return op.Address
	}
	return op.ID
}

// applyEditRows writes block-history rows a run of content.edit operations
// projects to.
func (p *Projector) applyEditRows(ctx context.Context, rows []history.Row) error {
	if len(rows) == 0 {
		return nil
	}
	if p.st.History == nil {
		return errNoSubsystem
	}
	return p.st.History.Put(ctx, rows)
}
