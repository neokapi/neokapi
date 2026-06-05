package backend

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
)

// GetItemBlocks returns all blocks for an item in the project.
// When connected, blocks are fetched from the server and cached locally.
// On connection failure, falls back to the local cache.
func (a *App) GetItemBlocks(projectID, itemName string) ([]BlockInfo, error) {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		blocks, err := a.remote.GetBlocks(ws, projectID, itemName)
		if err != nil {
			// Connection failed — fall back to offline mode.
			a.goOffline()
			return a.getItemBlocksLocal(projectID, itemName)
		}
		// Cache the blocks locally for offline access.
		a.cacheBlocks(projectID, itemName, blocks)
		return blocks, nil
	}
	return a.getItemBlocksLocal(projectID, itemName)
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
	return b
}

// storedBlockToBlockInfo converts a StoredBlock to a BlockInfo.
func storedBlockToBlockInfo(sb *store.StoredBlock, targetLocales []string) BlockInfo {
	targetRuns := make(map[string][]RunInfo, len(targetLocales))
	for _, locale := range targetLocales {
		runs := sb.Block.TargetRuns(model.LocaleID(locale))
		if len(runs) == 0 {
			continue
		}
		targetRuns[locale] = runsToRunInfos(runs)
	}

	props := make(map[string]string, len(sb.Block.Properties))
	for k, v := range sb.Block.Properties {
		props[k] = v
	}

	return BlockInfo{
		ID:           sb.Block.ID,
		SourceRuns:   runsToRunInfos(sb.Block.SourceRuns()),
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

// UpdateBlockTarget updates the target text for a specific block.
// When connected, sends to server. On failure, queues for later replay and updates local cache.
func (a *App) UpdateBlockTarget(req UpdateBlockRequest) error {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		// Wrap the plain-text update in a single TextRun so the
		// server sees the canonical Run sequence.
		runs := []RunInfo{{Text: &TextRunInfo{Text: req.Text}}}
		err := a.remote.UpdateBlockTarget(ws, req.ProjectID, req.BlockID, req.TargetLocale, runs)
		if err != nil {
			a.goOffline()
			a.enqueue("update_block_target", req)
			// Fall through to update local cache.
		} else {
			// Update local cache on success.
			a.updateBlockTargetLocal(req.ProjectID, req.BlockID, req.TargetLocale, req.Text)
			return nil
		}
	} else if a.isOffline() {
		a.enqueue("update_block_target", req)
	}

	// Update local store (both offline and local modes).
	return a.updateBlockTargetLocal(req.ProjectID, req.BlockID, req.TargetLocale, req.Text)
}

func (a *App) updateBlockTargetLocal(projectID, blockID, targetLocale, text string) error {
	ctx := context.Background()
	sb, err := a.store.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return err
	}
	sb.Block.SetTargetText(model.LocaleID(targetLocale), text)
	// Use StoreBlocks (not StoreBlocksForItem) because the block already carries
	// an internal ID — it should not be re-mapped through source_id assignment.
	return a.store.StoreBlocks(ctx, projectID, "main", []*model.Block{sb.Block})
}

// UpdateBlockTargetRuns updates the target for a block using a
// structured Run sequence.
func (a *App) UpdateBlockTargetRuns(req UpdateBlockTargetRunsRequest) error {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		err := a.remote.UpdateBlockTarget(ws, req.ProjectID, req.BlockID, req.TargetLocale, req.Runs)
		if err != nil {
			a.goOffline()
			a.enqueue("update_block_target_runs", req)
		} else {
			a.updateBlockTargetRunsLocal(req)
			return nil
		}
	} else if a.isOffline() {
		a.enqueue("update_block_target_runs", req)
	}

	return a.updateBlockTargetRunsLocal(req)
}

func (a *App) updateBlockTargetRunsLocal(req UpdateBlockTargetRunsRequest) error {
	ctx := context.Background()
	sb, err := a.store.GetBlock(ctx, req.ProjectID, "main", req.BlockID)
	if err != nil {
		return err
	}
	sb.Block.SetTargetRuns(model.LocaleID(req.TargetLocale), runInfosToRuns(req.Runs))
	return a.store.StoreBlocks(ctx, req.ProjectID, "main", []*model.Block{sb.Block})
}

// ReviewBlock marks a block as reviewed or un-reviewed for a target locale.
func (a *App) ReviewBlock(projectID, itemName, blockID, targetLocale string, reviewed bool) error {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		err := a.remote.ReviewBlock(ws, projectID, itemName, blockID, targetLocale, reviewed)
		if err != nil {
			a.goOffline()
			a.enqueue("review_block", reviewBlockPayload{
				ProjectID: projectID, ItemName: itemName, BlockID: blockID,
				TargetLocale: targetLocale, Reviewed: reviewed,
			})
		} else {
			a.reviewBlockLocal(projectID, blockID, reviewed)
			return nil
		}
	} else if a.isOffline() {
		a.enqueue("review_block", reviewBlockPayload{
			ProjectID: projectID, ItemName: itemName, BlockID: blockID,
			TargetLocale: targetLocale, Reviewed: reviewed,
		})
	}

	return a.reviewBlockLocal(projectID, blockID, reviewed)
}

func (a *App) reviewBlockLocal(projectID, blockID string, reviewed bool) error {
	ctx := context.Background()
	sb, err := a.store.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return err
	}
	if sb.Block.Properties == nil {
		sb.Block.Properties = make(map[string]string)
	}
	if reviewed {
		sb.Block.Properties["translation-status"] = "reviewed"
	} else {
		sb.Block.Properties["translation-status"] = "translated"
	}
	return a.store.StoreBlocks(ctx, projectID, "main", []*model.Block{sb.Block})
}

// PseudoTranslateItem pseudo-translates all blocks in an item.
func (a *App) PseudoTranslateItem(projectID, itemName, targetLocale string) (*TranslationStats, error) {
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

	pseudoTool := &tool.BaseTool{
		ToolName:        "pseudo-translate",
		ToolDescription: "Pseudo-translates blocks",
	}
	pseudoTool.Translate = func(v tool.TargetView) error {
		if !v.Translatable() {
			return nil
		}
		locale := model.LocaleID(targetLocale)
		runs := v.SourceRuns()
		if runsHaveInlineCodes(runs) {
			v.SetTargetRuns(locale, pseudoRuns(runs))
		} else {
			v.SetTargetText(locale, "["+pseudoAccent(v.SourceText())+"]")
		}
		return nil
	}

	outParts, err := runToolOnParts(ctx, pseudoTool, parts)
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

// TMTranslateItem leverages translation memory to translate blocks.
func (a *App) TMTranslateItem(projectID, itemName, targetLocale string) (*TranslationStats, error) {
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

	tm, err := a.getOrCreateTM()
	if err != nil {
		return nil, fmt.Errorf("init TM: %w", err)
	}

	parts := storedBlocksToParts(storedBlocks)

	tmTool := sievepen.NewTMLeverageTool(tm, sievepen.TMLeverageConfig{
		MinScore:     0.7,
		MaxResults:   5,
		SourceLocale: proj.DefaultSourceLanguage,
		TargetLocale: model.LocaleID(targetLocale),
	})

	outParts, err := runToolOnParts(ctx, tmTool, parts)
	if err != nil {
		return nil, fmt.Errorf("TM translate: %w", err)
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
		result.SourceWords += countWords(src)
		result.SourceChars += countChars(src)

		for _, locale := range targetLocales {
			t := sb.Block.TargetText(model.LocaleID(locale))
			if t != "" {
				result.TargetWords[locale] += countWords(t)
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
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "linux":
		cmd = exec.Command("xdg-open", filePath)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", filePath)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// TMMatchInfo is a TM match result for a single block, exposed to the frontend.
type TMMatchInfo struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"`
}

// LookupTMForBlock looks up TM matches for a specific block.
func (a *App) LookupTMForBlock(projectID, itemName, blockID, targetLocale string) ([]TMMatchInfo, error) {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		matches, err := a.remote.LookupTMForBlock(ws, projectID, blockID, targetLocale)
		if err != nil {
			a.goOffline()
			// Fall through to local TM lookup.
		} else {
			return matches, nil
		}
	}
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	tm, err := a.getOrCreateTM()
	if err != nil {
		return nil, fmt.Errorf("init TM: %w", err)
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

	opts := sievepen.DefaultLookupOptions()
	opts.MaxResults = 5
	matches, err := tm.Lookup(ctx, sb.Block, proj.DefaultSourceLanguage, model.LocaleID(targetLocale), opts)
	if err != nil {
		return nil, err
	}

	srcLoc := proj.DefaultSourceLanguage
	tgtLoc := model.LocaleID(targetLocale)
	result := make([]TMMatchInfo, len(matches))
	for i, m := range matches {
		result[i] = TMMatchInfo{
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
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		matches, err := a.remote.LookupTermsForBlock(ws, projectID, blockID, targetLocale)
		if err != nil {
			a.goOffline()
			// Fall through to local term lookup.
		} else {
			return matches, nil
		}
	}
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	tb, err := a.getOrCreateTB()
	if err != nil {
		return nil, fmt.Errorf("init termbase: %w", err)
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

	matches, err := tb.LookupAll(ctx, sourceText, termbase.LookupOptions{
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
		stats.WordCount += countWords(block.SourceText())
		if block.TargetText(model.LocaleID(targetLocale)) != "" {
			stats.TranslatedBlocks++
		}
	}
	return stats
}

// storedBlocksToParts wraps stored blocks as Part objects for tool processing.
func storedBlocksToParts(storedBlocks []*store.StoredBlock) []*model.Part {
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

// runToolOnParts executes a tool on parts using channels.
//
// Process runs in its own goroutine while the caller drains the output channel
// concurrently, so a fan-out tool that emits more parts than it consumes cannot
// deadlock on a bounded buffer.
func runToolOnParts(ctx context.Context, t tool.Tool, parts []*model.Part) ([]*model.Part, error) {
	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, pt := range parts {
		in <- pt
	}
	close(in)

	errCh := make(chan error, 1)
	go func() {
		err := t.Process(ctx, in, out)
		close(out)
		errCh <- err
	}()

	var result []*model.Part
	for pt := range out {
		result = append(result, pt)
	}
	if err := <-errCh; err != nil {
		return nil, err
	}
	return result, nil
}

// pseudoAccent applies accent characters to ASCII text for pseudo-translation.
func pseudoAccent(text string) string {
	var buf bytes.Buffer
	for _, r := range text {
		if replacement, ok := accentMap[r]; ok {
			buf.WriteRune(replacement)
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// runsHaveInlineCodes reports whether a Run sequence carries any non-text run
// (placeholder or paired code) — the signal that pseudo-translation must walk
// runs in place rather than flattening to text.
func runsHaveInlineCodes(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// pseudoRuns walks a Run sequence and applies pseudo-accent to TextRun
// content only, leaving inline-code runs untouched. The result is
// bracketed with `[` / `]` as plain TextRuns at the boundaries.
func pseudoRuns(runs []model.Run) []model.Run {
	out := make([]model.Run, 0, len(runs)+2)
	out = append(out, model.Run{Text: &model.TextRun{Text: "["}})
	for _, r := range runs {
		if r.Text != nil {
			out = append(out, model.Run{Text: &model.TextRun{Text: pseudoAccent(r.Text.Text)}})
			continue
		}
		out = append(out, r)
	}
	out = append(out, model.Run{Text: &model.TextRun{Text: "]"}})
	return out
}

var accentMap = map[rune]rune{
	'a': '\u00e0', 'b': '\u0180', 'c': '\u00e7', 'd': '\u00f0',
	'e': '\u00e9', 'f': '\u0192', 'g': '\u011d', 'h': '\u0125',
	'i': '\u00ee', 'j': '\u0135', 'k': '\u0137', 'l': '\u013c',
	'm': '\u1e3f', 'n': '\u00f1', 'o': '\u00f6', 'p': '\u00fe',
	'q': '\u01eb', 'r': '\u0155', 's': '\u0161', 't': '\u0163',
	'u': '\u00fb', 'v': '\u1e7d', 'w': '\u0175', 'x': '\u1e8b',
	'y': '\u00fd', 'z': '\u017e',
	'A': '\u00c0', 'B': '\u0181', 'C': '\u00c7', 'D': '\u00d0',
	'E': '\u00c9', 'F': '\u0191', 'G': '\u011c', 'H': '\u0124',
	'I': '\u00ce', 'J': '\u0134', 'K': '\u0136', 'L': '\u013b',
	'M': '\u1e3e', 'N': '\u00d1', 'O': '\u00d6', 'P': '\u00de',
	'Q': '\u01ea', 'R': '\u0154', 'S': '\u0160', 'T': '\u0162',
	'U': '\u00db', 'V': '\u1e7c', 'W': '\u0174', 'X': '\u1e8a',
	'Y': '\u00dd', 'Z': '\u017d',
}

// fileDir returns the directory of a file path.
func fileDir(path string) string {
	dir := path
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' || dir[i] == '\\' {
			return dir[:i]
		}
	}
	return "."
}

// copyTargets creates a shallow copy of a string map.
func copyTargets(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// copyStringMap creates a shallow copy of a string map.
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
