package backend

import (
	"context"
	"fmt"
	"maps"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/editorclient"

	"github.com/neokapi/neokapi/bowrain/core/store"
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
			view := PendingReviewEntryView{BlockID: e.BlockID, ItemName: e.ItemName, Locale: e.Locale}
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
// block from its own request — the demotion an edit triggers is the store's
// decision on both paths (applyTargetTextEdit locally, the server's
// demoteStaleReviewOnEdit remotely), and a second copy of that rule in
// TypeScript is a copy that can disagree.
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
	NotStarted int `json:"not-started"`
	Draft      int `json:"draft"`
	Translated int `json:"translated"`
	Reviewed   int `json:"reviewed"`
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
					NotStarted: counts.Status.NotStarted,
					Draft:      counts.Status.Draft,
					Translated: counts.Status.Translated,
					Reviewed:   counts.Status.Reviewed,
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
			NotStarted: counts.NotStarted,
			Draft:      counts.Draft,
			Translated: counts.Translated,
			Reviewed:   counts.Reviewed,
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
		status := string(model.TargetStatusReviewed)
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
func blockInfoToBlock(bi BlockInfo) *model.Block {
	b := &model.Block{
		ID:           bi.ID,
		Translatable: bi.Translatable,
		Properties:   bi.Properties,
		Targets:      make(map[model.VariantKey]*model.Target),
	}
	b.SetSourceRuns(runInfosToRuns(bi.SourceRuns))
	for locale, runs := range bi.TargetRuns {
		b.SetTargetRuns(model.LocaleID(locale), runInfosToRuns(runs))
	}
	// Carry the per-locale target text and review status (model.Target.Status)
	// into the cache so an offline reload round-trips review state.
	for locale, ti := range bi.Targets {
		loc := model.LocaleID(locale)
		if b.Target(loc) == nil && ti.Text != "" {
			// The targets map carries committed text the runs map didn't
			// (plain-text blocks travel without targets_runs).
			b.SetTargetText(loc, ti.Text)
		}
		if t := b.Target(loc); t != nil && ti.Status != "" {
			t.Status = model.TargetStatus(ti.Status)
		}
	}
	return b
}

// storedBlockToBlockInfo converts a StoredBlock to a BlockInfo.
func storedBlockToBlockInfo(sb *venue.StoredBlock, targetLocales []string) BlockInfo {
	targetRuns := make(map[string][]RunInfo, len(targetLocales))
	// Mirror the server's storedBlockToInfoResponse: targets carries, per
	// locale, the committed plain text plus the per-locale review status
	// (model.Target.Status), so the shared editor reads the same shape online
	// and offline.
	targets := make(map[string]BlockTargetInfo, len(targetLocales))
	for _, locale := range targetLocales {
		loc := model.LocaleID(locale)
		if runs := sb.Block.TargetRuns(loc); len(runs) > 0 {
			targetRuns[locale] = runsToRunInfos(runs)
		}
		text := sb.Block.TargetText(loc)
		status := ""
		if t := sb.Block.Target(loc); t != nil {
			status = string(t.Status)
		}
		if text != "" || status != "" {
			targets[locale] = BlockTargetInfo{Text: text, Status: status}
		}
	}
	if len(targets) == 0 {
		targets = nil
	}

	props := make(map[string]string, len(sb.Block.Properties))
	maps.Copy(props, sb.Block.Properties)

	return BlockInfo{
		ID:           sb.Block.ID,
		SourceRuns:   runsToRunInfos(sb.Block.SourceRuns()),
		Targets:      targets,
		TargetRuns:   targetRuns,
		Translatable: sb.Block.Translatable,
		Properties:   props,
	}
}

// runsToRunInfos converts a run sequence to the frontend-facing
// RunInfo representation.
func runsToRunInfos(runs []model.Run) []RunInfo {
	if len(runs) == 0 {
		return nil
	}
	out := make([]RunInfo, len(runs))
	for i, r := range runs {
		out[i] = runToRunInfo(r)
	}
	return out
}

// runInfosToRuns reverses runsToRunInfos.
func runInfosToRuns(infos []RunInfo) []model.Run {
	if len(infos) == 0 {
		return nil
	}
	out := make([]model.Run, len(infos))
	for i, ri := range infos {
		out[i] = runInfoToRun(ri)
	}
	return out
}

func runToRunInfo(r model.Run) RunInfo {
	switch {
	case r.Text != nil:
		return RunInfo{Text: &TextRunInfo{Text: r.Text.Text}}
	case r.Ph != nil:
		return RunInfo{Ph: &PlaceholderRunInfo{
			ID: r.Ph.ID, Type: r.Ph.Type, SubType: r.Ph.SubType,
			Data: r.Ph.Data, Equiv: r.Ph.Equiv, Disp: r.Ph.Disp,
			Constraints: runConstraintsToInfo(r.Ph.Constraints),
		}}
	case r.PcOpen != nil:
		return RunInfo{PcOpen: &PcOpenRunInfo{
			ID: r.PcOpen.ID, Type: r.PcOpen.Type, SubType: r.PcOpen.SubType,
			Data: r.PcOpen.Data, Equiv: r.PcOpen.Equiv, Disp: r.PcOpen.Disp,
			Constraints: runConstraintsToInfo(r.PcOpen.Constraints),
		}}
	case r.PcClose != nil:
		return RunInfo{PcClose: &PcCloseRunInfo{
			ID: r.PcClose.ID, Type: r.PcClose.Type, SubType: r.PcClose.SubType,
			Data: r.PcClose.Data, Equiv: r.PcClose.Equiv,
		}}
	case r.Sub != nil:
		return RunInfo{Sub: &SubRunInfo{ID: r.Sub.ID, Ref: r.Sub.Ref, Equiv: r.Sub.Equiv}}
	case r.Plural != nil:
		forms := make(map[string][]RunInfo, len(r.Plural.Forms))
		for form, runs := range r.Plural.Forms {
			forms[string(form)] = runsToRunInfos(runs)
		}
		return RunInfo{Plural: &PluralRunInfo{Pivot: r.Plural.Pivot, Forms: forms}}
	case r.Select != nil:
		cases := make(map[string][]RunInfo, len(r.Select.Cases))
		for key, runs := range r.Select.Cases {
			cases[key] = runsToRunInfos(runs)
		}
		return RunInfo{Select: &SelectRunInfo{Pivot: r.Select.Pivot, Cases: cases}}
	}
	return RunInfo{}
}

func runInfoToRun(ri RunInfo) model.Run {
	switch {
	case ri.Text != nil:
		return model.Run{Text: &model.TextRun{Text: ri.Text.Text}}
	case ri.Ph != nil:
		return model.Run{Ph: &model.PlaceholderRun{
			ID: ri.Ph.ID, Type: ri.Ph.Type, SubType: ri.Ph.SubType,
			Data: ri.Ph.Data, Equiv: ri.Ph.Equiv, Disp: ri.Ph.Disp,
			Constraints: runConstraintsFromInfo(ri.Ph.Constraints),
		}}
	case ri.PcOpen != nil:
		return model.Run{PcOpen: &model.PcOpenRun{
			ID: ri.PcOpen.ID, Type: ri.PcOpen.Type, SubType: ri.PcOpen.SubType,
			Data: ri.PcOpen.Data, Equiv: ri.PcOpen.Equiv, Disp: ri.PcOpen.Disp,
			Constraints: runConstraintsFromInfo(ri.PcOpen.Constraints),
		}}
	case ri.PcClose != nil:
		return model.Run{PcClose: &model.PcCloseRun{
			ID: ri.PcClose.ID, Type: ri.PcClose.Type, SubType: ri.PcClose.SubType,
			Data: ri.PcClose.Data, Equiv: ri.PcClose.Equiv,
		}}
	case ri.Sub != nil:
		return model.Run{Sub: &model.SubRun{ID: ri.Sub.ID, Ref: ri.Sub.Ref, Equiv: ri.Sub.Equiv}}
	case ri.Plural != nil:
		forms := make(map[model.PluralForm][]model.Run, len(ri.Plural.Forms))
		for form, runs := range ri.Plural.Forms {
			forms[model.PluralForm(form)] = runInfosToRuns(runs)
		}
		return model.Run{Plural: &model.PluralRun{Pivot: ri.Plural.Pivot, Forms: forms}}
	case ri.Select != nil:
		cases := make(map[string][]model.Run, len(ri.Select.Cases))
		for key, runs := range ri.Select.Cases {
			cases[key] = runInfosToRuns(runs)
		}
		return model.Run{Select: &model.SelectRun{Pivot: ri.Select.Pivot, Cases: cases}}
	}
	return model.Run{}
}

func runConstraintsToInfo(c *model.RunConstraints) *RunConstraintsInfo {
	if c == nil {
		return nil
	}
	return &RunConstraintsInfo{Deletable: c.Deletable, Cloneable: c.Cloneable, Reorderable: c.Reorderable}
}

func runConstraintsFromInfo(ri *RunConstraintsInfo) *model.RunConstraints {
	if ri == nil {
		return nil
	}
	return &model.RunConstraints{Deletable: ri.Deletable, Cloneable: ri.Cloneable, Reorderable: ri.Reorderable}
}

// UpdateBlockTarget updates the target text for a specific block. The local
// cache is authoritative for block edits, so a successful server write is still
// reconciled into the cache; an offline or failed write queues the op and
// updates the cache alone.
func (a *App) UpdateBlockTarget(req UpdateBlockRequest) error {
	local := func() error {
		return a.updateBlockTargetLocal(req.ProjectID, req.BlockID, req.TargetLocale, req.Text)
	}
	return a.writeThroughVoid(updateBlockTargetOp{req},
		func() error {
			client, ws := a.editorRemote()
			// Wrap the plain-text update in a single TextRun so the server
			// sees the canonical Run sequence.
			runs := []model.Run{{Text: &model.TextRun{Text: req.Text}}}
			return client.UpdateBlockTargetRuns(context.Background(), ws, req.ProjectID, req.BlockID, req.TargetLocale, runs)
		},
		nil, // reconcile the local cache on success
		local,
	)
}

func (a *App) updateBlockTargetLocal(projectID, blockID, targetLocale, text string) error {
	ctx := context.Background()
	sb, err := a.store.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return err
	}
	loc := model.LocaleID(targetLocale)
	oldRuns := sb.Block.TargetRuns(loc)
	sb.Block.SetTargetText(loc, text)
	stampHumanEditOrigin(sb.Block, loc)
	demoteStaleReviewOnEdit(sb.Block, loc, oldRuns)
	// Use StoreBlocks (not StoreBlocksForItem) because the block already carries
	// an internal ID — it should not be re-mapped through source_id assignment.
	return a.store.StoreBlocks(ctx, projectID, "main", []*model.Block{sb.Block})
}

// stampHumanEditOrigin records a translation a person typed as produced by a
// person, the same stamp the server writes for an edit (server/editor.go). The
// offline cache applies it so a block edited without a connection reads the way
// the same edit reads online.
func stampHumanEditOrigin(b *model.Block, loc model.LocaleID) {
	if t := b.Target(loc); t != nil {
		t.Origin = model.Origin{Kind: model.OriginHuman, Timestamp: time.Now().UTC().Format(time.RFC3339)}
	}
}

// demoteStaleReviewOnEdit drops a reviewed/signed-off Target.Status back to
// translated when an edit changed the target's runs. A review decision judges
// ONE specific translation, so rewriting it invalidates the approval; the
// offline cache applies the same rule as the server (server.editor.go), so a
// reconnect does not resurrect an approval the edit already invalidated.
// Statuses at or below translated are left alone — they are not decisions.
func demoteStaleReviewOnEdit(b *model.Block, locale model.LocaleID, oldRuns []model.Run) {
	t := b.Target(locale)
	if t == nil {
		return
	}
	if t.Status != model.TargetStatusReviewed && t.Status != model.TargetStatusSignedOff {
		return
	}
	if reflect.DeepEqual(oldRuns, t.Runs) {
		return
	}
	t.Status = model.TargetStatusTranslated
}

// UpdateBlockTargetRuns updates the target for a block using a
// structured Run sequence.
func (a *App) UpdateBlockTargetRuns(req UpdateBlockTargetRunsRequest) error {
	return a.writeThroughVoid(updateBlockTargetRunsOp{req},
		func() error {
			client, ws := a.editorRemote()
			return client.UpdateBlockTargetRuns(context.Background(), ws, req.ProjectID, req.BlockID, req.TargetLocale, runInfosToRuns(req.Runs))
		},
		nil, // reconcile the local cache on success
		func() error { return a.updateBlockTargetRunsLocal(req) },
	)
}

func (a *App) updateBlockTargetRunsLocal(req UpdateBlockTargetRunsRequest) error {
	ctx := context.Background()
	sb, err := a.store.GetBlock(ctx, req.ProjectID, "main", req.BlockID)
	if err != nil {
		return err
	}
	loc := model.LocaleID(req.TargetLocale)
	oldRuns := sb.Block.TargetRuns(loc)
	sb.Block.SetTargetRuns(loc, runInfosToRuns(req.Runs))
	stampHumanEditOrigin(sb.Block, loc)
	demoteStaleReviewOnEdit(sb.Block, loc, oldRuns)
	return a.store.StoreBlocks(ctx, req.ProjectID, "main", []*model.Block{sb.Block})
}

// ReviewBlock marks a block as reviewed, signed off or un-reviewed for a
// target locale. status picks the rung: with reviewed=true it is "" for an
// approval or "signed-off" for a sign-off; with reviewed=false it is "" or
// "translated" for a plain un-review, "draft" for a reviewer rejection (the
// unit re-enters the work queue).
func (a *App) ReviewBlock(projectID, itemName, blockID, targetLocale string, reviewed bool, status string) error {
	op := reviewBlockOp{
		ProjectID: projectID, ItemName: itemName, BlockID: blockID,
		TargetLocale: targetLocale, Reviewed: reviewed, Status: status,
	}
	return a.writeThroughVoid(op,
		func() error {
			client, ws := a.editorRemote()
			return client.ReviewBlock(context.Background(), ws, projectID, itemName, blockID, targetLocale, reviewed, status)
		},
		nil, // reconcile the local cache on success
		func() error { return a.reviewBlockLocal(projectID, blockID, targetLocale, reviewed, status) },
	)
}

// legacyTranslationStatusProperty is the pre-per-locale review flag: a
// block-GLOBAL property the old scheme wrote. Review state now lives on the
// per-locale model.Target.Status (mirroring the server's HandleReviewBlock);
// the property is write-never, kept only as a read fallback for cached blocks
// written before the change, and cleared on un-review when there is no target
// to demote.
const legacyTranslationStatusProperty = "translation-status"

// reviewBlockLocal applies a review decision to the locally cached block with
// the same per-locale semantics as the server's HandleReviewBlock: the status
// lives on the block's target for ONE locale (reviewing fr never touches de).
// Approving a block with no non-empty translation for the locale is an error
// (the server's 422); un-reviewing a locale with no target clears the legacy
// block-global property if present and is otherwise a no-op. status picks the
// rung the same way the server's optional status field does: "signed-off" on
// an approval, "draft" on a clearing call for a rejection, otherwise the
// default rung for the direction. The queued op carries it, so a working copy
// that signs off while offline shows the rung it queued.
func (a *App) reviewBlockLocal(projectID, blockID, targetLocale string, reviewed bool, status string) error {
	ctx := context.Background()
	sb, err := a.store.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return err
	}

	loc := model.LocaleID(targetLocale)
	target := sb.Block.Target(loc)

	if reviewed {
		if target == nil || strings.TrimSpace(sb.Block.TargetText(loc)) == "" {
			return fmt.Errorf("block %q has no %s translation to review: translate it first", blockID, targetLocale)
		}
		if target.Status == model.TargetStatusSignedOff {
			// Signed-off is the top of the ladder; approving or re-signing it
			// must not demote it (mirrors the server's HandleReviewBlock no-op).
			return nil
		}
		if status == string(model.TargetStatusSignedOff) {
			target.Status = model.TargetStatusSignedOff
		} else {
			target.Status = model.TargetStatusReviewed
		}
	} else {
		if target == nil {
			// Nothing to demote. Clear the legacy block-global flag if present so
			// a block reviewed under the old scheme can be un-reviewed at all.
			if _, ok := sb.Block.Properties[legacyTranslationStatusProperty]; ok {
				delete(sb.Block.Properties, legacyTranslationStatusProperty)
				return a.store.StoreBlocks(ctx, projectID, "main", []*model.Block{sb.Block})
			}
			return nil
		}
		if status == string(model.TargetStatusDraft) {
			target.Status = model.TargetStatusDraft
		} else {
			target.Status = model.TargetStatusTranslated
		}
	}
	return a.store.StoreBlocks(ctx, projectID, "main", []*model.Block{sb.Block})
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

	memoryTool := leverage.NewTool(tm, proj.DefaultSourceLanguage, model.LocaleID(targetLocale), 0)

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
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"`
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
			Source:    m.Entry.VariantText(srcLoc),
			Target:    m.Entry.VariantText(tgtLoc),
			Score:     m.Score,
			MatchType: string(m.MatchType),
		}
	}
	return result, nil
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
