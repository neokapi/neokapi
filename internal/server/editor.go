package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gokapi/gokapi/ai/provider"
	"github.com/gokapi/gokapi/ai/tools"
	"github.com/gokapi/gokapi/core/credentials"
	"github.com/gokapi/gokapi/core/kaz"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/gokapi/gokapi/lib/sievepen"
	"github.com/gokapi/gokapi/lib/termbase"
	"github.com/google/uuid"
)

// editorProject is the server-side in-memory state for a translation project.
type editorProject struct {
	info  ProjectInfoResponse
	items map[string]*editorItemData
	dirty bool
	tm    *sievepen.SQLiteTM

	// accessedAt tracks last access for LRU eviction.
	accessedAt time.Time
}

// editorItemData holds the parsed content of an item within a project.
type editorItemData struct {
	format     string
	itemType   string // "file", "data", etc.
	parts      []*model.Part
	sourceBytes []byte
	blockIndex *kaz.BlockIndex
}

// workspaceTMTB holds workspace-scoped TM and terminology.
type workspaceTMTB struct {
	tm *sievepen.SQLiteTM
	tb *termbase.InMemoryTermBase
}

// EditorStore manages in-memory editor sessions keyed by workspace/project.
type EditorStore struct {
	mu       sync.RWMutex
	projects map[string]*editorProject // key: "ws/projectID"
	maxSize  int

	wsMu        sync.RWMutex
	workspaces  map[string]*workspaceTMTB // key: ws slug
}

// NewEditorStore creates an EditorStore with the given max capacity.
func NewEditorStore(maxSize int) *EditorStore {
	return &EditorStore{
		projects:   make(map[string]*editorProject),
		maxSize:    maxSize,
		workspaces: make(map[string]*workspaceTMTB),
	}
}

func (es *EditorStore) getOrCreateWS(ws string) *workspaceTMTB {
	es.wsMu.Lock()
	defer es.wsMu.Unlock()
	w, ok := es.workspaces[ws]
	if !ok {
		w = &workspaceTMTB{}
		es.workspaces[ws] = w
	}
	return w
}

func (es *EditorStore) getWSOrCreateTM(ws string) (*sievepen.SQLiteTM, error) {
	w := es.getOrCreateWS(ws)
	if w.tm != nil {
		return w.tm, nil
	}
	tm, err := sievepen.NewSQLiteTM(":memory:")
	if err != nil {
		return nil, err
	}
	w.tm = tm
	return tm, nil
}

func (es *EditorStore) getWSOrCreateTB(ws string) *termbase.InMemoryTermBase {
	w := es.getOrCreateWS(ws)
	if w.tb != nil {
		return w.tb
	}
	w.tb = termbase.NewInMemoryTermBase()
	return w.tb
}

func editorKey(ws, projectID string) string {
	return ws + "/" + projectID
}

func (es *EditorStore) get(ws, projectID string) (*editorProject, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()
	p, ok := es.projects[editorKey(ws, projectID)]
	if !ok {
		return nil, fmt.Errorf("project %q not found", projectID)
	}
	p.accessedAt = time.Now()
	return p, nil
}

func (es *EditorStore) put(ws string, p *editorProject) {
	es.mu.Lock()
	defer es.mu.Unlock()
	p.accessedAt = time.Now()
	es.projects[editorKey(ws, p.info.ID)] = p
	es.evictLocked()
}

func (es *EditorStore) remove(ws, projectID string) {
	es.mu.Lock()
	defer es.mu.Unlock()
	key := editorKey(ws, projectID)
	if p, ok := es.projects[key]; ok {
		if p.tm != nil {
			p.tm.Close()
		}
	}
	delete(es.projects, key)
}

func (es *EditorStore) list(ws string) []ProjectInfoResponse {
	es.mu.RLock()
	defer es.mu.RUnlock()
	prefix := ws + "/"
	var result []ProjectInfoResponse
	for key, p := range es.projects {
		if strings.HasPrefix(key, prefix) {
			result = append(result, p.info)
		}
	}
	return result
}

// evictLocked evicts the least recently used project if the store exceeds maxSize.
// Must be called with es.mu held.
func (es *EditorStore) evictLocked() {
	for len(es.projects) > es.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for key, p := range es.projects {
			if oldestKey == "" || p.accessedAt.Before(oldestTime) {
				oldestKey = key
				oldestTime = p.accessedAt
			}
		}
		if oldestKey != "" {
			if p, ok := es.projects[oldestKey]; ok {
				if p.tm != nil {
					p.tm.Close()
				}
			}
			delete(es.projects, oldestKey)
		}
	}
}

// --- Types ---

// ProjectInfoResponse is the API response for a translation project.
type ProjectInfoResponse struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	SourceLocale  string             `json:"source_locale"`
	TargetLocales []string           `json:"target_locales"`
	Items         []ProjectItemResponse `json:"items"`
	CreatedAt     string             `json:"created_at"`
	ModifiedAt    string             `json:"modified_at"`
}

// ProjectItemResponse describes an item within a project.
type ProjectItemResponse struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	BlockCount int    `json:"block_count"`
	WordCount  int    `json:"word_count"`
}

// SpanInfoResponse describes an inline span element.
type SpanInfoResponse struct {
	SpanType string `json:"span_type"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	Data     string `json:"data"`
}

// BlockInfoResponse is a serializable representation of a translatable block.
type BlockInfoResponse struct {
	ID           string            `json:"id"`
	Source       string            `json:"source"`
	SourceCoded  string            `json:"source_coded,omitempty"`
	SourceSpans  []SpanInfoResponse `json:"source_spans,omitempty"`
	Targets      map[string]string `json:"targets"`
	TargetsCoded map[string]string `json:"targets_coded,omitempty"`
	Translatable bool              `json:"translatable"`
	HasSpans     bool              `json:"has_spans"`
	Properties   map[string]string `json:"properties"`
}

// UpdateBlockTargetRequest holds parameters for updating a block target.
type UpdateBlockTargetRequest struct {
	TargetLocale string `json:"target_locale"`
	Text         string `json:"text"`
}

// UpdateBlockTargetCodedRequest holds parameters for coded text update.
type UpdateBlockTargetCodedRequest struct {
	TargetLocale string             `json:"target_locale"`
	CodedText    string             `json:"coded_text"`
	Spans        []SpanInfoResponse `json:"spans"`
}

// TranslateRequest holds parameters for translation operations.
type TranslateRequest struct {
	TargetLocale     string `json:"target_locale"`
	Provider         string `json:"provider,omitempty"`
	APIKey           string `json:"api_key,omitempty"`
	Model            string `json:"model,omitempty"`
	ProviderConfigID string `json:"provider_config_id,omitempty"`
}

// TranslationStatsResponse holds statistics about a translation operation.
type TranslationStatsResponse struct {
	TotalBlocks      int `json:"total_blocks"`
	TranslatedBlocks int `json:"translated_blocks"`
	WordCount        int `json:"word_count"`
}

// WordCountResponse holds word and character counts.
type WordCountResponse struct {
	SourceWords int            `json:"source_words"`
	SourceChars int            `json:"source_chars"`
	TargetWords map[string]int `json:"target_words"`
	TargetChars map[string]int `json:"target_chars"`
}

// TMMatchInfoResponse is a TM match result.
type TMMatchInfoResponse struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"`
}

// BlockTermMatchResponse is a term match for a block.
type BlockTermMatchResponse struct {
	SourceTerm  string   `json:"source_term"`
	TargetTerms []string `json:"target_terms"`
	Domain      string   `json:"domain"`
	Status      string   `json:"status"`
	Start       int      `json:"start"`
	End         int      `json:"end"`
}

// --- Editor operations ---

func (es *EditorStore) createProject(ws string, formatReg *registry.FormatRegistry, name, sourceLang string, targetLangs []string) (*ProjectInfoResponse, error) {
	if name == "" {
		return nil, fmt.Errorf("project name is required")
	}
	if sourceLang == "" {
		return nil, fmt.Errorf("source language is required")
	}
	if len(targetLangs) == 0 {
		return nil, fmt.Errorf("at least one target language is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	p := &editorProject{
		info: ProjectInfoResponse{
			ID:            uuid.New().String(),
			Name:          name,
			SourceLocale:  sourceLang,
			TargetLocales: targetLangs,
			Items:         []ProjectItemResponse{},
			CreatedAt:     now,
			ModifiedAt:    now,
		},
		items: make(map[string]*editorItemData),
	}

	es.put(ws, p)
	return &p.info, nil
}

func (es *EditorStore) addFiles(ws, projectID string, formatReg *registry.FormatRegistry, files map[string][]byte) (*ProjectInfoResponse, error) {
	p, err := es.get(ws, projectID)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	for itemName, data := range files {
		// Detect format from extension
		ext := filepath.Ext(itemName)
		fmtName, err := formatReg.Detector().DetectByExtension(ext)
		if err != nil {
			continue // skip unsupported formats
		}

		reader, err := formatReg.NewReader(fmtName)
		if err != nil {
			continue
		}

		doc := &model.RawDocument{
			URI:          itemName,
			SourceLocale: model.LocaleID(p.info.SourceLocale),
			Encoding:     "UTF-8",
			Reader:       io.NopCloser(bytes.NewReader(data)),
		}

		if err := reader.Open(ctx, doc); err != nil {
			return nil, fmt.Errorf("parse %q: %w", itemName, err)
		}

		var parts []*model.Part
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				reader.Close()
				return nil, fmt.Errorf("read %q: %w", itemName, result.Error)
			}
			parts = append(parts, result.Part)
		}
		reader.Close()

		blockIndex := kaz.BuildBlockIndex(parts, p.info.SourceLocale, fmtName, itemName)

		blockCount := len(blockIndex.Blocks)
		wordCount := 0
		for _, b := range blockIndex.Blocks {
			wordCount += editorCountWords(b.Source)
		}

		p.items[itemName] = &editorItemData{
			format:      fmtName,
			itemType:    "file",
			parts:       parts,
			sourceBytes: data,
			blockIndex:  blockIndex,
		}

		p.info.Items = append(p.info.Items, ProjectItemResponse{
			Name:       itemName,
			Format:     fmtName,
			Type:       "file",
			Size:       int64(len(data)),
			BlockCount: blockCount,
			WordCount:  wordCount,
		})
	}

	p.info.ModifiedAt = time.Now().UTC().Format(time.RFC3339)
	p.dirty = true
	return &p.info, nil
}

func (es *EditorStore) removeFile(ws, projectID, fileName string) (*ProjectInfoResponse, error) {
	p, err := es.get(ws, projectID)
	if err != nil {
		return nil, err
	}

	if _, ok := p.items[fileName]; !ok {
		return nil, fmt.Errorf("file %q not found in project", fileName)
	}

	delete(p.items, fileName)

	var updated []ProjectItemResponse
	for _, item := range p.info.Items {
		if item.Name != fileName {
			updated = append(updated, item)
		}
	}
	p.info.Items = updated
	p.info.ModifiedAt = time.Now().UTC().Format(time.RFC3339)
	p.dirty = true
	return &p.info, nil
}

func (es *EditorStore) getBlocks(ws, projectID, itemName string) ([]BlockInfoResponse, error) {
	p, err := es.get(ws, projectID)
	if err != nil {
		return nil, err
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in project", itemName)
	}

	blockMap := make(map[string]*model.Block)
	for _, pt := range id.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		if block, ok := pt.Resource.(*model.Block); ok {
			blockMap[block.ID] = block
		}
	}

	if id.blockIndex != nil {
		var blocks []BlockInfoResponse
		for _, b := range id.blockIndex.Blocks {
			bi := BlockInfoResponse{
				ID:           b.ID,
				Source:       b.Source,
				Targets:      copyStringMap(b.Targets),
				Translatable: b.Translatable,
				HasSpans:     b.SourceHTML != b.Source,
				Properties:   copyStringMap(b.Properties),
			}
			if mb, ok := blockMap[b.ID]; ok {
				enrichBlockInfoResponse(&bi, mb, p.info.TargetLocales)
			}
			blocks = append(blocks, bi)
		}
		return blocks, nil
	}

	var blocks []BlockInfoResponse
	for _, pt := range id.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok {
			continue
		}

		targets := make(map[string]string)
		for _, locale := range p.info.TargetLocales {
			if t := block.TargetText(model.LocaleID(locale)); t != "" {
				targets[locale] = t
			}
		}

		props := make(map[string]string)
		for k, v := range block.Properties {
			props[k] = v
		}

		hasSpans := false
		if len(block.Source) > 0 && block.Source[0].Content != nil {
			hasSpans = block.Source[0].Content.HasSpans()
		}

		bi := BlockInfoResponse{
			ID:           block.ID,
			Source:       block.SourceText(),
			Targets:      targets,
			Translatable: block.Translatable,
			HasSpans:     hasSpans,
			Properties:   props,
		}
		enrichBlockInfoResponse(&bi, block, p.info.TargetLocales)
		blocks = append(blocks, bi)
	}

	return blocks, nil
}

func (es *EditorStore) updateBlockTarget(ws, projectID, blockID string, req UpdateBlockTargetRequest) error {
	p, err := es.get(ws, projectID)
	if err != nil {
		return err
	}

	// Find the item containing this block
	for _, id := range p.items {
		if id.blockIndex != nil {
			if err := id.blockIndex.UpdateTarget(blockID, req.TargetLocale, req.Text); err != nil {
				continue // block not in this item
			}
		}

		for _, pt := range id.parts {
			if pt.Type != model.PartBlock {
				continue
			}
			block, ok := pt.Resource.(*model.Block)
			if !ok || block.ID != blockID {
				continue
			}
			block.SetTargetText(model.LocaleID(req.TargetLocale), req.Text)
			p.dirty = true
			return nil
		}
	}

	return fmt.Errorf("block %q not found", blockID)
}

func (es *EditorStore) updateBlockTargetCoded(ws, projectID, blockID string, req UpdateBlockTargetCodedRequest) error {
	p, err := es.get(ws, projectID)
	if err != nil {
		return err
	}

	plainText := editorStripMarkers(req.CodedText)

	for _, id := range p.items {
		if id.blockIndex != nil {
			if err := id.blockIndex.UpdateTarget(blockID, req.TargetLocale, plainText); err != nil {
				continue
			}
		}

		for _, pt := range id.parts {
			if pt.Type != model.PartBlock {
				continue
			}
			block, ok := pt.Resource.(*model.Block)
			if !ok || block.ID != blockID {
				continue
			}

			frag := &model.Fragment{
				CodedText: req.CodedText,
			}
			for _, si := range req.Spans {
				frag.Spans = append(frag.Spans, editorInfoToSpan(si))
			}
			block.SetTargetFragment(model.LocaleID(req.TargetLocale), frag)
			p.dirty = true
			return nil
		}
	}

	return fmt.Errorf("block %q not found", blockID)
}

func (es *EditorStore) pseudoTranslate(ws, projectID, itemName, targetLocale string) (*TranslationStatsResponse, error) {
	p, err := es.get(ws, projectID)
	if err != nil {
		return nil, err
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in project", itemName)
	}

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
			pseudoCoded := "[" + editorPseudoAccent(frag.CodedText) + "]"
			targetFrag := frag.Clone()
			targetFrag.CodedText = pseudoCoded
			block.SetTargetFragment(locale, targetFrag)
		} else {
			src := block.SourceText()
			pseudo := "[" + editorPseudoAccent(src) + "]"
			block.SetTargetText(locale, pseudo)
		}
		return part, nil
	}

	ctx := context.Background()
	in := make(chan *model.Part, len(id.parts))
	out := make(chan *model.Part, len(id.parts))
	for _, pt := range id.parts {
		in <- pt
	}
	close(in)

	if err := pseudoTool.Process(ctx, in, out); err != nil {
		return nil, fmt.Errorf("pseudo-translate: %w", err)
	}
	close(out)

	var newParts []*model.Part
	for pt := range out {
		newParts = append(newParts, pt)
	}
	id.parts = newParts

	editorSyncBlockIndex(id, p.info.SourceLocale)
	stats := editorComputeStats(id.parts, targetLocale)
	p.dirty = true
	return stats, nil
}

func (es *EditorStore) aiTranslate(ws, projectID, itemName string, req TranslateRequest, credStore *credentials.Store) (*TranslationStatsResponse, error) {
	p, err := es.get(ws, projectID)
	if err != nil {
		return nil, err
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in project", itemName)
	}

	var prov provider.LLMProvider
	if req.ProviderConfigID != "" && credStore != nil {
		prov, err = credentials.NewProvider(credStore, req.ProviderConfigID)
		if err != nil {
			return nil, fmt.Errorf("resolve provider config: %w", err)
		}
	} else {
		prov = editorCreateProvider(req.Provider, req.APIKey, req.Model)
	}

	translateTool := tools.NewAITranslateTool(prov, tools.AITranslateConfig{
		SourceLocale: model.LocaleID(p.info.SourceLocale),
		TargetLocale: model.LocaleID(req.TargetLocale),
	})

	ctx := context.Background()
	in := make(chan *model.Part, len(id.parts))
	out := make(chan *model.Part, len(id.parts))
	for _, pt := range id.parts {
		in <- pt
	}
	close(in)

	if err := translateTool.Process(ctx, in, out); err != nil {
		return nil, fmt.Errorf("AI translate: %w", err)
	}
	close(out)

	var newParts []*model.Part
	for pt := range out {
		newParts = append(newParts, pt)
	}
	id.parts = newParts

	editorSyncBlockIndex(id, p.info.SourceLocale)
	stats := editorComputeStats(id.parts, req.TargetLocale)
	p.dirty = true
	return stats, nil
}

func (es *EditorStore) tmTranslate(ws, projectID, itemName, targetLocale string) (*TranslationStatsResponse, error) {
	p, err := es.get(ws, projectID)
	if err != nil {
		return nil, err
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in project", itemName)
	}

	tm, err := editorGetOrCreateTM(p)
	if err != nil {
		return nil, fmt.Errorf("init TM: %w", err)
	}

	tmTool := sievepen.NewTMLeverageTool(tm, sievepen.TMLeverageConfig{
		MinScore:     0.7,
		MaxResults:   5,
		SourceLocale: model.LocaleID(p.info.SourceLocale),
		TargetLocale: model.LocaleID(targetLocale),
	})

	ctx := context.Background()
	in := make(chan *model.Part, len(id.parts))
	out := make(chan *model.Part, len(id.parts))
	for _, pt := range id.parts {
		in <- pt
	}
	close(in)

	if err := tmTool.Process(ctx, in, out); err != nil {
		return nil, fmt.Errorf("TM translate: %w", err)
	}
	close(out)

	var newParts []*model.Part
	for pt := range out {
		newParts = append(newParts, pt)
	}
	id.parts = newParts

	editorSyncBlockIndex(id, p.info.SourceLocale)
	stats := editorComputeStats(id.parts, targetLocale)
	p.dirty = true
	return stats, nil
}

func (es *EditorStore) getWordCount(ws, projectID, itemName string) (*WordCountResponse, error) {
	p, err := es.get(ws, projectID)
	if err != nil {
		return nil, err
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in project", itemName)
	}

	result := &WordCountResponse{
		TargetWords: make(map[string]int),
		TargetChars: make(map[string]int),
	}

	for _, pt := range id.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}

		src := block.SourceText()
		result.SourceWords += editorCountWords(src)
		result.SourceChars += len([]rune(src))

		for _, locale := range p.info.TargetLocales {
			t := block.TargetText(model.LocaleID(locale))
			if t != "" {
				result.TargetWords[locale] += editorCountWords(t)
				result.TargetChars[locale] += len([]rune(t))
			}
		}
	}

	return result, nil
}

func (es *EditorStore) exportTranslatedFile(ws, projectID, itemName, targetLocale string, formatReg *registry.FormatRegistry, dataDir string) (string, error) {
	p, err := es.get(ws, projectID)
	if err != nil {
		return "", err
	}

	id, ok := p.items[itemName]
	if !ok {
		return "", fmt.Errorf("item %q not found in project", itemName)
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(itemName)), ".")
	baseName := itemName
	if ext != "" {
		baseName = itemName[:len(itemName)-len(ext)-1]
	}
	outputName := fmt.Sprintf("%s_%s.%s", baseName, targetLocale, ext)

	// Write to data dir or temp dir
	dir := dataDir
	if dir == "" {
		dir = os.TempDir()
	}
	outputPath := filepath.Join(dir, outputName)

	writer, err := formatReg.NewWriter(id.format)
	if err != nil {
		return "", fmt.Errorf("no writer for %q: %w", id.format, err)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return "", fmt.Errorf("set output: %w", err)
	}
	writer.SetLocale(model.LocaleID(targetLocale))

	ch := make(chan *model.Part, len(id.parts))
	for _, pt := range id.parts {
		ch <- pt
	}
	close(ch)

	ctx := context.Background()
	if err := writer.Write(ctx, ch); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	writer.Close()

	return outputPath, nil
}

func (es *EditorStore) lookupTMForBlock(ws, projectID, itemName, blockID, targetLocale string) ([]TMMatchInfoResponse, error) {
	p, err := es.get(ws, projectID)
	if err != nil {
		return nil, err
	}
	if p.tm == nil || p.tm.Count() == 0 {
		return nil, nil
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found", itemName)
	}

	for _, pt := range id.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok || block.ID != blockID {
			continue
		}

		opts := sievepen.DefaultLookupOptions()
		opts.MaxResults = 5
		matches, err := p.tm.Lookup(block, model.LocaleID(p.info.SourceLocale), model.LocaleID(targetLocale), opts)
		if err != nil {
			return nil, err
		}

		result := make([]TMMatchInfoResponse, len(matches))
		for i, m := range matches {
			result[i] = TMMatchInfoResponse{
				Source:    m.Entry.SourceText(),
				Target:    m.Entry.TargetText(),
				Score:     m.Score,
				MatchType: string(m.MatchType),
			}
		}
		return result, nil
	}

	return nil, nil
}

func (es *EditorStore) lookupTermsForBlock(ws, projectID, itemName, blockID, targetLocale string) ([]BlockTermMatchResponse, error) {
	p, err := es.get(ws, projectID)
	if err != nil {
		return nil, err
	}
	tb := es.getWSOrCreateTB(ws)
	if tb.Count() == 0 {
		return nil, nil
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found", itemName)
	}

	for _, pt := range id.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok || block.ID != blockID {
			continue
		}

		sourceText := block.SourceText()
		if sourceText == "" {
			return nil, nil
		}

		matches := tb.LookupAll(sourceText, termbase.LookupOptions{
			SourceLocale: model.LocaleID(p.info.SourceLocale),
			TargetLocale: model.LocaleID(targetLocale),
		})

		var result []BlockTermMatchResponse
		for _, m := range matches {
			var targetTerms []string
			for _, t := range m.Concept.Terms {
				if t.Locale == model.LocaleID(targetLocale) {
					targetTerms = append(targetTerms, t.Text)
				}
			}
			result = append(result, BlockTermMatchResponse{
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

	return nil, nil
}

// --- TM operations ---

// TMEntryInfoResponse is the API response for a TM entry.
type TMEntryInfoResponse struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	UpdatedAt    string `json:"updated_at"`
}

// TMSearchResponse holds a page of TM search results.
type TMSearchResponse struct {
	Entries    []TMEntryInfoResponse `json:"entries"`
	TotalCount int                   `json:"total_count"`
}

// TMAddRequest holds parameters for adding a TM entry.
type TMAddRequest struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
}

// TMUpdateRequest holds parameters for updating a TM entry.
type TMUpdateRequest struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
}

func editorGetOrCreateTM(p *editorProject) (*sievepen.SQLiteTM, error) {
	if p.tm != nil {
		return p.tm, nil
	}
	tm, err := sievepen.NewSQLiteTM(":memory:")
	if err != nil {
		return nil, err
	}
	p.tm = tm
	return tm, nil
}

func editorEntryToInfo(e sievepen.TMEntry) TMEntryInfoResponse {
	return TMEntryInfoResponse{
		ID:           e.ID,
		Source:       e.SourceText(),
		Target:       e.TargetText(),
		SourceLocale: string(e.SourceLocale),
		TargetLocale: string(e.TargetLocale),
		UpdatedAt:    e.UpdatedAt.Format(time.RFC3339),
	}
}

func (es *EditorStore) getTMEntries(ws, query, sourceLocale, targetLocale string, offset, limit int) (*TMSearchResponse, error) {
	tm, err := es.getWSOrCreateTM(ws)
	if err != nil {
		return nil, fmt.Errorf("init TM: %w", err)
	}

	entries, total := tm.SearchEntries(query, sourceLocale, targetLocale, offset, limit)
	infos := make([]TMEntryInfoResponse, len(entries))
	for i, e := range entries {
		infos[i] = editorEntryToInfo(e)
	}

	return &TMSearchResponse{
		Entries:    infos,
		TotalCount: total,
	}, nil
}

func (es *EditorStore) getTMCount(ws string) (int, error) {
	tm, err := es.getWSOrCreateTM(ws)
	if err != nil {
		return 0, err
	}
	return tm.Count(), nil
}

func (es *EditorStore) addTMEntry(ws string, req TMAddRequest) (*TMEntryInfoResponse, error) {
	tm, err := es.getWSOrCreateTM(ws)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	entry := sievepen.TMEntry{
		ID:           uuid.New().String(),
		Source:       model.NewFragment(req.Source),
		Target:       model.NewFragment(req.Target),
		SourceLocale: model.LocaleID(req.SourceLocale),
		TargetLocale: model.LocaleID(req.TargetLocale),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := tm.Add(entry); err != nil {
		return nil, err
	}

	info := editorEntryToInfo(entry)
	return &info, nil
}

func (es *EditorStore) updateTMEntry(ws, entryID string, req TMUpdateRequest) error {
	tm, err := es.getWSOrCreateTM(ws)
	if err != nil {
		return err
	}

	entry, ok := tm.GetEntry(entryID)
	if !ok {
		return fmt.Errorf("TM entry %q not found", entryID)
	}

	entry.Source = model.NewFragment(req.Source)
	entry.Target = model.NewFragment(req.Target)
	entry.SourceLocale = model.LocaleID(req.SourceLocale)
	entry.TargetLocale = model.LocaleID(req.TargetLocale)
	entry.UpdatedAt = time.Now()

	return tm.Add(entry)
}

func (es *EditorStore) deleteTMEntry(ws, entryID string) error {
	tm, err := es.getWSOrCreateTM(ws)
	if err != nil {
		return err
	}
	return tm.Delete(entryID)
}

// --- Terminology operations ---

// TermInfoResponse is a term in a concept.
type TermInfoResponse struct {
	Text         string `json:"text"`
	Locale       string `json:"locale"`
	Status       string `json:"status"`
	PartOfSpeech string `json:"part_of_speech,omitempty"`
	Gender       string `json:"gender,omitempty"`
	Note         string `json:"note,omitempty"`
}

// ConceptInfoResponse is the API response for a concept.
type ConceptInfoResponse struct {
	ID         string            `json:"id"`
	Domain     string            `json:"domain"`
	Definition string            `json:"definition"`
	Terms      []TermInfoResponse `json:"terms"`
	Properties map[string]string `json:"properties,omitempty"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
}

// TermSearchResponse holds a page of term search results.
type TermSearchResponse struct {
	Concepts   []ConceptInfoResponse `json:"concepts"`
	TotalCount int                   `json:"total_count"`
}

// AddConceptRequest holds parameters for adding a concept.
type AddConceptRequest struct {
	Domain     string             `json:"domain"`
	Definition string             `json:"definition"`
	Terms      []TermInfoResponse `json:"terms"`
}

// UpdateConceptRequest holds parameters for updating a concept.
type UpdateConceptRequest struct {
	Domain     string             `json:"domain"`
	Definition string             `json:"definition"`
	Terms      []TermInfoResponse `json:"terms"`
}

// ImportCSVRequest holds parameters for CSV term import.
type ImportCSVRequest struct {
	CSVContent   string `json:"csv_content"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	Domain       string `json:"domain"`
	HasHeader    bool   `json:"has_header"`
}

// ImportJSONRequest holds parameters for JSON term import.
type ImportJSONRequest struct {
	JSONContent string `json:"json_content"`
}

// ExportJSONRequest holds parameters for JSON term export.
type ExportJSONRequest struct {
	Name string `json:"name"`
}

func editorConceptToInfo(c termbase.Concept) ConceptInfoResponse {
	terms := make([]TermInfoResponse, len(c.Terms))
	for i, t := range c.Terms {
		terms[i] = TermInfoResponse{
			Text:         t.Text,
			Locale:       string(t.Locale),
			Status:       string(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		}
	}
	return ConceptInfoResponse{
		ID:         c.ID,
		Domain:     c.Domain,
		Definition: c.Definition,
		Terms:      terms,
		Properties: c.Properties,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  c.UpdatedAt.Format(time.RFC3339),
	}
}

func editorTermsFromInfo(terms []TermInfoResponse) []termbase.Term {
	result := make([]termbase.Term, len(terms))
	for i, t := range terms {
		result[i] = termbase.Term{
			Text:         t.Text,
			Locale:       model.LocaleID(t.Locale),
			Status:       model.TermStatus(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		}
		if result[i].Status == "" {
			result[i].Status = model.TermApproved
		}
	}
	return result
}

func (es *EditorStore) getTerms(ws, query, sourceLocale, targetLocale string, offset, limit int) (*TermSearchResponse, error) {
	tb := es.getWSOrCreateTB(ws)
	results, total := tb.Search(query, sourceLocale, targetLocale, offset, limit)
	infos := make([]ConceptInfoResponse, len(results))
	for i, c := range results {
		infos[i] = editorConceptToInfo(c)
	}
	return &TermSearchResponse{
		Concepts:   infos,
		TotalCount: total,
	}, nil
}

func (es *EditorStore) getTermCount(ws string) int {
	tb := es.getWSOrCreateTB(ws)
	return tb.Count()
}

func (es *EditorStore) addConcept(ws string, req AddConceptRequest) (*ConceptInfoResponse, error) {
	tb := es.getWSOrCreateTB(ws)
	concept := termbase.Concept{
		ID:         uuid.New().String(),
		Domain:     req.Domain,
		Definition: req.Definition,
		Terms:      editorTermsFromInfo(req.Terms),
	}

	if err := tb.AddConcept(concept); err != nil {
		return nil, err
	}

	stored, _ := tb.GetConcept(concept.ID)
	info := editorConceptToInfo(stored)
	return &info, nil
}

func (es *EditorStore) updateConcept(ws, conceptID string, req UpdateConceptRequest) error {
	tb := es.getWSOrCreateTB(ws)
	concept := termbase.Concept{
		ID:         conceptID,
		Domain:     req.Domain,
		Definition: req.Definition,
		Terms:      editorTermsFromInfo(req.Terms),
	}

	return tb.AddConcept(concept)
}

func (es *EditorStore) deleteConcept(ws, conceptID string) error {
	tb := es.getWSOrCreateTB(ws)
	return tb.DeleteConcept(conceptID)
}

func (es *EditorStore) importTermsCSV(ws string, req ImportCSVRequest) (int, error) {
	tb := es.getWSOrCreateTB(ws)
	count, err := termbase.ImportCSV(tb, strings.NewReader(req.CSVContent), termbase.CSVImportOptions{
		SourceLocale: model.LocaleID(req.SourceLocale),
		TargetLocale: model.LocaleID(req.TargetLocale),
		Domain:       req.Domain,
		HasHeader:    req.HasHeader,
	})
	if err != nil {
		return 0, fmt.Errorf("import CSV: %w", err)
	}
	return count, nil
}

func (es *EditorStore) importTermsJSON(ws string, jsonContent string) (int, error) {
	tb := es.getWSOrCreateTB(ws)
	count, err := termbase.ImportJSON(tb, strings.NewReader(jsonContent))
	if err != nil {
		return 0, fmt.Errorf("import JSON: %w", err)
	}
	return count, nil
}

func (es *EditorStore) exportTermsJSON(ws, name string) (string, error) {
	tb := es.getWSOrCreateTB(ws)
	var buf bytes.Buffer
	if err := termbase.ExportJSON(tb, &buf, name); err != nil {
		return "", fmt.Errorf("export JSON: %w", err)
	}
	return buf.String(), nil
}

// --- Provider operations ---

// ProviderConfigResponse is the API response for a provider config (no API key).
type ProviderConfigResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url"`
}

// SaveProviderConfigRequest is used to create/update a provider with optional API key.
type SaveProviderConfigRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
}

func toProviderConfigResponse(c credentials.ProviderConfig) ProviderConfigResponse {
	return ProviderConfigResponse{
		ID:           c.ID,
		Name:         c.Name,
		ProviderType: c.ProviderType,
		Model:        c.Model,
		BaseURL:      c.BaseURL,
	}
}

func (r SaveProviderConfigRequest) toCredentials() credentials.ProviderConfig {
	return credentials.ProviderConfig{
		ID:           r.ID,
		Name:         r.Name,
		ProviderType: r.ProviderType,
		Model:        r.Model,
		BaseURL:      r.BaseURL,
	}
}

// --- Helper functions ---

func enrichBlockInfoResponse(bi *BlockInfoResponse, block *model.Block, targetLocales []string) {
	if len(block.Source) == 0 || block.Source[0].Content == nil {
		return
	}
	frag := block.Source[0].Content
	if !frag.HasSpans() {
		return
	}

	bi.HasSpans = true
	bi.SourceCoded = frag.CodedText
	bi.SourceSpans = make([]SpanInfoResponse, len(frag.Spans))
	for i, s := range frag.Spans {
		bi.SourceSpans[i] = editorSpanToInfo(s)
	}

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

func editorSpanToInfo(s *model.Span) SpanInfoResponse {
	var spanType string
	switch s.SpanType {
	case model.SpanOpening:
		spanType = "opening"
	case model.SpanClosing:
		spanType = "closing"
	case model.SpanPlaceholder:
		spanType = "placeholder"
	}
	return SpanInfoResponse{
		SpanType: spanType,
		Type:     s.Type,
		ID:       s.ID,
		Data:     s.Data,
	}
}

func editorInfoToSpan(si SpanInfoResponse) *model.Span {
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

func editorStripMarkers(coded string) string {
	var buf []byte
	for _, r := range coded {
		if r < '\uE001' || r > '\uE003' {
			buf = append(buf, []byte(string(r))...)
		}
	}
	return string(buf)
}

func editorComputeStats(parts []*model.Part, targetLocale string) *TranslationStatsResponse {
	stats := &TranslationStatsResponse{}
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		stats.TotalBlocks++
		stats.WordCount += editorCountWords(block.SourceText())
		if block.TargetText(model.LocaleID(targetLocale)) != "" {
			stats.TranslatedBlocks++
		}
	}
	return stats
}

func editorSyncBlockIndex(id *editorItemData, sourceLocale string) {
	if id.blockIndex == nil {
		return
	}
	for _, pt := range id.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok {
			continue
		}
		b := id.blockIndex.BlockByID(block.ID)
		if b == nil {
			continue
		}
		b.Targets = make(map[string]string)
		for locale, segs := range block.Targets {
			if len(segs) > 0 {
				b.Targets[string(locale)] = block.TargetText(locale)
			}
		}
	}
}

func editorCountWords(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

func editorPseudoAccent(text string) string {
	var buf bytes.Buffer
	for _, r := range text {
		if replacement, ok := editorAccentMap[r]; ok {
			buf.WriteRune(replacement)
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

var editorAccentMap = map[rune]rune{
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

func editorCreateProvider(provType, apiKey, modelName string) provider.LLMProvider {
	return credentials.NewProviderFromConfig(credentials.ProviderConfig{
		ProviderType: provType,
		Model:        modelName,
	}, apiKey)
}

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
