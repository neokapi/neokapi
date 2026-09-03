package backend

import (
	"context"
	"encoding/json"

	"github.com/neokapi/neokapi/bowrain/editorclient"
	"github.com/neokapi/neokapi/core/model"
)

// opKind is the sealed set of offline mutation kinds the desktop outbox can
// carry. Each value doubles as the persisted `operation` column of the offline
// queue, so these string literals are a stable on-disk contract — do not rename
// them (older queued entries must still decode after an upgrade).
type opKind string

const (
	opUpdateBlockTarget     opKind = "update_block_target"
	opUpdateBlockTargetRuns opKind = "update_block_target_runs"
	opReviewBlock           opKind = "review_block"
	opAddMemoryEntry        opKind = "add_tm_entry"
	opUpdateMemoryEntry     opKind = "update_tm_entry"
	opDeleteMemoryEntry     opKind = "delete_tm_entry"
	opAddConcept            opKind = "add_concept"
	opUpdateConcept         opKind = "update_concept"
	opDeleteConcept         opKind = "delete_concept"
	opAddItems              opKind = "add_items"
	opRemoveItem            opKind = "remove_item"
	opPseudoTranslateItem   opKind = "pseudo_translate_item"
	opMemoryTranslateItem   opKind = "tm_translate_item"
)

// offlineOp is a typed, self-describing offline mutation: it knows its kind
// (the persisted operation tag) and how to replay itself against the REST
// editor client when the connection is restored. The concrete ops below are the
// only values ever enqueued, so the outbox is typed end-to-end — call sites
// enqueue an offlineOp and replay dispatches on the typed op via decodeOp,
// never on a bare string.
type offlineOp interface {
	opKind() opKind
	replay(ctx context.Context, client *editorclient.EditorClient, ws string) error
}

// decodeOp reconstructs the typed op persisted under kind+payload. It returns
// (nil, nil) for an unknown kind so replay can skip a forward-incompatible queue
// entry written by a newer build.
func decodeOp(kind opKind, payload string) (offlineOp, error) {
	switch kind {
	case opUpdateBlockTarget:
		return unmarshalOp[updateBlockTargetOp](payload)
	case opUpdateBlockTargetRuns:
		return unmarshalOp[updateBlockTargetRunsOp](payload)
	case opReviewBlock:
		return unmarshalOp[reviewBlockOp](payload)
	case opAddMemoryEntry:
		return unmarshalOp[addMemoryEntryOp](payload)
	case opUpdateMemoryEntry:
		return unmarshalOp[updateMemoryEntryOp](payload)
	case opDeleteMemoryEntry:
		return unmarshalOp[deleteMemoryEntryOp](payload)
	case opAddConcept:
		return unmarshalOp[addConceptOp](payload)
	case opUpdateConcept:
		return unmarshalOp[updateConceptOp](payload)
	case opDeleteConcept:
		return unmarshalOp[deleteConceptOp](payload)
	case opAddItems:
		return unmarshalOp[addItemsOp](payload)
	case opRemoveItem:
		return unmarshalOp[removeItemOp](payload)
	case opPseudoTranslateItem:
		return unmarshalOp[pseudoTranslateItemOp](payload)
	case opMemoryTranslateItem:
		return unmarshalOp[memoryTranslateItemOp](payload)
	default:
		return nil, nil
	}
}

// unmarshalOp decodes a JSON payload into the concrete op type T.
func unmarshalOp[T offlineOp](payload string) (offlineOp, error) {
	var op T
	if err := json.Unmarshal([]byte(payload), &op); err != nil {
		return nil, err
	}
	return op, nil
}

// --- Block editor ops ---

// updateBlockTargetOp queues a plain-text target update. It embeds the request
// so the persisted JSON is identical to the original UpdateBlockRequest payload.
type updateBlockTargetOp struct{ UpdateBlockRequest }

func (updateBlockTargetOp) opKind() opKind { return opUpdateBlockTarget }

func (o updateBlockTargetOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	// Plain-text replay: emit a single TextRun so the server receives the
	// canonical Run sequence.
	runs := []model.Run{{Text: &model.TextRun{Text: o.Text}}}
	return client.UpdateBlockTargetRuns(ctx, ws, o.ProjectID, o.BlockID, o.TargetLocale, runs)
}

// updateBlockTargetRunsOp queues a structured Run-sequence target update.
type updateBlockTargetRunsOp struct{ UpdateBlockTargetRunsRequest }

func (updateBlockTargetRunsOp) opKind() opKind { return opUpdateBlockTargetRuns }

func (o updateBlockTargetRunsOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	return client.UpdateBlockTargetRuns(ctx, ws, o.ProjectID, o.BlockID, o.TargetLocale, runInfosToRuns(o.Runs))
}

// reviewBlockOp queues a block review/un-review. Status is the optional rung,
// mirroring the server's review body: "signed-off" on an approval, "draft" on
// a clearing call for a rejection, empty for the default rung either way.
type reviewBlockOp struct {
	ProjectID    string `json:"project_id"`
	ItemName     string `json:"item_name"`
	BlockID      string `json:"block_id"`
	TargetLocale string `json:"target_locale"`
	Reviewed     bool   `json:"reviewed"`
	Status       string `json:"status,omitempty"`
}

func (reviewBlockOp) opKind() opKind { return opReviewBlock }

func (o reviewBlockOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	return client.ReviewBlock(ctx, ws, o.ProjectID, o.ItemName, o.BlockID, o.TargetLocale, o.Reviewed, o.Status)
}

// --- Translation-memory ops ---

// addMemoryEntryOp queues a new content-memory entry.
type addMemoryEntryOp struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
}

func (addMemoryEntryOp) opKind() opKind { return opAddMemoryEntry }

func (o addMemoryEntryOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	_, err := client.AddMemoryEntry(ctx, ws, o.Source, o.Target, o.SourceLocale, o.TargetLocale)
	return err
}

// updateMemoryEntryOp queues an edit to an existing content-memory entry.
type updateMemoryEntryOp struct{ MemoryUpdateRequest }

func (updateMemoryEntryOp) opKind() opKind { return opUpdateMemoryEntry }

func (o updateMemoryEntryOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	return client.UpdateMemoryEntry(ctx, ws, o.EntryID, o.Source, o.Target, o.SourceLocale, o.TargetLocale)
}

// deleteMemoryEntryOp queues a content-memory entry deletion.
type deleteMemoryEntryOp struct {
	EntryID string `json:"entry_id"`
}

func (deleteMemoryEntryOp) opKind() opKind { return opDeleteMemoryEntry }

func (o deleteMemoryEntryOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	return client.DeleteMemoryEntry(ctx, ws, o.EntryID)
}

// --- Terms (concept) ops ---

// addConceptOp queues a new concept.
type addConceptOp struct{ AddConceptRequest }

func (addConceptOp) opKind() opKind { return opAddConcept }

func (o addConceptOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	_, err := client.EditorAddConcept(ctx, ws, o.Domain, o.Definition, termInfosToEditor(o.Terms))
	return err
}

// updateConceptOp queues an edit to an existing concept.
type updateConceptOp struct{ UpdateConceptRequest }

func (updateConceptOp) opKind() opKind { return opUpdateConcept }

func (o updateConceptOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	return client.EditorUpdateConcept(ctx, ws, o.ConceptID, o.Domain, o.Definition, termInfosToEditor(o.Terms))
}

// deleteConceptOp queues a concept deletion.
type deleteConceptOp struct {
	ConceptID string `json:"concept_id"`
}

func (deleteConceptOp) opKind() opKind { return opDeleteConcept }

func (o deleteConceptOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	return client.EditorDeleteConcept(ctx, ws, o.ConceptID)
}

// --- Item ops ---

// addItemsOp queues a file upload — the raw bytes travel base64-encoded in the
// JSON payload so the upload can replay on reconnect.
type addItemsOp struct {
	ProjectID string            `json:"project_id"`
	Files     map[string][]byte `json:"files"`
}

func (addItemsOp) opKind() opKind { return opAddItems }

func (o addItemsOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	_, err := client.UploadItems(ctx, ws, o.ProjectID, o.Files)
	return err
}

// removeItemOp queues an item removal.
type removeItemOp struct {
	ProjectID string `json:"project_id"`
	ItemName  string `json:"item_name"`
}

func (removeItemOp) opKind() opKind { return opRemoveItem }

func (o removeItemOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	_, err := client.RemoveItem(ctx, ws, o.ProjectID, o.ItemName)
	return err
}

// pseudoTranslateItemOp queues a bulk pseudo-translate action for an item.
type pseudoTranslateItemOp struct {
	ProjectID    string `json:"project_id"`
	ItemName     string `json:"item_name"`
	TargetLocale string `json:"target_locale"`
}

func (pseudoTranslateItemOp) opKind() opKind { return opPseudoTranslateItem }

func (o pseudoTranslateItemOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	_, err := client.PseudoTranslateItem(ctx, ws, o.ProjectID, o.ItemName, o.TargetLocale)
	return err
}

// memoryTranslateItemOp queues a bulk content memory-translate action for an item.
type memoryTranslateItemOp struct {
	ProjectID    string `json:"project_id"`
	ItemName     string `json:"item_name"`
	TargetLocale string `json:"target_locale"`
}

func (memoryTranslateItemOp) opKind() opKind { return opMemoryTranslateItem }

func (o memoryTranslateItemOp) replay(ctx context.Context, client *editorclient.EditorClient, ws string) error {
	_, err := client.MemoryTranslateItem(ctx, ws, o.ProjectID, o.ItemName, o.TargetLocale)
	return err
}
