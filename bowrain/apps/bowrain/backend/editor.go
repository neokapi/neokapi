package backend

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gokapi/gokapi/bowrain/credentials"
	"github.com/gokapi/gokapi/core/ai/tools"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/sievepen"
	"github.com/gokapi/gokapi/core/termbase"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/gokapi/gokapi/platform/store"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
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

	targetLocales := make([]string, len(proj.TargetLocales))
	for i, l := range proj.TargetLocales {
		targetLocales[i] = string(l)
	}

	storedBlocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
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
		_ = a.store.StoreBlocksForItem(ctx, projectID, itemName, modelBlocks)
	}
}

// blockInfoToBlock converts a BlockInfo (from server) to a model.Block for local storage.
func blockInfoToBlock(bi BlockInfo) *model.Block {
	b := &model.Block{
		ID:           bi.ID,
		Translatable: bi.Translatable,
		Properties:   bi.Properties,
	}

	// Set source.
	if bi.HasSpans && bi.SourceCoded != "" {
		frag := &model.Fragment{CodedText: bi.SourceCoded}
		for _, si := range bi.SourceSpans {
			frag.Spans = append(frag.Spans, infoToSpan(si))
		}
		b.Source = []*model.Segment{{Content: frag}}
	} else if bi.Source != "" {
		b.Source = []*model.Segment{{Content: model.NewFragment(bi.Source)}}
	}

	// Set targets.
	if len(bi.Targets) > 0 {
		b.Targets = make(map[model.LocaleID][]*model.Segment)
		for locale, text := range bi.Targets {
			lid := model.LocaleID(locale)
			if bi.TargetsCoded != nil {
				if coded, ok := bi.TargetsCoded[locale]; ok && coded != "" {
					b.Targets[lid] = []*model.Segment{{Content: &model.Fragment{CodedText: coded}}}
					continue
				}
			}
			b.Targets[lid] = []*model.Segment{{Content: model.NewFragment(text)}}
		}
	}

	return b
}

// storedBlockToBlockInfo converts a StoredBlock to a BlockInfo.
func storedBlockToBlockInfo(sb *store.StoredBlock, targetLocales []string) BlockInfo {
	targets := make(map[string]string)
	for _, locale := range targetLocales {
		if t := sb.Block.TargetText(model.LocaleID(locale)); t != "" {
			targets[locale] = t
		}
	}

	props := make(map[string]string)
	for k, v := range sb.Block.Properties {
		props[k] = v
	}

	bi := BlockInfo{
		ID:           sb.Block.ID,
		Source:       sb.Block.SourceText(),
		Targets:      targets,
		Translatable: sb.Block.Translatable,
		Properties:   props,
	}

	enrichBlockInfo(&bi, sb.Block, targetLocales)
	return bi
}

// enrichBlockInfo populates coded text and span metadata on a BlockInfo
// from the underlying model.Block, if spans are present.
func enrichBlockInfo(bi *BlockInfo, block *model.Block, targetLocales []string) {
	if len(block.Source) == 0 || block.Source[0].Content == nil {
		return
	}
	frag := block.Source[0].Content
	if !frag.HasSpans() {
		return
	}

	bi.HasSpans = true
	bi.SourceCoded = frag.CodedText
	bi.SourceSpans = make([]SpanInfo, len(frag.Spans))
	for i, s := range frag.Spans {
		bi.SourceSpans[i] = spanToInfo(s)
	}

	// Extract target coded text per locale
	bi.TargetsCoded = make(map[string]string)
	for _, locale := range targetLocales {
		segs, ok := block.Targets[model.LocaleID(locale)]
		if !ok || len(segs) == 0 {
			continue
		}
		if segs[0].Content != nil {
			bi.TargetsCoded[locale] = segs[0].Content.CodedText
		}
	}
}

// spanToInfo converts a model.Span to a SpanInfo for frontend serialization.
func spanToInfo(s *model.Span) SpanInfo {
	var spanType string
	switch s.SpanType {
	case model.SpanOpening:
		spanType = "opening"
	case model.SpanClosing:
		spanType = "closing"
	case model.SpanPlaceholder:
		spanType = "placeholder"
	}
	return SpanInfo{
		SpanType: spanType,
		Type:     s.Type,
		ID:       s.ID,
		Data:     s.Data,
	}
}

// UpdateBlockTarget updates the target text for a specific block.
// When connected, sends to server. On failure, queues for later replay and updates local cache.
func (a *App) UpdateBlockTarget(req UpdateBlockRequest) error {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		err := a.remote.UpdateBlockTarget(ws, req.ProjectID, req.BlockID, req.TargetLocale, req.Text, "", nil)
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
	sb, err := a.store.GetBlock(ctx, projectID, blockID)
	if err != nil {
		return err
	}
	sb.Block.SetTargetText(model.LocaleID(targetLocale), text)
	return a.store.StoreBlocksForItem(ctx, projectID, sb.ItemName, []*model.Block{sb.Block})
}

// UpdateBlockTargetCoded updates the target for a block using coded text with span data.
func (a *App) UpdateBlockTargetCoded(req UpdateBlockTargetCodedRequest) error {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		spans := make([]SpanInfo, len(req.Spans))
		copy(spans, req.Spans)
		err := a.remote.UpdateBlockTarget(ws, req.ProjectID, req.BlockID, req.TargetLocale, "", req.CodedText, spans)
		if err != nil {
			a.goOffline()
			a.enqueue("update_block_target_coded", req)
		} else {
			a.updateBlockTargetCodedLocal(req)
			return nil
		}
	} else if a.isOffline() {
		a.enqueue("update_block_target_coded", req)
	}

	return a.updateBlockTargetCodedLocal(req)
}

func (a *App) updateBlockTargetCodedLocal(req UpdateBlockTargetCodedRequest) error {
	ctx := context.Background()
	sb, err := a.store.GetBlock(ctx, req.ProjectID, req.BlockID)
	if err != nil {
		return err
	}

	frag := &model.Fragment{
		CodedText: req.CodedText,
	}
	for _, si := range req.Spans {
		frag.Spans = append(frag.Spans, infoToSpan(si))
	}
	sb.Block.SetTargetFragment(model.LocaleID(req.TargetLocale), frag)

	return a.store.StoreBlocksForItem(ctx, req.ProjectID, sb.ItemName, []*model.Block{sb.Block})
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
	sb, err := a.store.GetBlock(ctx, projectID, blockID)
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
	return a.store.StoreBlocksForItem(ctx, projectID, sb.ItemName, []*model.Block{sb.Block})
}

// stripMarkers removes Unicode span markers from coded text, returning plain text.
func stripMarkers(coded string) string {
	var buf []byte
	for _, r := range coded {
		if r < '\uE001' || r > '\uE003' {
			buf = append(buf, []byte(string(r))...)
		}
	}
	return string(buf)
}

// infoToSpan converts a SpanInfo from the frontend back to a model.Span.
func infoToSpan(si SpanInfo) *model.Span {
	var st model.SpanType
	switch si.SpanType {
	case "opening":
		st = model.SpanOpening
	case "closing":
		st = model.SpanClosing
	case "placeholder":
		st = model.SpanPlaceholder
	}
	return &model.Span{
		SpanType: st,
		Type:     si.Type,
		ID:       si.ID,
		Data:     si.Data,
	}
}

// PseudoTranslateItem pseudo-translates all blocks in an item.
func (a *App) PseudoTranslateItem(projectID, itemName, targetLocale string) (*TranslationStats, error) {
	ctx := context.Background()
	storedBlocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
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
	pseudoTool.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok || !block.Translatable {
			return part, nil
		}
		locale := model.LocaleID(targetLocale)
		frag := block.FirstFragment()
		if frag != nil && frag.HasSpans() {
			// Pseudo-translate coded text, preserving span markers
			pseudoCoded := "[" + pseudoAccent(frag.CodedText) + "]"
			targetFrag := frag.Clone()
			targetFrag.CodedText = pseudoCoded
			block.SetTargetFragment(locale, targetFrag)
		} else {
			src := block.SourceText()
			pseudo := "[" + pseudoAccent(src) + "]"
			block.SetTargetText(locale, pseudo)
		}
		return part, nil
	}

	outParts, err := runToolOnParts(ctx, pseudoTool, parts)
	if err != nil {
		return nil, fmt.Errorf("pseudo-translate: %w", err)
	}

	// Store updated blocks back.
	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := a.store.StoreBlocksForItem(ctx, projectID, itemName, blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return computeStats(outParts, targetLocale), nil
}

// AITranslateItem translates all blocks using an AI provider.
func (a *App) AITranslateItem(req AITranslateFileRequest) (*TranslationStats, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}

	storedBlocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: req.ProjectID,
		ItemName:  req.ItemName,
	})
	if err != nil {
		return nil, err
	}

	parts := storedBlocksToParts(storedBlocks)

	var prov = createProvider(req.Provider, req.APIKey, req.Model)
	if req.ProviderConfigID != "" {
		var provErr error
		prov, provErr = credentials.NewProvider(a.credentials, req.ProviderConfigID)
		if provErr != nil {
			return nil, fmt.Errorf("resolve provider config: %w", provErr)
		}
	}

	translateTool := tools.NewAITranslateTool(prov, tools.AITranslateConfig{
		SourceLocale: proj.SourceLocale,
		TargetLocale: model.LocaleID(req.TargetLocale),
	})

	outParts, err := runToolOnParts(ctx, translateTool, parts)
	if err != nil {
		return nil, fmt.Errorf("AI translate: %w", err)
	}

	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := a.store.StoreBlocksForItem(ctx, req.ProjectID, req.ItemName, blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return computeStats(outParts, req.TargetLocale), nil
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
		SourceLocale: proj.SourceLocale,
		TargetLocale: model.LocaleID(targetLocale),
	})

	outParts, err := runToolOnParts(ctx, tmTool, parts)
	if err != nil {
		return nil, fmt.Errorf("TM translate: %w", err)
	}

	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := a.store.StoreBlocksForItem(ctx, projectID, itemName, blocks); err != nil {
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
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	targetLocales := make([]string, len(proj.TargetLocales))
	for i, l := range proj.TargetLocales {
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

// ExportTranslatedItem writes the translated item to disk and returns the output path.
func (a *App) ExportTranslatedItem(projectID, itemName, targetLocale string) (string, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return "", err
	}

	item, err := a.store.GetItem(ctx, projectID, itemName)
	if err != nil {
		return "", fmt.Errorf("get item: %w", err)
	}

	if len(item.SourceBytes) == 0 {
		return "", fmt.Errorf("item %q has no source bytes for export", itemName)
	}

	// Re-parse source bytes to get the Part stream.
	reader, err := a.formatReg.NewReader(item.Format)
	if err != nil {
		return "", fmt.Errorf("no reader for %q: %w", item.Format, err)
	}

	doc := &model.RawDocument{
		URI:          itemName,
		SourceLocale: proj.SourceLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(item.SourceBytes)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return "", fmt.Errorf("parse source: %w", err)
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			return "", fmt.Errorf("read source: %w", result.Error)
		}
		parts = append(parts, result.Part)
	}
	reader.Close()

	// Load updated blocks from ContentStore and inject targets into parts.
	storedBlocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		ItemName:  itemName,
	})
	if err != nil {
		return "", fmt.Errorf("get blocks: %w", err)
	}

	blockMap := make(map[string]*model.Block, len(storedBlocks))
	for _, sb := range storedBlocks {
		blockMap[sb.Block.ID] = sb.Block
	}

	// Inject stored targets into the parsed parts.
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok {
			continue
		}
		if stored, ok := blockMap[block.ID]; ok {
			block.Targets = stored.Targets
		}
	}

	// Write output.
	ext := fileExtension(itemName)
	baseName := itemName
	if ext != "" {
		baseName = itemName[:len(itemName)-len(ext)-1]
	}
	outputPath := fmt.Sprintf("%s_%s.%s", baseName, targetLocale, ext)

	// Put output in temp dir.
	dir := filepath.Join(defaultStorePath(), "..")
	outputPath = filepath.Join(dir, outputPath)

	writer, err := a.formatReg.NewWriter(item.Format)
	if err != nil {
		return "", fmt.Errorf("no writer for %q: %w", item.Format, err)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return "", fmt.Errorf("set output: %w", err)
	}
	writer.SetLocale(model.LocaleID(targetLocale))

	ch := make(chan *model.Part, len(parts))
	for _, pt := range parts {
		ch <- pt
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	writer.Close()

	return outputPath, nil
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
	if tm.Count() == 0 {
		return nil, nil
	}

	sb, err := a.store.GetBlock(ctx, projectID, blockID)
	if err != nil {
		return nil, err
	}

	opts := sievepen.DefaultLookupOptions()
	opts.MaxResults = 5
	matches, err := tm.Lookup(sb.Block, proj.SourceLocale, model.LocaleID(targetLocale), opts)
	if err != nil {
		return nil, err
	}

	result := make([]TMMatchInfo, len(matches))
	for i, m := range matches {
		result[i] = TMMatchInfo{
			Source:    m.Entry.SourceText(),
			Target:    m.Entry.TargetText(),
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

	tb := a.getOrCreateTB()
	if tb.Count() == 0 {
		return nil, nil
	}

	sb, err := a.store.GetBlock(ctx, projectID, blockID)
	if err != nil {
		return nil, err
	}

	sourceText := sb.Block.SourceText()
	if sourceText == "" {
		return nil, nil
	}

	matches := tb.LookupAll(sourceText, termbase.LookupOptions{
		SourceLocale: proj.SourceLocale,
		TargetLocale: model.LocaleID(targetLocale),
	})

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
func runToolOnParts(ctx context.Context, t tool.Tool, parts []*model.Part) ([]*model.Part, error) {
	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, pt := range parts {
		in <- pt
	}
	close(in)

	if err := t.Process(ctx, in, out); err != nil {
		return nil, err
	}
	close(out)

	var result []*model.Part
	for pt := range out {
		result = append(result, pt)
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
