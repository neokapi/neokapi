package backend

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"runtime"

	"github.com/neokapi/neokapi/bowrain/editorclient"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
	"github.com/neokapi/neokapi/terms"
)

// GetItemBlocks returns all blocks for an item in the project.
// When connected, blocks are fetched from the server and cached locally.
// On connection failure, falls back to the local cache.
// PendingReviewEntryView is one entry of the translation review queue as the
// frontend consumes it.
type PendingReviewEntryView struct {
	BlockID  string     `json:"block_id"`
	ItemName string     `json:"item_name"`
	Locale   string     `json:"locale"`
	Block    *BlockInfo `json:"block,omitempty"`
	// PreReview is an agent's pre-review of the translation, as the server
	// serves it; the local working copy keeps none.
	PreReview *editorclient.EditorPreReview `json:"pre_review,omitempty"`
}

// PendingReviewPageView is one page of the queue plus its total size.
type PendingReviewPageView struct {
	Entries []PendingReviewEntryView `json:"entries"`
	Total   int                      `json:"total"`
	Limit   int                      `json:"limit"`
	Offset  int                      `json:"offset"`
}

// GetPendingReview pages the translation review queue: server-side when
// connected, from the local store offline — the same predicate either way.
func (a *App) GetPendingReview(projectID string, locales []string, limit, offset int) (*PendingReviewPageView, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		page, err := client.GetPendingReview(context.Background(), ws, projectID, locales, limit, offset)
		if err != nil {
			a.goOffline()
			return a.getPendingReviewLocal(projectID, locales, limit, offset)
		}
		out := &PendingReviewPageView{Total: page.Total, Limit: page.Limit, Offset: page.Offset}
		for _, e := range page.Entries {
			view := PendingReviewEntryView{BlockID: e.BlockID, ItemName: e.ItemName, Locale: e.Locale, PreReview: e.PreReview}
			if e.Block != nil {
				infos := editorBlocksToInfos([]editorclient.EditorBlock{*e.Block})
				if len(infos) == 1 {
					view.Block = &infos[0]
				}
			}
			out.Entries = append(out.Entries, view)
		}
		return out, nil
	}
	return a.getPendingReviewLocal(projectID, locales, limit, offset)
}

func (a *App) GetItemBlocks(projectID, itemName string) ([]BlockInfo, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		remoteBlocks, err := client.GetEditorBlocks(context.Background(), ws, projectID, itemName)
		if err != nil {
			// Connection failed — fall back to offline mode.
			a.goOffline()
			return a.getItemBlocksLocal(projectID, itemName)
		}
		blocks := editorBlocksToInfos(remoteBlocks)
		// Cache the blocks locally for offline access.
		a.cacheBlocks(projectID, itemName, blocks)
		return blocks, nil
	}
	return a.getItemBlocksLocal(projectID, itemName)
}

// getPendingReviewLocal answers the queue from the local store — the same
// predicate the server runs, so online and offline agree on what is pending.
func (a *App) getPendingReviewLocal(projectID string, locales []string, limit, offset int) (*PendingReviewPageView, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	targetLocales := make([]string, len(proj.TargetLanguages))
	for i, l := range proj.TargetLanguages {
		targetLocales[i] = string(l)
	}

	// No collection scope: the desktop's queue is the whole project's. The
	// filter exists for the web review session, which is entered from a
	// collection card.
	refs, total, err := a.store.ListPendingReview(ctx, store.PendingReviewQuery{
		ProjectID: projectID,
		Stream:    "main",
		Locales:   locales,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, r := range refs {
		if !seen[r.BlockID] {
			seen[r.BlockID] = true
			ids = append(ids, r.BlockID)
		}
	}
	byID := map[string]*BlockInfo{}
	if len(ids) > 0 {
		stored, err := a.store.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main", IDs: ids})
		if err != nil {
			return nil, err
		}
		for _, sb := range stored {
			bi := storedBlockToBlockInfo(sb, targetLocales)
			byID[bi.ID] = &bi
		}
	}
	out := &PendingReviewPageView{Total: total, Limit: limit, Offset: offset}
	for _, r := range refs {
		out.Entries = append(out.Entries, PendingReviewEntryView{
			BlockID: r.BlockID, ItemName: r.ItemName, Locale: r.Locale, Block: byID[r.BlockID],
		})
	}
	return out, nil
}

func (a *App) getItemBlocksLocal(projectID, itemName string) ([]BlockInfo, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	targetLocales := make([]string, len(proj.TargetLanguages))
	for i, l := range proj.TargetLanguages {
		targetLocales[i] = string(l)
	}

	storedBlocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	blocks := make([]BlockInfo, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		bi := storedBlockToBlockInfo(sb, targetLocales)
		blocks = append(blocks, bi)
	}
	return blocks, nil
}

// EditorBlockFilter narrows a block page. Every field is optional: the zero
// value pages an item's blocks unfiltered. Status names one per-locale bucket
// and needs Locale, which also scopes the target side of Text.
type EditorBlockFilter struct {
	Locale       string `json:"locale,omitempty"`
	Status       string `json:"status,omitempty"`
	Text         string `json:"q,omitempty"`
	Translatable *bool  `json:"translatable,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Offset       int    `json:"offset,omitempty"`
}

func (f EditorBlockFilter) remote(itemName string) editorclient.EditorBlockQuery {
	return editorclient.EditorBlockQuery{
		ItemName:     itemName,
		Locale:       f.Locale,
		Status:       f.Status,
		Text:         f.Text,
		Translatable: f.Translatable,
		Limit:        f.Limit,
		Offset:       f.Offset,
	}
}

func (f EditorBlockFilter) local(projectID, itemName string) store.BlockQuery {
	return store.BlockQuery{
		ProjectID:    projectID,
		Stream:       "main",
		ItemName:     itemName,
		TargetLocale: f.Locale,
		Status:       f.Status,
		Text:         f.Text,
		Translatable: f.Translatable,
		Limit:        f.Limit,
		Offset:       f.Offset,
	}
}

// QueryItemBlocks pages an item's blocks through the same filters the server
// applies: server-side when connected, from the local store offline.
func (a *App) QueryItemBlocks(projectID, itemName string, filter EditorBlockFilter) ([]BlockInfo, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		remote, err := client.QueryEditorBlocks(context.Background(), ws, projectID, filter.remote(itemName))
		if err != nil {
			a.goOffline()
			return a.queryItemBlocksLocal(projectID, itemName, filter)
		}
		return editorBlocksToInfos(remote), nil
	}
	return a.queryItemBlocksLocal(projectID, itemName, filter)
}

func (a *App) queryItemBlocksLocal(projectID, itemName string, filter EditorBlockFilter) ([]BlockInfo, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	targetLocales := make([]string, len(proj.TargetLanguages))
	for i, l := range proj.TargetLanguages {
		targetLocales[i] = string(l)
	}
	stored, err := a.store.GetBlocks(ctx, filter.local(projectID, itemName))
	if err != nil {
		return nil, err
	}
	blocks := make([]BlockInfo, 0, len(stored))
	for _, sb := range stored {
		blocks = append(blocks, storedBlockToBlockInfo(sb, targetLocales))
	}
	return blocks, nil
}

// GetBlock returns one block in the same shape the block page returns its
// elements: server-side when connected, from the local working copy offline.
//
// It is what a surface reads after writing a target, instead of rebuilding the
// block from its own request: the status an edit leaves is the change
// service's decision on both paths (change.ApplyBlock), and a second copy of
// that rule in TypeScript is a copy that can disagree.
func (a *App) GetBlock(projectID, blockID string) (*BlockInfo, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		remote, err := client.GetEditorBlock(context.Background(), ws, projectID, blockID)
		if err == nil {
			infos := editorBlocksToInfos([]editorclient.EditorBlock{*remote})
			if len(infos) == 1 {
				return &infos[0], nil
			}
		}
		a.goOffline()
	}
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	targetLocales := make([]string, len(proj.TargetLanguages))
	for i, l := range proj.TargetLanguages {
		targetLocales[i] = string(l)
	}
	sb, err := a.store.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return nil, err
	}
	bi := storedBlockToBlockInfo(sb, targetLocales)
	return &bi, nil
}

// BlockStatusCountsView is the per-locale status histogram.
type BlockStatusCountsView struct {
	NotStarted  int `json:"not-started"`
	Draft       int `json:"draft"`
	Translated  int `json:"translated"`
	Established int `json:"established"`
}

// BlockCountsView is a block query's totals and histogram.
type BlockCountsView struct {
	Total        int                   `json:"total"`
	Translatable int                   `json:"translatable"`
	Locale       string                `json:"locale,omitempty"`
	Status       BlockStatusCountsView `json:"status"`
}

// GetBlockCounts answers an item's progress with one aggregate query. The
// filter's Status is ignored — the histogram is what the call reports.
func (a *App) GetBlockCounts(projectID, itemName string, filter EditorBlockFilter) (*BlockCountsView, error) {
	filter.Status = ""
	if a.isConnected() {
		client, ws := a.editorRemote()
		counts, err := client.GetEditorBlockCounts(context.Background(), ws, projectID, filter.remote(itemName))
		if err == nil {
			return &BlockCountsView{
				Total:        counts.Total,
				Translatable: counts.Translatable,
				Locale:       counts.Locale,
				Status: BlockStatusCountsView{
					NotStarted:  counts.Status.NotStarted,
					Draft:       counts.Status.Draft,
					Translated:  counts.Status.Translated,
					Established: counts.Status.Established,
				},
			}, nil
		}
		a.goOffline()
	}
	query := filter.local(projectID, itemName)
	query.Limit, query.Offset = 0, 0
	counts, err := a.store.CountBlocks(context.Background(), query)
	if err != nil {
		return nil, err
	}
	return &BlockCountsView{
		Total:        counts.Total,
		Translatable: counts.Translatable,
		Locale:       filter.Locale,
		Status: BlockStatusCountsView{
			NotStarted:  counts.NotStarted,
			Draft:       counts.Draft,
			Translated:  counts.Translated,
			Established: counts.Established,
		},
	}, nil
}

// ItemView is one item's metadata and block tallies.
type ItemView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Format       string `json:"format"`
	Type         string `json:"type"`
	CollectionID string `json:"collection_id,omitempty"`
	BlockCount   int    `json:"block_count"`
	Translatable int    `json:"translatable"`
}

// GetItem returns one item's metadata without the project's whole item list.
func (a *App) GetItem(projectID, itemName string) (*ItemView, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		item, err := client.GetEditorItem(context.Background(), ws, projectID, itemName)
		if err == nil {
			return &ItemView{
				ID:           item.ID,
				Name:         item.Name,
				Format:       item.Format,
				Type:         item.Type,
				CollectionID: item.CollectionID,
				BlockCount:   item.BlockCount,
				Translatable: item.Translatable,
			}, nil
		}
		a.goOffline()
	}
	ctx := context.Background()
	item, err := a.store.GetItem(ctx, projectID, "main", itemName)
	if err != nil {
		return nil, err
	}
	counts, err := a.store.CountBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: item.Name,
	})
	if err != nil {
		return nil, err
	}
	return &ItemView{
		ID:           item.ID,
		Name:         item.Name,
		Format:       item.Format,
		Type:         item.ItemType,
		CollectionID: item.CollectionID,
		BlockCount:   counts.Total,
		Translatable: counts.Translatable,
	}, nil
}

// BulkReviewArgs applies one review decision to a selection of blocks. Status
// picks the demotion rung when Approve is false: "translated" (the default) or
// "draft", a rejection that re-opens the work.
type BulkReviewArgs struct {
	BlockIDs     []string `json:"block_ids"`
	TargetLocale string   `json:"target_locale"`
	Approve      bool     `json:"approve"`
	Status       string   `json:"status,omitempty"`
	Comment      string   `json:"comment,omitempty"`
	ItemName     string   `json:"item_name,omitempty"`
}

// BlockResultView is one block's outcome inside a batch.
type BlockResultView struct {
	BlockID string `json:"block_id"`
	OK      bool   `json:"ok"`
	Status  string `json:"status,omitempty"`
	Error   string `json:"error,omitempty"`
}

// BulkReviewView reports a batch review.
type BulkReviewView struct {
	Results         []BlockResultView `json:"results"`
	Succeeded       int               `json:"succeeded"`
	Failed          int               `json:"failed"`
	ReviewCompleted bool              `json:"review_completed"`
}

// BulkReviewBlocks applies one review decision across a selection of blocks.
// Connected it is one request; offline each block goes through the same local
// review path a single decision takes, so a block that refuses is reported in
// its own result either way.
func (a *App) BulkReviewBlocks(projectID string, req BulkReviewArgs) (*BulkReviewView, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		resp, err := client.BulkReviewBlocks(context.Background(), ws, projectID, editorclient.EditorBulkReviewRequest{
			BlockIDs:     req.BlockIDs,
			TargetLocale: req.TargetLocale,
			Approve:      req.Approve,
			Status:       req.Status,
			Comment:      req.Comment,
			ItemName:     req.ItemName,
		})
		if err == nil {
			out := &BulkReviewView{
				Succeeded:       resp.Succeeded,
				Failed:          resp.Failed,
				ReviewCompleted: resp.ReviewCompleted,
				Results:         make([]BlockResultView, 0, len(resp.Results)),
			}
			for _, r := range resp.Results {
				out.Results = append(out.Results, BlockResultView{
					BlockID: r.BlockID, OK: r.OK, Status: r.Status, Error: r.Error,
				})
			}
			if req.ItemName != "" {
				a.refreshItemCache(projectID, req.ItemName)
			}
			return out, nil
		}
		a.goOffline()
	}

	out := &BulkReviewView{Results: make([]BlockResultView, 0, len(req.BlockIDs))}
	for _, id := range req.BlockIDs {
		if err := a.reviewBlockLocal(projectID, id, req.TargetLocale, req.Approve, req.Status); err != nil {
			out.Failed++
			out.Results = append(out.Results, BlockResultView{BlockID: id, Error: err.Error()})
			continue
		}
		status := string(model.TargetStatusEstablished)
		if !req.Approve {
			status = string(model.TargetStatusTranslated)
			if req.Status == string(model.TargetStatusDraft) {
				status = string(model.TargetStatusDraft)
			}
		}
		out.Succeeded++
		out.Results = append(out.Results, BlockResultView{BlockID: id, OK: true, Status: status})
	}
	return out, nil
}

// BulkApplyMemoryArgs applies the best content-memory match to a selection of
// blocks. A zero Threshold takes the server default of 1 — an exact match.
type BulkApplyMemoryArgs struct {
	BlockIDs     []string `json:"block_ids"`
	TargetLocale string   `json:"target_locale"`
	Threshold    float64  `json:"threshold,omitempty"`
}

// AppliedMemoryView names a block that took a match, and what it took.
type AppliedMemoryView struct {
	BlockID string  `json:"block_id"`
	Text    string  `json:"text"`
	Score   float64 `json:"score"`
}

// SkippedMemoryView names a block that took nothing, and why.
type SkippedMemoryView struct {
	BlockID string `json:"block_id"`
	Reason  string `json:"reason"`
}

// BulkApplyMemoryView reports a batch content-memory apply.
type BulkApplyMemoryView struct {
	Applied []AppliedMemoryView `json:"applied"`
	Skipped []SkippedMemoryView `json:"skipped"`
}

// BulkApplyMemory writes the best content-memory match above the threshold
// into each selected block's target. The match is resolved against the
// workspace memory the server holds, so offline every block is skipped rather
// than matched against a different corpus.
func (a *App) BulkApplyMemory(projectID string, req BulkApplyMemoryArgs) (*BulkApplyMemoryView, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		body := editorclient.EditorBulkApplyMemoryRequest{
			BlockIDs:     req.BlockIDs,
			TargetLocale: req.TargetLocale,
		}
		if req.Threshold > 0 {
			body.Threshold = &req.Threshold
		}
		resp, err := client.BulkApplyMemory(context.Background(), ws, projectID, body)
		if err == nil {
			out := &BulkApplyMemoryView{
				Applied: make([]AppliedMemoryView, 0, len(resp.Applied)),
				Skipped: make([]SkippedMemoryView, 0, len(resp.Skipped)),
			}
			for _, ap := range resp.Applied {
				out.Applied = append(out.Applied, AppliedMemoryView{BlockID: ap.BlockID, Text: ap.Text, Score: ap.Score})
			}
			for _, sk := range resp.Skipped {
				out.Skipped = append(out.Skipped, SkippedMemoryView{BlockID: sk.BlockID, Reason: sk.Reason})
			}
			return out, nil
		}
		a.goOffline()
	}

	out := &BulkApplyMemoryView{
		Applied: []AppliedMemoryView{},
		Skipped: make([]SkippedMemoryView, 0, len(req.BlockIDs)),
	}
	for _, id := range req.BlockIDs {
		out.Skipped = append(out.Skipped, SkippedMemoryView{BlockID: id, Reason: "offline"})
	}
	return out, nil
}

// refreshItemCache re-fetches an item's blocks from the server and updates the
// local cache. Called after a server-side bulk action so an offline reader sees
// the mutated content. Best-effort: cache-refresh failures are non-fatal.
func (a *App) refreshItemCache(projectID, itemName string) {
	client, ws := a.editorRemote()
	if client == nil {
		return
	}
	remoteBlocks, err := client.GetEditorBlocks(context.Background(), ws, projectID, itemName)
	if err != nil {
		return
	}
	a.cacheBlocks(projectID, itemName, editorBlocksToInfos(remoteBlocks))
}

// cacheBlocks stores server-fetched blocks in the local ContentStore for offline access.
func (a *App) cacheBlocks(projectID, itemName string, blocks []BlockInfo) {
	ctx := context.Background()

	// Ensure the project exists locally for caching.
	_, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		// Project not cached yet — create a minimal placeholder.
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		_ = a.store.CreateProject(ctx, &store.Project{
			ID:          projectID,
			Name:        projectID, // minimal; real name comes from GetProject
			WorkspaceID: ws,
		})
	}

	// Convert BlockInfos back to model.Blocks for storage.
	var modelBlocks []*model.Block
	for _, bi := range blocks {
		b := blockInfoToBlock(bi)
		modelBlocks = append(modelBlocks, b)
	}

	if len(modelBlocks) > 0 {
		// Use StoreBlocks (not StoreBlocksForItem) because the blocks already
		// carry internal IDs from the server — they should not be re-mapped.
		_ = a.store.StoreBlocks(ctx, projectID, "main", modelBlocks)
	}
}

// blockInfoToBlock converts a BlockInfo (from server) to a model.Block for local storage.
// The server serves a plain source as its text alone, and a plain translation
// as its text in the targets map.
func blockInfoToBlock(bi BlockInfo) *model.Block {
	source := bi.SourceRuns
	if len(source) == 0 && bi.Source != "" {
		source = []model.Run{model.TextR(bi.Source)}
	}
	b := model.NewRunsBlock(bi.ID, source)
	b.Name = bi.Name
	b.Translatable = bi.Translatable
	b.Properties = bi.Properties
	for locale, runs := range bi.TargetRuns {
		b.SetTargetRuns(model.LocaleID(locale), runs)
	}
	// Carry the per-locale target text and review status (the edition's
	// status) into the cache so an offline reload round-trips review state.
	for locale, ti := range bi.Targets {
		loc := model.LocaleID(locale)
		if _, ok := b.TargetEdition(loc); !ok && ti.Text != "" {
			// The targets map carries committed text the runs map didn't
			// (plain-text blocks travel without targets_runs).
			b.SetTargetText(loc, ti.Text)
		}
		if t, ok := b.TargetEdition(loc); ok && ti.Status != "" {
			t.Status = model.Status(ti.Status)
			b.SetTargetEdition(model.Variant(loc), t)
		}
	}
	return b
}

// storedBlockToBlockInfo converts a StoredBlock to a BlockInfo, in the shape
// the server's blocks route serves (storedBlockToInfoResponse): targets
// carries, per locale, the committed plain text and the per-locale review
// status (the edition's status), so the shared editor reads the same shape
// online and offline. The source's and each target's runs ride along whole.
func storedBlockToBlockInfo(sb *venue.StoredBlock, targetLocales []string) BlockInfo {
	targetRuns := make(map[string][]model.Run, len(targetLocales))
	targets := make(map[string]BlockTargetInfo, len(targetLocales))
	revisions := make(map[string]string, len(targetLocales))
	for _, locale := range targetLocales {
		loc := model.LocaleID(locale)
		revisions[locale] = store.TargetRevision(sb, loc)
		if runs := sb.Block.TargetRuns(loc); len(runs) > 0 {
			targetRuns[locale] = runs
		}
		text := sb.Block.TargetText(loc)
		status := ""
		if t, ok := sb.Block.TargetEdition(loc); ok {
			status = string(t.Status)
		}
		if text != "" || status != "" {
			targets[locale] = BlockTargetInfo{Text: text, Status: status}
		}
	}

	props := make(map[string]string, len(sb.Block.Properties))
	maps.Copy(props, sb.Block.Properties)

	source := sb.Block.SourceRuns()
	return BlockInfo{
		ID:              sb.Block.ID,
		SourceID:        sb.SourceID,
		Name:            sb.Block.Name,
		Source:          sb.Block.SourceText(),
		SourceRuns:      source,
		Targets:         targets,
		TargetRuns:      targetRuns,
		Translatable:    sb.Block.Translatable,
		HasInlineCodes:  model.RunsHaveInlineCodes(source),
		Properties:      props,
		TargetRevisions: revisions,
	}
}

// legacyTranslationStatusProperty is the block-wide review flag of cached
// blocks written before review status lived on each translation
// (model.Edition.Status). It is never written; a decision that moves a block
// with no translation clears it.
const legacyTranslationStatusProperty = "translation-status"

// reviewBlockLocal applies one decision of a bulk review to the locally cached
// block, the way a decide operation applies to it (decideCached): approve
// establishes the translation, and a clearing call moves it to translated, or
// to draft for a rejection (status "draft").
func (a *App) reviewBlockLocal(projectID, blockID, targetLocale string, approve bool, status string) error {
	ctx := context.Background()
	sb, err := a.store.GetBlock(ctx, projectID, editorStream, blockID)
	if err != nil {
		return err
	}
	outcome := change.OutcomeWithdraw
	switch {
	case approve:
		outcome = change.OutcomeEstablish
	case status == string(model.TargetStatusDraft):
		outcome = change.OutcomeReject
	}
	moved, cerr := decideCached(sb.Block, model.LocaleID(targetLocale), outcome)
	if cerr != nil {
		return errors.New(cerr.Message)
	}
	if !moved {
		return nil
	}
	return a.store.StoreBlocks(ctx, projectID, editorStream, []*model.Block{sb.Block})
}

// PseudoTranslateItem pseudo-translates all blocks in an item. When connected
// the action runs on the server (source of truth) and the local cache is
// refreshed from the result; on failure or offline it runs locally against the
// cache and queues the action for replay on reconnect.
func (a *App) PseudoTranslateItem(projectID, itemName, targetLocale string) (*TranslationStats, error) {
	op := pseudoTranslateItemOp{ProjectID: projectID, ItemName: itemName, TargetLocale: targetLocale}
	return writeThroughResult(a, op,
		func() (*TranslationStats, error) {
			client, ws := a.editorRemote()
			stats, err := client.PseudoTranslateItem(context.Background(), ws, projectID, itemName, targetLocale)
			if err != nil {
				return nil, err
			}
			return editorStatsToStats(stats), nil
		},
		func(*TranslationStats) { a.refreshItemCache(projectID, itemName) },
		func() (*TranslationStats, error) {
			return a.pseudoTranslateItemLocal(projectID, itemName, targetLocale)
		},
	)
}

func (a *App) pseudoTranslateItemLocal(projectID, itemName, targetLocale string) (*TranslationStats, error) {
	ctx := context.Background()
	storedBlocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	parts := storedBlocksToParts(storedBlocks)

	// Reset() declares the defaults, and a caller building the config as a
	// literal has to apply them: since markers became switchable, an unset
	// Prefix/Suffix means "no marker" rather than "the default one", so a struct
	// literal silently produced unmarked pseudo text. The config path and the
	// wasm playground apply them for the same reason.
	//
	// The desktop wants the markers. They are what makes a pseudo string
	// identifiable in the editor; the demo-site case that motivated turning them
	// off is about a page someone is being shown, which this is not.
	pseudoCfg := &libtools.PseudoConfig{}
	pseudoCfg.Reset()
	pseudoCfg.TargetLocale = model.LocaleID(targetLocale)
	pseudoTool := libtools.NewPseudoTranslateTool(pseudoCfg)

	outParts, err := tool.RunOnParts(ctx, pseudoTool, parts)
	if err != nil {
		return nil, fmt.Errorf("pseudo-translate: %w", err)
	}

	// Store updated blocks back — they already have internal IDs from GetBlocks.
	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := a.store.StoreBlocks(ctx, projectID, "main", blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return computeStats(outParts, targetLocale), nil
}

// MemoryTranslateItem leverages content memory to translate blocks. Routing
// mirrors PseudoTranslateItem: server-first when connected, local cache with a
// queued replay when offline.
func (a *App) MemoryTranslateItem(projectID, itemName, targetLocale string) (*TranslationStats, error) {
	op := memoryTranslateItemOp{ProjectID: projectID, ItemName: itemName, TargetLocale: targetLocale}
	return writeThroughResult(a, op,
		func() (*TranslationStats, error) {
			client, ws := a.editorRemote()
			stats, err := client.MemoryTranslateItem(context.Background(), ws, projectID, itemName, targetLocale)
			if err != nil {
				return nil, err
			}
			return editorStatsToStats(stats), nil
		},
		func(*TranslationStats) { a.refreshItemCache(projectID, itemName) },
		func() (*TranslationStats, error) {
			return a.memoryTranslateItemLocal(projectID, itemName, targetLocale)
		},
	)
}

func (a *App) memoryTranslateItemLocal(projectID, itemName, targetLocale string) (*TranslationStats, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	storedBlocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	tm, err := a.getOrCreateMemory()
	if err != nil {
		return nil, fmt.Errorf("init content memory: %w", err)
	}

	parts := storedBlocksToParts(storedBlocks)

	memoryTool := leverage.NewTool(tm, proj.DefaultSourceLanguage, model.LocaleID(targetLocale), 0, nil)

	outParts, err := tool.RunOnParts(ctx, memoryTool, parts)
	if err != nil {
		return nil, fmt.Errorf("content memory translate: %w", err)
	}

	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		// Blocks already have internal IDs from GetBlocks — use StoreBlocks.
		if err := a.store.StoreBlocks(ctx, projectID, "main", blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return computeStats(outParts, targetLocale), nil
}

// GetWordCount returns word and character counts for an item.
func (a *App) GetWordCount(projectID, itemName string) (*WordCountResult, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	storedBlocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	targetLocales := make([]string, len(proj.TargetLanguages))
	for i, l := range proj.TargetLanguages {
		targetLocales[i] = string(l)
	}

	result := &WordCountResult{
		TargetWords: make(map[string]int),
		TargetChars: make(map[string]int),
	}

	for _, sb := range storedBlocks {
		if !sb.Block.Translatable {
			continue
		}
		src := sb.Block.SourceText()
		result.SourceWords += model.CountWords(src)
		result.SourceChars += countChars(src)

		for _, locale := range targetLocales {
			t := sb.Block.TargetText(model.LocaleID(locale))
			if t != "" {
				result.TargetWords[locale] += model.CountWords(t)
				result.TargetChars[locale] += countChars(t)
			}
		}
	}

	return result, nil
}

// ExportTranslatedItem is no longer supported in the desktop app.
// Source bytes are no longer stored in the Item model. Use the CLI
// ('kapi pull') for translated file export.
func (a *App) ExportTranslatedItem(_, itemName, _ string) (string, error) {
	return "", fmt.Errorf("server-side export not available for %q: use 'kapi pull' for translated file export", itemName)
}

// OpenFileInOS opens a file using the OS default application.
func (a *App) OpenFileInOS(filePath string) error {
	ctx := context.Background()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", filePath)
	case "linux":
		cmd = exec.CommandContext(ctx, "xdg-open", filePath)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", filePath)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// MemoryMatchInfo is a content-memory match result for a single block, exposed to the frontend.
type MemoryMatchInfo struct {
	Source string `json:"source"`
	Target string `json:"target"`
	// TargetRuns is the target as runs when it holds an inline code or a
	// plural, which Target leaves out: applying the match saves these. They
	// are the runs a change carries, in the shape the editor sends them.
	TargetRuns []model.Run `json:"target_runs,omitempty"`
	Score      float64     `json:"score"`
	MatchType  string      `json:"match_type"`
}

// LookupMemoryForBlock looks up content-memory matches for a specific block.
func (a *App) LookupMemoryForBlock(projectID, itemName, blockID, targetLocale string) ([]MemoryMatchInfo, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		matches, err := client.LookupMemoryForBlock(context.Background(), ws, projectID, blockID, targetLocale)
		if err != nil {
			a.goOffline()
			// Fall through to local content-memory lookup.
		} else {
			return editorMemoryMatchesToInfos(matches), nil
		}
	}
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	tm, err := a.getOrCreateMemory()
	if err != nil {
		return nil, fmt.Errorf("init content memory: %w", err)
	}
	if count, err := tm.Count(ctx); err != nil {
		return nil, err
	} else if count == 0 {
		return nil, nil
	}

	sb, err := a.store.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return nil, err
	}

	opts := memory.DefaultLookupOptions()
	opts.MaxResults = 5
	matches, err := tm.Lookup(ctx, sb.Block, proj.DefaultSourceLanguage, model.LocaleID(targetLocale), opts)
	if err != nil {
		return nil, err
	}

	srcLoc := proj.DefaultSourceLanguage
	tgtLoc := model.LocaleID(targetLocale)
	result := make([]MemoryMatchInfo, len(matches))
	for i, m := range matches {
		result[i] = MemoryMatchInfo{
			Source:     m.Entry.VariantText(srcLoc),
			Target:     m.Entry.VariantText(tgtLoc),
			TargetRuns: matchRuns(m.Entry.Variant(tgtLoc)),
			Score:      m.Score,
			MatchType:  string(m.MatchType),
		}
	}
	return result, nil
}

// matchRuns is a match's target runs when they hold an inline code or a
// plural, and nil for plain text, which Target already carries.
func matchRuns(runs []model.Run) []model.Run {
	if !model.RunsHaveInlineCodes(runs) {
		return nil
	}
	return runs
}

// BlockTermMatch is a term match for a block, exposed to the frontend.
type BlockTermMatch struct {
	SourceTerm  string   `json:"source_term"`
	TargetTerms []string `json:"target_terms"`
	Domain      string   `json:"domain"`
	Status      string   `json:"status"`
	Start       int      `json:"start"`
	End         int      `json:"end"`
}

// LookupTermsForBlock looks up term matches in a specific block's source text.
func (a *App) LookupTermsForBlock(projectID, itemName, blockID, targetLocale string) ([]BlockTermMatch, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		matches, err := client.LookupTermsForBlock(context.Background(), ws, projectID, blockID, targetLocale)
		if err != nil {
			a.goOffline()
			// Fall through to local term lookup.
		} else {
			return editorTermMatchesToBlockMatches(matches), nil
		}
	}
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	tb, err := a.getOrCreateTB()
	if err != nil {
		return nil, fmt.Errorf("init terms: %w", err)
	}
	if count, err := tb.Count(ctx); err != nil {
		return nil, err
	} else if count == 0 {
		return nil, nil
	}

	sb, err := a.store.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return nil, err
	}

	sourceText := sb.Block.SourceText()
	if sourceText == "" {
		return nil, nil
	}

	matches, err := tb.LookupAll(ctx, sourceText, terms.LookupOptions{
		SourceLocale: proj.DefaultSourceLanguage,
		TargetLocale: model.LocaleID(targetLocale),
	})
	if err != nil {
		return nil, err
	}

	var result []BlockTermMatch
	for _, m := range matches {
		// Collect target terms for the requested locale
		var targetTerms []string
		for _, t := range m.Concept.Terms {
			if t.Locale == model.LocaleID(targetLocale) {
				targetTerms = append(targetTerms, t.Text)
			}
		}

		result = append(result, BlockTermMatch{
			SourceTerm:  m.Term.Text,
			TargetTerms: targetTerms,
			Domain:      m.Concept.Domain,
			Status:      string(m.Term.Status),
			Start:       m.Position.Start,
			End:         m.Position.End,
		})
	}
	return result, nil
}

// computeStats calculates translation statistics from parts.
func computeStats(parts []*model.Part, targetLocale string) *TranslationStats {
	stats := &TranslationStats{}
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		stats.TotalBlocks++
		stats.WordCount += model.CountWords(block.SourceText())
		if block.TargetText(model.LocaleID(targetLocale)) != "" {
			stats.TranslatedBlocks++
		}
	}
	return stats
}

// storedBlocksToParts wraps stored blocks as Part objects for tool processing.
func storedBlocksToParts(storedBlocks []*venue.StoredBlock) []*model.Part {
	parts := make([]*model.Part, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		parts = append(parts, &model.Part{
			Type:     model.PartBlock,
			Resource: sb.Block,
		})
	}
	return parts
}

// partsToBlocks extracts model.Block objects from a Part slice.
func partsToBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		if block, ok := pt.Resource.(*model.Block); ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}
